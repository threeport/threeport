package controller

import (
	"fmt"
	"strings"

	. "github.com/dave/jennifer/jen"
)

const (
	supportServicesPath = "github.com/threeport/threeport/pkg/supportservices"
	supportClientPath   = "github.com/threeport/threeport/pkg/client/v0"
)

// supportComponents returns the shared components declared on typeName.
// A component field is a requires relationship to KubernetesWorkloadInstance.
// The result is empty when the type has no KubernetesRuntimeInstanceID.
func supportComponents(
	tags map[string]map[string]map[string]string,
	typeName string,
) []string {
	fields := tags[typeName]
	if _, ok := fields["KubernetesRuntimeInstanceID"]; !ok {
		return nil
	}

	// keep the install order fixed
	order := []struct {
		field string
		name  string
	}{
		{"IngressComponentInstanceID", "ingress"},
		{"CertificatesComponentInstanceID", "certificates"},
		{"SecretsComponentInstanceID", "secrets"},
		{"DatabaseComponentInstanceID", "database"},
	}
	var found []string
	for _, item := range order {
		if requiresWorkload(fields[item.field]["relationship"]) {
			found = append(found, item.name)
		}
	}
	return found
}

// requiresWorkload reports whether the relationship tag requires a workload.
func requiresWorkload(tag string) bool {
	parts := strings.Split(tag, ";")
	if len(parts) == 0 || parts[0] != "requires" {
		return false
	}
	for _, part := range parts[1:] {
		key, value, ok := strings.Cut(part, ":")
		if ok && key == "type" && value == "KubernetesWorkloadInstance" {
			return true
		}
	}
	return false
}

// emitSupportComponents writes the runtime load and one call per component.
// release selects Release over Ensure.
func emitSupportComponents(
	g *Group,
	components []string,
	release bool,
	varObjectName string,
	typeName string,
	version string,
	modulePath string,
) {
	if len(components) == 0 {
		return
	}

	// use the versioned object for the runtime id and the caller
	g.Id("componentObject").Op(":=").Id(varObjectName).Assert(Op("*").Qual(
		fmt.Sprintf("%s/pkg/api/%s", modulePath, version),
		typeName,
	))
	g.If(Id("componentObject").Dot("KubernetesRuntimeInstanceID").Op("!=").Nil()).BlockFunc(func(b *Group) {
		// load the runtime the components install on
		b.List(Id("runtime"), Err()).Op(":=").Qual(
			supportClientPath,
			"GetKubernetesRuntimeInstanceByID",
		).Call(
			Line().Id("r").Dot("APIClient"),
			Line().Id("r").Dot("APIServer"),
			Line().Op("*").Id("componentObject").Dot("KubernetesRuntimeInstanceID"),
			Line(),
		)
		b.If(Err().Op("!=").Nil()).Block(
			Id("operationErr").Op("=").Qual("fmt", "Errorf").Call(
				Lit("failed to get kubernetes runtime instance: %w"),
				Err(),
			),
		).Else().BlockFunc(func(e *Group) {
			for _, component := range components {
				emitComponentCall(e, component, release, typeName, version, modulePath)
			}
		})
	})
}

// emitComponentCall writes one ensure or release call.
// A failed call sets operationErr and later components are skipped.
func emitComponentCall(
	g *Group,
	component string,
	release bool,
	typeName string,
	version string,
	modulePath string,
) {
	g.If(Id("operationErr").Op("==").Nil()).BlockFunc(func(b *Group) {
		switch component {
		case "secrets":
			if release {
				assignComponentErr(
					b,
					Qual(supportServicesPath, "ReleaseSecretsComponent").Call(
						Line().Id("r"),
						Line().Id("runtime"),
						Line().Id("componentObject"),
						Line(),
					),
					"failed to release secrets component",
				)
				return
			}

			// secrets takes the runtime's infra provider
			b.List(Id("provider"), Err()).Op(":=").Qual(
				supportClientPath,
				"GetInfraProviderByKubernetesRuntimeInstanceID",
			).Call(
				Line().Id("r").Dot("APIClient"),
				Line().Id("r").Dot("APIServer"),
				Line().Id("runtime").Dot("ID"),
				Line(),
			)
			b.If(Err().Op("!=").Nil()).Block(
				Id("operationErr").Op("=").Qual("fmt", "Errorf").Call(
					Lit("failed to get infra provider: %w"),
					Err(),
				),
			).Else().BlockFunc(func(s *Group) {
				assignComponentErr(
					s,
					Qual(supportServicesPath, "EnsureSecretsComponent").Call(
						Line().Id("r"),
						Line().Id("runtime"),
						Line().Id("componentObject"),
						Line().Op("&").Id("componentObject").Dot("SecretsComponentInstanceID"),
						Line().Qual(modulePath+"/pkg/client/"+version, "Update"+typeName),
						Line().Id("provider"),
						Line().Op("&").Id("log"),
						Line(),
					),
					"failed to ensure secrets component",
				)
			})
		default:
			name := "Ensure" + exportComponent(component) + "Component"
			message := "failed to ensure " + component + " component"
			args := []Code{
				Line().Id("r"),
				Line().Id("runtime"),
				Line().Id("componentObject"),
			}
			if !release {
				args = append(
					args,
					Line().Op("&").Id("componentObject").Dot(exportComponent(component)+"ComponentInstanceID"),
					Line().Qual(
						fmt.Sprintf("%s/pkg/client/%s", modulePath, version),
						"Update"+typeName,
					),
					Line().Op("&").Id("log"),
				)
			}
			if release {
				name = "Release" + exportComponent(component) + "Component"
				message = "failed to release " + component + " component"
			}
			args = append(args, Line())
			assignComponentErr(b, Qual(supportServicesPath, name).Call(args...), message)
		}
	})
}

// assignComponentErr stores a component call error on operationErr.
func assignComponentErr(g *Group, call *Statement, message string) {
	g.If(
		Err().Op(":=").Add(call),
		Err().Op("!=").Nil(),
	).Block(
		Id("operationErr").Op("=").Qual("fmt", "Errorf").Call(
			Lit(message+": %w"),
			Err(),
		),
	)
}

// exportComponent returns the function-name token for a component.
func exportComponent(component string) string {
	// map the component to its function token
	switch component {
	case "ingress":
		return "Ingress"
	case "certificates":
		return "Certificates"
	case "database":
		return "Database"
	default:
		return ""
	}
}
