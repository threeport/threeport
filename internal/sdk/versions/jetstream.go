package versions

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	. "github.com/dave/jennifer/jen"
	"github.com/iancoleman/strcase"
	"github.com/threeport/threeport/internal/sdk"
)

// InitJetStreamContext generates the source code to initialize
// the NATS JetStream context and controller streams used for
// controller notifications.
func (gvc *GlobalVersionConfig) InitJetStreamContext(sdkConfig *sdk.ApiObjectConfig) error {
	addStreamCalls := &Statement{}
	addStreamCalls.Line()
	keys := make([]string, 0, len(sdkConfig.ApiObjectGroups))
	groupMap := make(map[string][]*sdk.ApiObject)
	for _, og := range sdkConfig.ApiObjectGroups {
		keys = append(keys, *og.Name)
		groupMap[*og.Name] = og.Objects
	}
	sort.Strings(keys)

	for _, controllerDomain := range keys {

		apiObjects := groupMap[controllerDomain]
		// Determine if any objects within this controller domain need reconcilliation
		needReconcilers := false
		for _, obj := range apiObjects {
			if obj.Reconcilable != nil && *obj.Reconcilable {
				needReconcilers = true
				break
			}
		}

		if needReconcilers {
			streamName := fmt.Sprintf(
				"%sStreamName", strcase.ToCamel(controllerDomain),
			)

			subjectFuncName := fmt.Sprintf(
				"Get%sSubjects", strcase.ToCamel(controllerDomain),
			)

			addStreamCalls.Id("_").Op(",").Id("err").Op("=").Id("js").Dot("AddStream").Call(
				Op("&").Qual("github.com/nats-io/nats.go", "StreamConfig").Values(
					Dict{
						Id("Name"):     Qual("github.com/threeport/threeport/pkg/api/v0", streamName),
						Id("Subjects"): Qual("github.com/threeport/threeport/pkg/api/v0", subjectFuncName).Call(),
					},
				),
			)
			addStreamCalls.Line()
			addStreamCalls.If(Id("err").Op("!=").Nil().Block(
				Return(Nil(),
					Qual("fmt", "Errorf").Call(Lit("could not add stream %s: %w"), Id("v0").Dot(streamName), Err())),
			))
			addStreamCalls.Line()
		}
	}

	f := NewFile("util")
	f.HeaderComment("generated by 'threeport-sdk gen' for nats jetstream boilerplate - do not edit")
	f.ImportAlias("github.com/threeport/threeport/pkg/api/v0", "v0")

	// Import necessary packages
	f.ImportAlias("github.com/nats-io/nats.go", "nats")
	f.ImportAlias("github.com/threeport/threeport/pkg/api/v0", "v0")

	// Add comment for the function
	f.Comment(`Initialize the NATS Jet stream context with controller streams`)
	f.Func().Id("InitJetStream").Params(
		Id("nc").Op("*").Qual("github.com/nats-io/nats.go", "Conn"),
	).Params(
		Op("*").Qual("github.com/nats-io/nats.go", "JetStreamContext"),
		Error(),
	).Block(
		List(Id("js"), Err()).Op(":=").Id("nc").Dot("JetStream").Call(
			Qual("github.com/nats-io/nats.go", "PublishAsyncMaxPending").Call(Lit(256)),
		),
		If(Err().Op("!=").Nil()).Block(
			Return(Nil(),
				Qual("fmt", "Errorf").Call(Lit("failed to create jetstream context: %w"), Err())),
		),
		Comment("add controller streams"),
		addStreamCalls,
		Return(Op("&").Id("js"), Nil()),
	)

	// write code to file
	controllerStreamFilePath := filepath.Join("cmd", "rest-api", "util", "controller_stream_gen.go")
	file, err := os.OpenFile(controllerStreamFilePath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return fmt.Errorf("failed to open file to write generated code for controller streams: %w", err)
	}
	defer file.Close()
	if err := f.Render(file); err != nil {
		return fmt.Errorf("failed to render generated source code for controller streams: %w", err)
	}
	fmt.Println("code generation complete for controller streams")

	return nil
}
