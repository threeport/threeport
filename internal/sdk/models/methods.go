package models

import (
	"fmt"
	"os"

	. "github.com/dave/jennifer/jen"
	"github.com/gertd/go-pluralize"
	"github.com/iancoleman/strcase"

	"github.com/threeport/threeport/internal/sdk"
)

// ModelConstantsMethods generates the constants and methods for each model.
func (cc *ControllerConfig) ModelConstantsMethods() error {
	pluralize := pluralize.NewClient()
	f := NewFile(cc.ParsedModelFile.Name.Name)
	f.HeaderComment("generated by 'threeport-sdk codegen api-model' - do not edit")

	f.ImportAlias("github.com/threeport/threeport/pkg/notifications/v0", "notifications")

	// object type constants
	objectTypes := &Statement{}
	for _, mc := range cc.ModelConfigs {
		objectTypes.Id(fmt.Sprintf(
			"ObjectType%s", mc.TypeName,
		)).Id("ObjectType").Op("=").Lit(mc.TypeName)
		objectTypes.Line()
	}
	// NATS subject constants used for controller notifications
	type modelSubjects struct {
		model    string
		subjects []string
	}
	subjects := &Statement{}
	for i, mc := range cc.ModelConfigs {
		mc.CreateSubject = mc.TypeName + "CreateSubject"
		mc.UpdateSubject = mc.TypeName + "UpdateSubject"
		mc.DeleteSubject = mc.TypeName + "DeleteSubject"
		cc.ModelConfigs[i] = mc
		subjects.Id(mc.TypeName + "Subject").Op("=").Lit(strcase.ToLowerCamel(mc.TypeName) + ".*")
		subjects.Line()
		subjects.Id(mc.CreateSubject).Op("=").Lit(strcase.ToLowerCamel(mc.TypeName) + ".create")
		subjects.Line()
		subjects.Id(mc.UpdateSubject).Op("=").Lit(strcase.ToLowerCamel(mc.TypeName) + ".update")
		subjects.Line()
		subjects.Id(mc.DeleteSubject).Op("=").Lit(strcase.ToLowerCamel(mc.TypeName) + ".delete")
		subjects.Line()
		subjects.Line()
	}
	// API routing path constants
	paths := &Statement{}
	for _, mc := range cc.ModelConfigs {
		paths.Id("Path" + pluralize.Pluralize(mc.TypeName, 2, false)).Op("=").Lit(
			fmt.Sprintf("/%s/%s", cc.ParsedModelFile.Name, pluralize.Pluralize(strcase.ToKebab(mc.TypeName), 2, false)),
		)
		paths.Line()
	}
	f.Const().Defs(
		objectTypes,
		Line(),
		Id(cc.ControllerDomain+"StreamName").Op("=").Lit(cc.ControllerDomainLower+"Stream"),
		Line(),
		subjects,
		Line(),
		paths,
	)
	f.Line()
	// NATS subject functions by object
	var subjectFuncs []string
	for _, mc := range cc.ModelConfigs {
		funcName := fmt.Sprintf("Get%sSubjects", mc.TypeName)
		subjectFuncs = append(subjectFuncs, funcName)
		subjects := &Statement{}
		subjects.Id(mc.CreateSubject).Op(",")
		subjects.Line()
		subjects.Id(mc.UpdateSubject).Op(",")
		subjects.Line()
		subjects.Id(mc.DeleteSubject).Op(",")
		f.Comment(fmt.Sprintf("%s returns the NATS subjects", funcName))
		f.Comment(fmt.Sprintf("for %s.", pluralize.Pluralize(strcase.ToDelimited(mc.TypeName, ' '), 2, false)))
		f.Func().Id(funcName).Params().Index().String().Block(
			Return(
				Index().String().Block(
					subjects,
				),
			),
		)
		f.Line()
	}
	// all NATS subjects for controller domain
	controllerSubjectsFuncName := fmt.Sprintf("Get%sSubjects", cc.ControllerDomain)
	controllerSubjectsLower := fmt.Sprintf("%sSubjects", cc.ControllerDomainLower)
	subjectAppends := &Statement{}
	for _, sf := range subjectFuncs {
		subjectAppends.Id(controllerSubjectsLower).Op("=").Append(
			Id(controllerSubjectsLower),
			Id(sf).Call().Op("..."),
		)
		subjectAppends.Line()
	}
	f.Comment(fmt.Sprintf("%s returns the NATS subjects", controllerSubjectsFuncName))
	f.Comment(fmt.Sprintf("for all %s objects.", strcase.ToDelimited(cc.ControllerDomain, ' ')))
	f.Func().Id(controllerSubjectsFuncName).Params().Index().String().Block(
		Var().Id(controllerSubjectsLower).Index().String(),
		Line(),
		subjectAppends,
		Line(),
		Return(Id(controllerSubjectsLower)),
	)
	// API object methods
	for _, mc := range cc.ModelConfigs {
		// NotificationPayload method
		f.Comment("NotificationPayload returns the notification payload that is delivered to the")
		f.Comment("controller when a change is made.  It includes the object as presented by the")
		f.Comment("client when the change was made.")
		f.Func().Params(
			Id(sdk.TypeAbbrev(mc.TypeName)).Op("*").Id(mc.TypeName),
		).Id("NotificationPayload").Params(
			Line().Id("operation").Qual(
				"github.com/threeport/threeport/pkg/notifications/v0",
				"NotificationOperation",
			),
			Line().Id("requeue").Bool(),
			Line().Id("creationTime").Int64(),
			Line(),
		).Parens(List(
			Op("*").Index().Byte(),
			Error(),
		)).Block(
			Id("notif").Op(":=").Qual(
				"github.com/threeport/threeport/pkg/notifications/v0",
				"Notification",
			).Values(Dict{
				Id("Operation"):    Id("operation"),
				Id("CreationTime"): Op("&").Id("creationTime"),
				Id("Object"):       Id(sdk.TypeAbbrev(mc.TypeName)),
			}),
			Line(),
			List(
				Id("payload"), Err(),
			).Op(":=").Qual("encoding/json", "Marshal").Call(Id("notif")),
			If(
				Err().Op("!=").Nil(),
			).Block(
				Return(List(
					Op("&").Id("payload"),
					Qual("fmt", "Errorf").Call(
						Lit("failed to marshal notification payload %+v: %w"),
						Id(sdk.TypeAbbrev(mc.TypeName)),
						Err(),
					),
				)),
			),
			Line(),
			Return(
				Op("&").Id("payload"),
				Nil(),
			),
		)
		f.Line()
		// DecodeNotifObject method
		f.Comment("DecodeNotifObject takes the threeport object in the form of a")
		f.Comment("map[string]interface and returns the typed object by marshalling into JSON")
		f.Comment("and then unmarshalling into the typed object.  We are not using the")
		f.Comment("mapstructure library here as that requires custom decode hooks to manage")
		f.Comment("fields with non-native go types.")
		f.Func().Params(
			Id(sdk.TypeAbbrev(mc.TypeName)).Op("*").Id(mc.TypeName),
		).Id("DecodeNotifObject").Params(Id("object").Interface()).Error().Block(
			List(Id("jsonObject"), Id("err")).Op(":=").Qual("encoding/json", "Marshal").Call(Id("object")),
			If(Id("err").Op("!=").Nil()).Block(
				Return(Qual("fmt", "Errorf").Call(
					Lit("failed to marshal object map from consumed notification message: %w"), Id("err")),
				),
			),
			If(Err().Op(":=").Qual("encoding/json", "Unmarshal").Call(
				Id("jsonObject"), Op("&").Id(sdk.TypeAbbrev(mc.TypeName)),
			).Op(";").Id("err").Op("!=").Nil()).Block(
				Return(Qual("fmt", "Errorf").Call(
					Lit("failed to unmarshal json object to typed object: %w"), Id("err"),
				)),
			),
			Return(Nil()),
		)
		// GetID method
		f.Comment("GetID returns the unique ID for the object.")
		f.Func().Params(
			Id(sdk.TypeAbbrev(mc.TypeName)).Op("*").Id(mc.TypeName),
		).Id("GetID").Params().Uint().Block(
			Return(Op("*").Id(sdk.TypeAbbrev(mc.TypeName)).Dot("ID")),
		)
		// String method
		f.Comment("String returns a string representation of the ojbect.")
		f.Func().Params(
			Id(sdk.TypeAbbrev(mc.TypeName)).Id(mc.TypeName),
		).Id("String").Params().String().Block(
			Return(Qual(
				"fmt", "Sprintf",
			).Call(Lit(fmt.Sprintf("%s.%s", cc.PackageName, mc.TypeName))),
			),
		)
	}

	// write code to file
	genFilename := fmt.Sprintf("%s_gen.go", sdk.FilenameSansExt(cc.ModelFilename))
	file, err := os.OpenFile(genFilename, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return fmt.Errorf("failed to open file to write generated code for database models: %w", err)
	}
	defer file.Close()
	if err := f.Render(file); err != nil {
		return fmt.Errorf("failed to render generated source code for database models: %w", err)
	}
	fmt.Printf("code generation complete for %s database models\n", cc.ControllerDomainLower)

	return nil
}

