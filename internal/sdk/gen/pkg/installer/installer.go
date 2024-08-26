package installer

import (
	"fmt"
	"path/filepath"

	. "github.com/dave/jennifer/jen"
	"github.com/iancoleman/strcase"

	"github.com/threeport/threeport/internal/sdk"
	"github.com/threeport/threeport/internal/sdk/gen"
	"github.com/threeport/threeport/internal/sdk/util"
	cli "github.com/threeport/threeport/pkg/cli/v0"
)

// GenInstaller generates the installer package for extension projects that
// installs the extension components alongside an existing Threeport control
// plane and registers that extension with Threeport.
func GenInstaller(gen *gen.Generator, sdkConfig *sdk.SdkConfig) error {
	f := NewFile("v0")
	f.HeaderComment("generated by 'threeport-sdk gen' - do not edit")

	f.ImportAlias("github.com/threeport/threeport/pkg/kube/v0", "kube")
	f.ImportAlias("github.com/threeport/threeport/pkg/api-server/v0/database", "tp_database")
	f.ImportAlias("k8s.io/apimachinery/pkg/apis/meta/v1", "metav1")

	extensionNameKebab := strcase.ToKebab(sdkConfig.ExtensionName)
	extensionNameSnake := strcase.ToSnake(sdkConfig.ExtensionName)
	extensionNameCamel := strcase.ToCamel(sdkConfig.ExtensionName)
	extensionNameLowerCamel := strcase.ToLowerCamel(sdkConfig.ExtensionName)

	f.Const().Defs(
		Id("defaultNamespace").Op("=").Lit(fmt.Sprintf(
			"threeport-%s",
			extensionNameKebab,
		)),
		Id("defaultThreeportNamespace").Op("=").Lit("threeport-control-plane"),
		Id("natsLabelSelector").Op("=").Lit("app.kubernetes.io/name=nats"),
	)

	f.Comment("Installer contains the values needed for an extension installation.")
	f.Type().Id("Installer").Struct(
		Comment("dynamice interface client for Kubernetes API"),
		Id("KubeClient").Qual("k8s.io/client-go/dynamic", "Interface"),

		Line().Comment("Kubernetes API REST mapper"),
		Id("KubeRestMapper").Op("*").Qual("k8s.io/apimachinery/pkg/api/meta", "RESTMapper"),

		Line().Comment("The Kubernetes namespace to install the extension components in."),
		Id("ExtensionNamespace").String(),

		Line().Comment("The Kubernetes namespace the Threeport control plane is installed in."),
		Id("ThreeportNamespace").String(),
	)

	f.Comment(fmt.Sprintf(
		"NewInstaller returns a %s extension installer with default values.",
		extensionNameKebab,
	))
	f.Func().Id("NewInstaller").Params(
		Line().Id("kubeClient").Qual("k8s.io/client-go/dynamic", "Interface"),
		Line().Id("restMapper").Op("*").Qual("k8s.io/apimachinery/pkg/api/meta", "RESTMapper"),
		Line(),
	).Op("*").Id("Installer").Block(
		Id("defaultInstaller").Op(":=").Id("Installer").Values(Dict{
			Id("KubeClient"):         Id("kubeClient"),
			Id("KubeRestMapper"):     Id("restMapper"),
			Id("ExtensionNamespace"): Id("defaultNamespace"),
			Id("ThreeportNamespace"): Id("defaultThreeportNamespace"),
		}),
		Line(),

		Return(Op("&").Id("defaultInstaller")),
	)

	installFuncName := fmt.Sprintf("Install%sExtension", extensionNameCamel)
	f.Comment(fmt.Sprintf(
		"%s installs the controller and API for the %s extension.",
		installFuncName,
		extensionNameKebab,
	))
	f.Func().Params(
		Id("i").Op("*").Id("Installer"),
	).Id(installFuncName).Params().Error().BlockFunc(func(g *Group) {

		g.Comment("get NATS service name from cluster")
		g.Id("gvr").Op(":=").Qual(
			"k8s.io/apimachinery/pkg/runtime/schema",
			"GroupVersionResource",
		).Values(Dict{
			Id("Group"):    Lit(""),
			Id("Version"):  Lit("v1"),
			Id("Resource"): Lit("services"),
		})
		g.List(Id("services"), Err()).Op(":=").Id("i").Dot("KubeClient").Dot("Resource").Call(
			Id("gvr"),
		).Dot("Namespace").Call(
			Id("i").Dot("ThreeportNamespace"),
		).Dot("List").Call(
			Qual("context", "TODO").Call(),
			Qual("k8s.io/apimachinery/pkg/apis/meta/v1", "ListOptions").Values(Dict{
				Line().Id("LabelSelector"): Id("natsLabelSelector"),
			}).Op(",").Line(),
		)
		g.If(Err().Op("!=").Nil()).Block(
			Return(Qual("fmt", "Errorf").Call(Lit("failed to retrieve NATS service name: %w"), Err())),
		)
		g.If(Len(Id("services").Dot("Items")).Op("!=").Lit(1)).Block(
			Return(Qual("fmt", "Errorf").Call(
				Line().Lit("expected one NATS service with label '%s' but found %d"),
				Line().Id("natsLabelSelector"),
				Line().Len(Id("services").Dot("Items")),
				Line(),
			)),
		)
		g.Id("natsServiceName").Op(":=").Id("services").Dot("Items").Index(Lit(0)).Dot("GetName").Call()
		g.Line()

		g.Comment("create namespace")
		g.Var().Id("namespace").Op("=").Op("&").Qual(
			"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured",
			"Unstructured",
		).Values(Dict{
			Line().Id("Object"): Map(String()).Interface().Values(Dict{
				Lit("apiVersion"): Lit("v1"),
				Lit("kind"):       Lit("Namespace"),
				Lit("metadata"): Map(String()).Interface().Values(Dict{
					Line().Lit("name"): Id("i.ExtensionNamespace").Op(",").Line(),
				}),
			}).Op(",").Line(),
		})
		g.Line()

		g.If(List(Id("_"), Err()).Op(":=").Qual(
			"github.com/threeport/threeport/pkg/kube/v0",
			"CreateOrUpdateResource",
		).Call(
			Line().Id("namespace"),
			Line().Id("i.KubeClient"),
			Line().Op("*").Id("i.KubeRestMapper"),
			Line(),
		), Err().Op("!=").Nil()).Block(
			Return(Qual("fmt", "Errorf").Call(
				Lit(fmt.Sprintf(
					"failed to create/update %s extension namespace: %%w",
					extensionNameKebab,
				)),
				Err(),
			)),
		)
		g.Line()

		g.Comment("copy secrets into extension namespace")
		copySecrets := []string{
			"db-root-cert",
			"db-threeport-cert",
			"encryption-key",
			"controller-config",
		}
		for _, secretName := range copySecrets {
			g.If(Err().Op(":=").Id("copySecret").Call(
				Line().Id("i.KubeClient"),
				Line().Op("*").Id("i.KubeRestMapper"),
				Line().Lit(secretName),
				Line().Id("i").Dot("ThreeportNamespace"),
				Line().Id("i").Dot("ExtensionNamespace"),
				Line(),
			).Op(";").Err().Op("!=").Nil()).Block(
				Return(Qual("fmt", "Errorf").Call(
					Lit("failed to copy secret: %w"),
					Err(),
				)),
			)
			g.Line()
		}
		g.Line()

		g.Comment("create secret for database connection")
		g.Var().Id("apiSecret").Op("=").Op("&").Qual(
			"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured",
			"Unstructured",
		).Values(Dict{
			Line().Id("Object"): Map(String()).Interface().Values(Dict{
				Lit("apiVersion"): Lit("v1"),
				Lit("kind"):       Lit("Secret"),
				Lit("metadata"): Map(String()).Interface().Values(Dict{
					Lit("name"):      Lit("db-config"),
					Lit("namespace"): Id("i").Dot("ExtensionNamespace"),
				}),
				Lit("stringData"): Map(String()).Interface().Values(Dict{
					Line().Lit("env"): Qual("fmt", "Sprintf").Call(
						Line().Lit(`DB_HOST=%s.%s.svc.cluster.local
DB_USER=%s
DB_NAME=%s
DB_PORT=%s
DB_SSL_MODE=%s
NATS_HOST=%s.%s.svc.cluster.local
NATS_PORT=4222
`),
						Line().Qual(
							"github.com/threeport/threeport/pkg/api-server/v0/database",
							"ThreeportDatabaseHost",
						),
						Line().Id("i").Dot("ThreeportNamespace"),
						Line().Qual(
							"github.com/threeport/threeport/pkg/api-server/v0/database",
							"ThreeportDatabaseUser",
						),
						Line().Qual(
							fmt.Sprintf(
								"%s/pkg/api-server/v0/database",
								gen.ModulePath,
							),
							fmt.Sprintf(
								"Threeport%sDatabaseName",
								extensionNameCamel,
							),
						),
						Line().Qual(
							"github.com/threeport/threeport/pkg/api-server/v0/database",
							"ThreeportDatabasePort",
						),
						Line().Qual(
							"github.com/threeport/threeport/pkg/api-server/v0/database",
							"ThreeportDatabaseSslMode",
						),
						Line().Id("natsServiceName"),
						Line().Id("i").Dot("ThreeportNamespace"),
						Line(),
					).Op(",").Line(),
				}),
			}).Op(",").Line(),
		})
		g.If(List(Id("_"), Err()).Op(":=").Qual(
			"github.com/threeport/threeport/pkg/kube/v0",
			"CreateOrUpdateResource",
		).Call(
			Id("apiSecret"),
			Id("i").Dot("KubeClient"),
			Op("*").Id("i").Dot("KubeRestMapper"),
		), Err().Op("!=").Nil()).Block(
			Return(Qual("fmt", "Errorf").Call(
				Lit("failed to create/update API server secret for DB connection: %w"),
				Err(),
			)),
		)
		g.Line()

		extensionDbName := fmt.Sprintf("threeport_%s_api", extensionNameSnake)
		g.Comment("create configmap used to initialize API database")
		g.Var().Id("dbCreateConfig").Op("=").Op("&").Qual(
			"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured",
			"Unstructured",
		).Values(Dict{
			Id("Object"): Map(String()).Interface().Values(Dict{
				Lit("apiVersion"): Lit("v1"),
				Lit("kind"):       Lit("ConfigMap"),
				Lit("metadata"): Map(String()).Interface().Values(Dict{
					Lit("name"):      Lit("db-create"),
					Lit("namespace"): Id("i.ExtensionNamespace"),
				}),
				Lit("data"): Map(String()).Interface().Values(Dict{
					Line().Lit("db.sql"): Lit(fmt.Sprintf(`CREATE USER IF NOT EXISTS threeport;
CREATE DATABASE IF NOT EXISTS %[1]s encoding='utf-8';
GRANT ALL ON DATABASE %[1]s TO threeport;`, extensionDbName)).Op(",").Line(),
				}),
			}),
		})
		g.Line()

		g.If(List(Id("_"), Err()).Op(":=").Qual(
			"github.com/threeport/threeport/pkg/kube/v0",
			"CreateOrUpdateResource",
		).Call(
			Id("dbCreateConfig"),
			Id("i.KubeClient"),
			Op("*").Id("i.KubeRestMapper"),
		), Err().Op("!=").Nil()).Block(
			Return(Qual("fmt", "Errorf").Call(
				Lit(fmt.Sprintf(
					"failed to create/update %s DB initialization configmap: %%w",
					extensionNameKebab,
				)),
				Err(),
			)),
		)
		g.Line()

		g.Comment(fmt.Sprintf(
			"install %s API server",
			extensionNameKebab,
		))
		g.Var().Id(fmt.Sprintf(
			"%sApiDeploy",
			extensionNameLowerCamel,
		)).Op("=").Op("&").Qual(
			"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured",
			"Unstructured",
		).Values(Dict{
			Line().Id("Object"): Map(String()).Interface().Values(Dict{
				Lit("apiVersion"): Lit("apps/v1"),
				Lit("kind"):       Lit("Deployment"),
				Lit("metadata"): Map(String()).Interface().Values(Dict{
					Lit("name"): Lit(fmt.Sprintf(
						"threeport-%s-api-server",
						extensionNameKebab,
					)),
					Lit("namespace"): Id("i.ExtensionNamespace"),
				}),
				Lit("spec"): Map(String()).Interface().Values(Dict{
					Lit("replicas"): Lit(1),
					Lit("selector"): Map(String()).Interface().Values(Dict{
						Line().Lit("matchLabels"): Map(String()).Interface().Values(Dict{
							Line().Lit("app.kubernetes.io/name"): Lit(fmt.Sprintf(
								"threeport-%s-api-server",
								extensionNameKebab,
							)).Op(",").Line(),
						}).Op(",").Line(),
					}),
					Lit("strategy"): Map(String()).Interface().Values(Dict{
						Lit("rollingUpdate"): Map(String()).Interface().Values(Dict{
							Lit("maxSurge"):       Lit("25%"),
							Lit("maxUnavailable"): Lit("25%"),
						}),
						Lit("type"): Lit("RollingUpdate"),
					}),
					Lit("template"): Map(String()).Interface().Values(Dict{
						Lit("metadata"): Map(String()).Interface().Values(Dict{
							Lit("creationTimestamp"): Nil(),
							Lit("labels"): Map(String()).Interface().Values(Dict{
								Line().Lit("app.kubernetes.io/name"): Lit(fmt.Sprintf(
									"threeport-%s-api-server",
									extensionNameKebab,
								)).Op(",").Line(),
							}),
						}),
						Lit("spec"): Map(String()).Interface().Values(Dict{
							Lit("containers"): Index().Interface().Values(
								Line().Map(String()).Interface().Values(Dict{
									Lit("args"): Index().Interface().Values(
										Line().Lit("-auto-migrate=true"),
										Line().Lit("-auth-enabled=false"),
										Line(),
									),
									Lit("command"): Index().Interface().Values(
										Line().Lit("/rest-api"),
										Line(),
									),
									Lit("envFrom"): Index().Interface().Values(
										Line().Map(String()).Interface().Values(Dict{
											Line().Lit("secretRef"): Map(String()).Interface().Values(Dict{
												Line().Lit("name"): Lit("encryption-key").Op(",").Line(),
											}).Op(",").Line(),
										}).Op(",").Line(),
									),
									Lit("image"): Lit(fmt.Sprintf(
										"localhost:5001/threeport-%s-rest-api:dev",
										extensionNameKebab,
									)),
									Lit("imagePullPolicy"): Lit("IfNotPresent"),
									Lit("name"):            Lit("api-server"),
									Lit("ports"): Index().Interface().Values(
										Line().Map(String()).Interface().Values(Dict{
											Lit("containerPort"): Lit(1323),
											Lit("name"):          Lit("api"),
											Lit("protocol"):      Lit("TCP"),
										}).Op(",").Line(),
									),
									Lit("readinessProbe"): Map(String()).Interface().Values(Dict{
										Lit("failureThreshold"): Lit(1),
										Lit("httpGet"): Map(String()).Interface().Values(Dict{
											Lit("path"):   Lit("/readyz"),
											Lit("port"):   Lit(8081),
											Lit("scheme"): Lit("HTTP"),
										}),
										Lit("initialDelaySeconds"): Lit(1),
										Lit("periodSeconds"):       Lit(2),
										Lit("successThreshold"):    Lit(1),
										Lit("timeoutSeconds"):      Lit(1),
									}),
									Lit("volumeMounts"): Index().Interface().Values(
										Line().Map(String()).Interface().Values(Dict{
											Lit("mountPath"): Lit("/etc/threeport/"),
											Lit("name"):      Lit("db-config"),
										}),
										Line().Map(String()).Interface().Values(Dict{
											Lit("mountPath"): Lit("/etc/threeport/db-certs"),
											Lit("name"):      Lit("db-threeport-cert"),
										}).Op(",").Line(),
									),
								}).Op(",").Line(),
							),
							Lit("initContainers"): Index().Interface().Values(
								Line().Map(String()).Interface().Values(Dict{
									Lit("command"): Index().Interface().Values(
										Line().Lit("bash"),
										Line().Lit("-c"),
										Line().Qual("fmt", "Sprintf").Call(
											Lit("cockroach sql --certs-dir=/etc/threeport/db-certs --host crdb.%s.svc.cluster.local --port 26257 -f /etc/threeport/db-create/db.sql"),
											Id("i").Dot("ThreeportNamespace"),
										).Op(",").Line()),
									Lit("image"):           Lit("cockroachdb/cockroach:v23.1.14"),
									Lit("imagePullPolicy"): Lit("IfNotPresent"),
									Lit("name"):            Lit("db-init"),
									Lit("volumeMounts"): Index().Interface().Values(
										Line().Map(String()).Interface().Values(Dict{
											Lit("mountPath"): Lit("/etc/threeport/db-create"),
											Lit("name"):      Lit("db-create"),
										}),
										Line().Map(String()).Interface().Values(Dict{
											Lit("mountPath"): Lit("/etc/threeport/db-certs"),
											Lit("name"):      Lit("db-root-cert"),
										}).Op(",").Line(),
									),
								}),
								Line().Map(String()).Interface().Values(Dict{
									Lit("args"): Index().Interface().Values(
										Line().Lit("-env-file=/etc/threeport/env"),
										Line().Lit("up"),
										Line(),
									),
									Lit("command"): Index().Interface().Values(
										Line().Lit("/database-migrator"),
										Line(),
									),
									Lit("image"): Lit(fmt.Sprintf(
										"localhost:5001/threeport-%s-database-migrator:dev",
										extensionNameKebab,
									)),
									Lit("imagePullPolicy"): Lit("IfNotPresent"),
									Lit("name"):            Lit("database-migrator"),
									Lit("volumeMounts"): Index().Interface().Values(
										Line().Map(String()).Interface().Values(Dict{
											Lit("mountPath"): Lit("/etc/threeport/"),
											Lit("name"):      Lit("db-config"),
										}),
										Line().Map(String()).Interface().Values(Dict{
											Lit("mountPath"): Lit("/etc/threeport/db-certs"),
											Lit("name"):      Lit("db-threeport-cert"),
										}).Op(",").Line(),
									),
								}).Op(",").Line(),
							),
							Lit("restartPolicy"):                 Lit("Always"),
							Lit("terminationGracePeriodSeconds"): Lit(30),
							Lit("volumes"): Index().Interface().Values(
								Line().Map(String()).Interface().Values(Dict{
									Lit("name"): Lit("db-root-cert"),
									Lit("secret"): Map(String()).Interface().Values(Dict{
										Lit("defaultMode"): Lit(420),
										Lit("secretName"):  Lit("db-root-cert"),
									}),
								}),
								Line().Map(String()).Interface().Values(Dict{
									Lit("name"): Lit("db-threeport-cert"),
									Lit("secret"): Map(String()).Interface().Values(Dict{
										Lit("defaultMode"): Lit(420),
										Lit("secretName"):  Lit("db-threeport-cert"),
									}),
								}),
								Line().Map(String()).Interface().Values(Dict{
									Lit("name"): Lit("db-config"),
									Lit("secret"): Map(String()).Interface().Values(Dict{
										Lit("defaultMode"): Lit(420),
										Lit("secretName"):  Lit("db-config"),
									}),
								}),
								Line().Map(String()).Interface().Values(Dict{
									Lit("configMap"): Map(String()).Interface().Values(Dict{
										Lit("defaultMode"): Lit(420),
										Lit("name"):        Lit("db-create"),
									}),
									Lit("name"): Lit("db-create"),
								}).Op(",").Line(),
							),
						}),
					}),
				}),
			}).Op(",").Line(),
		})
		g.Line()

		g.If(List(Id("_"), Err()).Op(":=").Qual(
			"github.com/threeport/threeport/pkg/kube/v0",
			"CreateOrUpdateResource",
		).Call(
			Id(fmt.Sprintf(
				"%sApiDeploy",
				extensionNameLowerCamel,
			)),
			Id("i.KubeClient"),
			Op("*").Id("i.KubeRestMapper"),
		), Err().Op("!=").Nil()).Block(
			Return(Qual("fmt", "Errorf").Call(
				Lit(fmt.Sprintf(
					"failed to create/update %s API deployment: %%w",
					extensionNameKebab,
				)),
				Err(),
			)),
		)
		g.Line()

		g.Comment(fmt.Sprintf(
			"install %s controller",
			extensionNameKebab,
		))
		g.Var().Id(fmt.Sprintf(
			"%sControllerDeploy",
			extensionNameLowerCamel,
		)).Op("=").Op("&").Qual(
			"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured",
			"Unstructured",
		).Values(Dict{
			Line().Id("Object"): Map(String()).Interface().Values(Dict{
				Lit("apiVersion"): Lit("apps/v1"),
				Lit("kind"):       Lit("Deployment"),
				Lit("metadata"): Map(String()).Interface().Values(Dict{
					Lit("name"): Lit(fmt.Sprintf(
						"threeport-%s-controller",
						extensionNameKebab,
					)),
					Lit("namespace"): Id("i.ExtensionNamespace"),
				}),
				Lit("spec"): Map(String()).Interface().Values(Dict{
					Lit("replicas"): Lit(1),
					Lit("selector"): Map(String()).Interface().Values(Dict{
						Line().Lit("matchLabels"): Map(String()).Interface().Values(Dict{
							Line().Lit("app.kubernetes.io/name"): Lit(fmt.Sprintf(
								"threeport-%s-controller",
								extensionNameKebab,
							)).Op(",").Line(),
						}).Op(",").Line(),
					}),
					Lit("strategy"): Map(String()).Interface().Values(Dict{
						Lit("rollingUpdate"): Map(String()).Interface().Values(Dict{
							Lit("maxSurge"):       Lit("25%"),
							Lit("maxUnavailable"): Lit("25%"),
						}),
						Lit("type"): Lit("RollingUpdate"),
					}),
					Lit("template"): Map(String()).Interface().Values(Dict{
						Lit("metadata"): Map(String()).Interface().Values(Dict{
							Line().Lit("labels"): Map(String()).Interface().Values(Dict{
								Line().Lit("app.kubernetes.io/name"): Lit(fmt.Sprintf(
									"threeport-%s-controller",
									extensionNameKebab,
								)).Op(",").Line(),
							}).Op(",").Line(),
						}),
						Lit("spec"): Map(String()).Interface().Values(Dict{
							Lit("containers"): Index().Interface().Values(
								Line().Map(String()).Interface().Values(Dict{
									Lit("args"): Index().Interface().Values(
										Line().Lit("-auth-enabled=false"),
										Line(),
									),
									Lit("command"): Index().Interface().Values(
										Line().Lit(fmt.Sprintf(
											"/%s-controller",
											extensionNameKebab,
										)),
										Line(),
									),
									Lit("envFrom"): Index().Interface().Values(
										Line().Map(String()).Interface().Values(Dict{
											Line().Lit("secretRef"): Map(String()).Interface().Values(Dict{
												Line().Lit("name"): Lit("controller-config").Op(",").Line(),
											}).Op(",").Line(),
										}),
										Line().Map(String()).Interface().Values(Dict{
											Line().Lit("secretRef"): Map(String()).Interface().Values(Dict{
												Line().Lit("name"): Lit("encryption-key").Op(",").Line(),
											}).Op(",").Line(),
										}).Op(",").Line(),
									),
									Lit("image"): Lit(fmt.Sprintf(
										"localhost:5001/threeport-%s-controller:dev",
										extensionNameKebab,
									)),
									Lit("imagePullPolicy"): Lit("IfNotPresent"),
									Lit("name"): Lit(fmt.Sprintf(
										"%s-controller",
										extensionNameKebab,
									)),
									Lit("readinessProbe"): Map(String()).Interface().Values(Dict{
										Lit("failureThreshold"): Lit(1),
										Lit("httpGet"): Map(String()).Interface().Values(Dict{
											Lit("path"):   Lit("/readyz"),
											Lit("port"):   Lit(8081),
											Lit("scheme"): Lit("HTTP"),
										}),
										Lit("initialDelaySeconds"): Lit(1),
										Lit("periodSeconds"):       Lit(2),
										Lit("successThreshold"):    Lit(1),
										Lit("timeoutSeconds"):      Lit(1),
									}),
								}).Op(",").Line(),
							),
							Lit("restartPolicy"):                 Lit("Always"),
							Lit("terminationGracePeriodSeconds"): Lit(30),
						}),
					}),
				}),
			}).Op(",").Line(),
		})
		g.Line()

		g.If(List(Id("_"), Err()).Op(":=").Qual(
			"github.com/threeport/threeport/pkg/kube/v0",
			"CreateOrUpdateResource",
		).Call(
			Id(fmt.Sprintf(
				"%sControllerDeploy",
				extensionNameLowerCamel,
			)),
			Id("i.KubeClient"),
			Op("*").Id("i.KubeRestMapper"),
		), Err().Op("!=").Nil()).Block(
			Return(Qual("fmt", "Errorf").Call(
				Lit(fmt.Sprintf(
					"failed to create/update %s controller deployment: %%w",
					extensionNameKebab,
				)),
				Err(),
			)),
		)
		g.Line()

		g.Return(Nil())
	})
	f.Line()

	f.Comment("copySecret copies a secret from one namespace to another.  The function")
	f.Comment("returns without error if the secret already exists in the target namespace.")
	f.Func().Id("copySecret").Params(
		Line().Id("dynamicClient").Qual("k8s.io/client-go/dynamic", "Interface"),
		Line().Id("restMapper").Qual("k8s.io/apimachinery/pkg/api/meta", "RESTMapper"),
		Line().Id("secretName").String(),
		Line().Id("sourceNamespace").String(),
		Line().Id("targetNamespace").String(),
		Line(),
	).Params(
		Error(),
	).Block(
		Id("secretGVR").Op(":=").Qual("k8s.io/apimachinery/pkg/runtime/schema", "GroupVersionResource").Values(Dict{
			Id("Group"):    Lit(""),
			Id("Version"):  Lit("v1"),
			Id("Resource"): Lit("secrets"),
		}),
		Id("secretGK").Op(":=").Qual("k8s.io/apimachinery/pkg/runtime/schema", "GroupKind").Values(Dict{
			Id("Group"): Lit(""),
			Id("Kind"):  Lit("Secret"),
		}),
		Line(),

		List(Id("mapping"), Err()).Op(":=").Id("restMapper").Dot("RESTMapping").Call(
			Id("secretGK"),
			Id("secretGVR").Dot("Version"),
		),
		If(Err().Op("!=").Nil()).Block(
			Return(Qual("fmt", "Errorf").Call(Lit("failed to get RESTMapping for Secret resource: %w"), Err())),
		),
		Line(),

		Id("targetSecretResource").Op(":=").Id("dynamicClient").Dot("Resource").Call(
			Id("mapping").Dot("Resource"),
		).Dot("Namespace").Call(Id("targetNamespace")),
		List(Id("_"), Err()).Op("=").Id("targetSecretResource").Dot("Get").Call(
			Qual("context", "TODO").Call(),
			Id("secretName"),
			Qual("k8s.io/apimachinery/pkg/apis/meta/v1",
				"GetOptions").Values(),
		),
		If(Err().Op("==").Nil()).Block(
			Comment("secret already exists, return nil"),
			Return(Nil()),
		).Else().If(Op("!").Qual("k8s.io/apimachinery/pkg/api/errors", "IsNotFound").Call(Err())).Block(
			Return(Qual("fmt", "Errorf").Call(
				Line().Lit("failed to check if Secret '%s' exists in namespace '%s': %w"),
				Line().Id("secretName"),
				Line().Id("targetNamespace"),
				Line().Err(),
				Line(),
			)),
		),
		Line(),

		Id("secretResource").Op(":=").Id("dynamicClient").Dot("Resource").Call(
			Id("mapping").Dot("Resource"),
		).Dot("Namespace").Call(Id("sourceNamespace")),
		List(Id("secret"), Err()).Op(":=").Id("secretResource").Dot("Get").Call(
			Qual("context", "TODO").Call(),
			Id("secretName"),
			Qual("k8s.io/apimachinery/pkg/apis/meta/v1", "GetOptions").Values(),
		),
		If(Err().Op("!=").Nil()).Block(
			Return(Qual("fmt", "Errorf").Call(
				Line().Lit("failed to get Secret '%s' from namespace '%s': %w"),
				Line().Id("secretName"),
				Line().Id("sourceNamespace"),
				Line().Err(),
				Line(),
			)),
		),
		Line(),

		Id("secret").Dot("SetNamespace").Call(Id("targetNamespace")),
		Id("secret").Dot("SetResourceVersion").Call(Lit("")),
		Id("secret").Dot("SetUID").Call(Lit("")),
		Id("secret").Dot("SetSelfLink").Call(Lit("")),
		Id("secret").Dot("SetCreationTimestamp").Call(Qual("k8s.io/apimachinery/pkg/apis/meta/v1", "Time").Values()),
		Id("secret").Dot("SetManagedFields").Call(Nil()),
		Line(),

		List(Id("_"), Err()).Op("=").Id("targetSecretResource").Dot("Create").Call(
			Qual("context", "TODO").Call(),
			Id("secret"),
			Qual("k8s.io/apimachinery/pkg/apis/meta/v1", "CreateOptions").Values(),
		),
		If(Err().Op("!=").Nil()).Block(
			Return(Qual("fmt", "Errorf").Call(
				Lit("failed to create/update Secret in namespace '%s': %w"),
				Id("targetNamespace"),
				Err(),
			)),
		),
		Line(),

		Return(Nil()),
	)

	// write code to file
	genFilepath := filepath.Join(
		"pkg",
		"installer",
		"v0",
		"installer_gen.go",
	)
	_, err := util.WriteCodeToFile(f, genFilepath, true)
	if err != nil {
		return fmt.Errorf("failed to write generated code to file %s: %w", genFilepath, err)
	}
	cli.Info(fmt.Sprintf("source code for installer package written to %s", genFilepath))

	return nil
}
