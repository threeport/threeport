package controller

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	. "github.com/dave/jennifer/jen"
	"github.com/gertd/go-pluralize"
	"github.com/iancoleman/strcase"
)

func (cc *ControllerConfig) Notifications() error {
	pluralize := pluralize.NewClient()
	f := NewFile("notif")

	subjects := &Statement{}
	var subjectFuncs []string
	for _, object := range cc.ReconciledObjects {
		createSubject := object.Name + "CreateSubject"
		updateSubject := object.Name + "UpdateSubject"
		deleteSubject := object.Name + "DeleteSubject"
		subjects.Id(object.Name + "Subject").Op("=").Lit(strcase.ToLowerCamel(object.Name) + ".*")
		subjects.Line()
		subjects.Id(createSubject).Op("=").Lit(strcase.ToLowerCamel(object.Name) + ".create")
		subjects.Line()
		subjects.Id(updateSubject).Op("=").Lit(strcase.ToLowerCamel(object.Name) + ".update")
		subjects.Line()
		subjects.Id(deleteSubject).Op("=").Lit(strcase.ToLowerCamel(object.Name) + ".delete")
		subjects.Line()
		subjects.Line()
		funcName := fmt.Sprintf("Get%sSubjects", object.Name)
		subjectFuncs = append(subjectFuncs, funcName)
		subjects := &Statement{}
		subjects.Id(createSubject).Op(",")
		subjects.Line()
		subjects.Id(updateSubject).Op(",")
		subjects.Line()
		subjects.Id(deleteSubject).Op(",")
		f.Comment(fmt.Sprintf("%s returns the NATS subjects", funcName))
		f.Comment(fmt.Sprintf("for %s.", pluralize.Pluralize(strcase.ToDelimited(object.Name, ' '), 2, false)))
		f.Func().Id(funcName).Params().Index().String().Block(
			Return(
				Index().String().Block(
					subjects,
				),
			),
		)
		f.Line()
	}
	f.Const().Defs(
		Id(cc.StreamName).Op("=").Lit(cc.ShortName+"Stream"),
		Line(),
		subjects,
		Line(),
	)
	f.Line()

	// all NATS subjects for controller domain
	controllerSubjectsFuncName := fmt.Sprintf("Get%sSubjects", strcase.ToCamel(cc.ShortName))
	controllerSubjectsLower := fmt.Sprintf("%sSubjects", strcase.ToLowerCamel(cc.ShortName))
	subjectAppends := &Statement{}
	for _, sf := range subjectFuncs {
		subjectAppends.Id(controllerSubjectsLower).Op("=").Append(
			Id(controllerSubjectsLower),
			Id(sf).Call().Op("..."),
		)
		subjectAppends.Line()
	}
	f.Comment(fmt.Sprintf("%s returns the NATS subjects", controllerSubjectsFuncName))
	f.Comment(fmt.Sprintf("for all %s objects.", strcase.ToDelimited(cc.ShortName, ' ')))
	f.Func().Id(controllerSubjectsFuncName).Params().Index().String().Block(
		Var().Id(controllerSubjectsLower).Index().String(),
		Line(),
		subjectAppends,
		Line(),
		Return(Id(controllerSubjectsLower)),
	)

	// create directory if needed
	notifPath := filepath.Join("internal", cc.ShortName, "notif")
	if _, err := os.Stat(notifPath); errors.Is(err, os.ErrNotExist) {
		if err := os.MkdirAll(notifPath, 0755); err != nil {
			return fmt.Errorf("could not create cmd directories for controller notifcation subjects: %w", err)
		}
	}

	// write code to file
	filepath := filepath.Join(notifPath, "notif_gen.go")
	file, err := os.OpenFile(filepath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return fmt.Errorf("failed to open file to write generated code for %s controller notifications: %w", cc.ShortName, err)
	}
	defer file.Close()
	if err := f.Render(file); err != nil {
		return fmt.Errorf("failed to render generated source code for %s controller notifications: %w", cc.ShortName, err)
	}
	fmt.Printf("code generation complete for %s controller notifications\n", cc.ShortName)

	return nil
}