// ExtensionModelConstantsMethods generates the constants and methods for each model on a threeport extension.
func (cc *ControllerConfig) ExtensionModelConstantsMethods() error {
	pluralize := pluralize.NewClient()
	f := NewFile(cc.ParsedModelFile.Name.Name)
	f.HeaderComment("generated by 'threeport-sdk codegen api-model' - do not edit")

	f.ImportAlias("github.com/threeport/threeport/pkg/notifications/v0", "notifications")
	f.ImportAlias("github.com/threeport/threeport/pkg/api/v0", "tpv0")

	// object type constants
	objectTypes := &Statement{}
	for _, mc := range cc.ModelConfigs {
		objectTypes.Id(fmt.Sprintf(
			"ObjectType%s", mc.TypeName,
		)).Qual(
			"github.com/threeport/threeport/pkg/api/v0",
			"ObjectType",
		).Op("=").Lit(mc.TypeName)
		objectTypes.Line()
	}
	// NATS subject constants used for controller notifications
	type modelSubjects struct {
		model    string
		subjects []string
	}
	subjects := &Statement{}
	for i, mc := range cc.ModelConfigs {
		mc.CreateSubject = mc.TypeName + "CreateSubject"
		mc.UpdateSubject = mc.TypeName + "UpdateSubject"
		mc.DeleteSubject = mc.TypeName + "DeleteSubject"
		cc.ModelConfigs[i] = mc
		subjects.Id(mc.TypeName + "Subject").Op("=").Lit(strcase.ToLowerCamel(mc.TypeName) + ".*")
		subjects.Line()
		subjects.Id(mc.CreateSubject).Op("=").Lit(strcase.ToLowerCamel(mc.TypeName) + ".create")
		subjects.Line()
		subjects.Id(mc.UpdateSubject).Op("=").Lit(strcase.ToLowerCamel(mc.TypeName) + ".update")
		subjects.Line()
		subjects.Id(mc.DeleteSubject).Op("=").Lit(strcase.ToLowerCamel(mc.TypeName) + ".delete")
		subjects.Line()
		subjects.Line()
	}
	// API routing path constants
	paths := &Statement{}
	for _, mc := range cc.ModelConfigs {
		paths.Id("Path" + pluralize.Pluralize(mc.TypeName, 2, false)).Op("=").Lit(
			fmt.Sprintf("/%s/%s", cc.ParsedModelFile.Name, pluralize.Pluralize(strcase.ToKebab(mc.TypeName), 2, false)),
		)
		paths.Line()
	}
	f.Const().Defs(
		objectTypes,
		Line(),
		Id(cc.ControllerDomain+"StreamName").Op("=").Lit(cc.ControllerDomainLower+"Stream"),
		Line(),
		subjects,
		Line(),
		paths,
	)
	f.Line()
	// NATS subject functions by object
	var subjectFuncs []string
	for _, mc := range cc.ModelConfigs {
		funcName := fmt.Sprintf("Get%sSubjects", mc.TypeName)
		subjectFuncs = append(subjectFuncs, funcName)
		subjects := &Statement{}
		subjects.Id(mc.CreateSubject).Op(",")
		subjects.Line()
		subjects.Id(mc.UpdateSubject).Op(",")
		subjects.Line()
		subjects.Id(mc.DeleteSubject).Op(",")
		f.Comment(fmt.Sprintf("%s returns the NATS subjects", funcName))
		f.Comment(fmt.Sprintf("for %s.", pluralize.Pluralize(strcase.ToDelimited(mc.TypeName, ' '), 2, false)))
		f.Func().Id(funcName).Params().Index().String().Block(
			Return(
				Index().String().Block(
					subjects,
				),
			),
		)
		f.Line()
	}
	// all NATS subjects for controller domain
	controllerSubjectsFuncName := fmt.Sprintf("Get%sSubjects", cc.ControllerDomain)
	controllerSubjectsLower := fmt.Sprintf("%sSubjects", cc.ControllerDomainLower)
	subjectAppends := &Statement{}
	for _, sf := range subjectFuncs {
		subjectAppends.Id(controllerSubjectsLower).Op("=").Append(
			Id(controllerSubjectsLower),
			Id(sf).Call().Op("..."),
		)
		subjectAppends.Line()
	}
	f.Comment(fmt.Sprintf("%s returns the NATS subjects", controllerSubjectsFuncName))
	f.Comment(fmt.Sprintf("for all %s objects.", strcase.ToDelimited(cc.ControllerDomain, ' ')))
	f.Func().Id(controllerSubjectsFuncName).Params().Index().String().Block(
		Var().Id(controllerSubjectsLower).Index().String(),
		Line(),
		subjectAppends,
		Line(),
		Return(Id(controllerSubjectsLower)),
	)
	// API object methods
	for _, mc := range cc.ModelConfigs {
		// NotificationPayload method
		f.Comment("NotificationPayload returns the notification payload that is delivered to the")
		f.Comment("controller when a change is made.  It includes the object as presented by the")
		f.Comment("client when the change was made.")
		f.Func().Params(
			Id(sdk.TypeAbbrev(mc.TypeName)).Op("*").Id(mc.TypeName),
		).Id("NotificationPayload").Params(
			Line().Id("operation").Qual(
				"github.com/threeport/threeport/pkg/notifications/v0",
				"NotificationOperation",
			),
			Line().Id("requeue").Bool(),
			Line().Id("creationTime").Int64(),
			Line(),
		).Parens(List(
			Op("*").Index().Byte(),
			Error(),
		)).Block(
			Id("notif").Op(":=").Qual(
				"github.com/threeport/threeport/pkg/notifications/v0",
				"Notification",
			).Values(Dict{
				Id("Operation"):    Id("operation"),
				Id("CreationTime"): Op("&").Id("creationTime"),
				Id("Object"):       Id(sdk.TypeAbbrev(mc.TypeName)),
			}),
			Line(),
			List(
				Id("payload"), Err(),
			).Op(":=").Qual("encoding/json", "Marshal").Call(Id("notif")),
			If(
				Err().Op("!=").Nil(),
			).Block(
				Return(List(
					Op("&").Id("payload"),
					Qual("fmt", "Errorf").Call(
						Lit("failed to marshal notification payload %+v: %w"),
						Id(sdk.TypeAbbrev(mc.TypeName)),
						Err(),
					),
				)),
			),
			Line(),
			Return(
				Op("&").Id("payload"),
				Nil(),
			),
		)
		f.Line()
		// DecodeNotifObject method
		f.Comment("DecodeNotifObject takes the threeport object in the form of a")
		f.Comment("map[string]interface and returns the typed object by marshalling into JSON")
		f.Comment("and then unmarshalling into the typed object.  We are not using the")
		f.Comment("mapstructure library here as that requires custom decode hooks to manage")
		f.Comment("fields with non-native go types.")
		f.Func().Params(
			Id(sdk.TypeAbbrev(mc.TypeName)).Op("*").Id(mc.TypeName),
		).Id("DecodeNotifObject").Params(Id("object").Interface()).Error().Block(
			List(Id("jsonObject"), Id("err")).Op(":=").Qual("encoding/json", "Marshal").Call(Id("object")),
			If(Id("err").Op("!=").Nil()).Block(
				Return(Qual("fmt", "Errorf").Call(
					Lit("failed to marshal object map from consumed notification message: %w"), Id("err")),
				),
			),
			If(Err().Op(":=").Qual("encoding/json", "Unmarshal").Call(
				Id("jsonObject"), Op("&").Id(sdk.TypeAbbrev(mc.TypeName)),
			).Op(";").Id("err").Op("!=").Nil()).Block(
				Return(Qual("fmt", "Errorf").Call(
					Lit("failed to unmarshal json object to typed object: %w"), Id("err"),
				)),
			),
			Return(Nil()),
		)
		// GetID method
		f.Comment("GetID returns the unique ID for the object.")
		f.Func().Params(
			Id(sdk.TypeAbbrev(mc.TypeName)).Op("*").Id(mc.TypeName),
		).Id("GetID").Params().Uint().Block(
			Return(Op("*").Id(sdk.TypeAbbrev(mc.TypeName)).Dot("ID")),
		)
		// String method
		f.Comment("String returns a string representation of the ojbect.")
		f.Func().Params(
			Id(sdk.TypeAbbrev(mc.TypeName)).Id(mc.TypeName),
		).Id("String").Params().String().Block(
			Return(Qual(
				"fmt", "Sprintf",
			).Call(Lit(fmt.Sprintf("%s.%s", cc.PackageName, mc.TypeName))),
			),
		)
	}

	// write code to file
	genFilename := fmt.Sprintf("%s_gen.go", sdk.FilenameSansExt(cc.ModelFilename))
	file, err := os.OpenFile(genFilename, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return fmt.Errorf("failed to open file to write generated code for database models: %w", err)
	}
	defer file.Close()
	if err := f.Render(file); err != nil {
		return fmt.Errorf("failed to render generated source code for database models: %w", err)
	}
	fmt.Printf("code generation complete for %s database models\n", cc.ControllerDomainLower)

	return nil
}