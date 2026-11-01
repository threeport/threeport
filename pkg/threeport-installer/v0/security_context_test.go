package v0

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"gorm.io/datatypes"

	v0 "github.com/threeport/threeport/pkg/api/v0"
)

// installerForHardeningTests is enough of an installer to build a deployment.
func installerForHardeningTests() *ControlPlaneInstaller {
	installer := &ControlPlaneInstaller{}
	installer.Opts.Namespace = "threeport-control-plane"
	installer.Opts.Name = "dev-0"

	return installer
}

// TestTheRestrictedContextMeetsTheStandard covers the values themselves
// against the Kubernetes restricted Pod Security Standard.
//
// The images are distroless and already run as 65532, so most of this is
// restating what the image does - at the cluster level, where it is enforced
// and auditable rather than trusted. A base image whose default user changed
// would otherwise move the control plane to a different one silently.
func TestTheRestrictedContextMeetsTheStandard(t *testing.T) {
	container := restrictedContainerSecurityContext()
	assert.Equal(t, false, container["allowPrivilegeEscalation"])
	assert.Equal(t, true, container["runAsNonRoot"])
	assert.Equal(t, nonRootUID, container["runAsUser"])
	assert.Equal(t, nonRootGID, container["runAsGroup"])

	dropped, _, err := unstructured.NestedStringSlice(container, "capabilities", "drop")
	require.NoError(t, err)
	assert.Equal(t, []string{"ALL"}, dropped, "restricted requires every capability dropped")

	assert.Equal(
		t, true, container["readOnlyRootFilesystem"],
		"the image is immutable for the life of the pod except where writableVolumeMounts allows",
	)

	pod := restrictedPodSecurityContext()
	assert.Equal(t, true, pod["runAsNonRoot"])
	profile, _, err := unstructured.NestedString(pod, "seccompProfile", "type")
	require.NoError(t, err)
	assert.Equal(t, "RuntimeDefault", profile, "restricted requires a seccomp profile on the pod")
}

// TestAControllerDeploymentIsHardened covers the one deployment that can be
// built without a cluster, end to end.
func TestAControllerDeploymentIsHardened(t *testing.T) {
	deployment, err := installerForHardeningTests().getControllerDeployment(
		"threeport-kubernetes-workload-controller",
		"threeport-control-plane",
		v0.ControlPlaneComponent{
			Name:               "kubernetes-workload-controller",
			BinaryName:         "kubernetes-workload-controller",
			ImageName:          "threeport-kubernetes-workload-controller",
			ServiceAccountName: "kubernetes-workload-controller",
		},
	)
	require.NoError(t, err)

	// the deployment spec holds plain ints - replicas, container ports - which
	// the copying accessors refuse, so these read the tree in place
	podContext, found, err := unstructured.NestedFieldNoCopy(
		deployment.Object, "spec", "template", "spec", "securityContext",
	)
	require.NoError(t, err)
	require.True(t, found, "the pod carries no security context")
	assert.Equal(t, restrictedPodSecurityContext(), podContext)

	containers, found, err := unstructured.NestedFieldNoCopy(
		deployment.Object, "spec", "template", "spec", "containers",
	)
	require.NoError(t, err)
	require.True(t, found)
	require.NotEmpty(t, containers)

	for _, raw := range containers.([]interface{}) {
		container := raw.(map[string]interface{})
		name := container["name"]
		context, found := container["securityContext"]
		require.True(t, found, "container %s carries no security context", name)
		assert.Equal(t, restrictedContainerSecurityContext(), context, "container %s", name)

		mounts, found := container["volumeMounts"].([]interface{})
		require.True(t, found, "container %s mounts nothing, so it can write nowhere", name)
		for _, writable := range writableVolumeMounts() {
			assert.Contains(
				t, mounts, writable,
				"container %s has a read-only root and no mount at %s",
				name, writable.(map[string]interface{})["mountPath"],
			)
		}
	}

	volumes, found, err := unstructured.NestedFieldNoCopy(
		deployment.Object, "spec", "template", "spec", "volumes",
	)
	require.NoError(t, err)
	require.True(t, found, "the pod declares no volumes")
	assert.Contains(
		t, volumes.([]interface{}), writableVolume(),
		"the mounts above need the volume behind them",
	)
}

// TestTheWritablePathsCoverWhatTheComponentsWrite covers the other half of a
// read-only root filesystem.
//
// Setting the flag without leaving the right paths writable does not fail at
// install. The container starts, reports ready, and fails the first time it
// writes - which for the terraform and pulumi controllers is the first
// reconcile of a real workload, and for helm is the first chart.
func TestTheWritablePathsCoverWhatTheComponentsWrite(t *testing.T) {
	paths := map[string]string{}
	for _, raw := range writableVolumeMounts() {
		mount := raw.(map[string]interface{})
		assert.Equal(
			t, writableVolumeName, mount["name"],
			"every writable path shares one volume",
		)
		assert.NotEmpty(t, mount["subPath"], "each path needs its own subPath within that volume")
		paths[mount["mountPath"].(string)] = mount["subPath"].(string)
	}

	for path, why := range map[string]string{
		"/tmp":          "terraform working directories, helm's rendered values, Go temporary files",
		"/home/nonroot": "pulumi state and plugin cache, gcloud application default credentials, the OCI config",
		"/var/helm":     "helm's repository config, which is written here and not under the cache or data directory",
	} {
		assert.Contains(t, paths, path, "%s has to stay writable: %s", path, why)
	}

	// a live install caught this: mounting only /var/helm/.cache/helm and
	// /var/helm/.local/share/helm left HelmRepoConfigFile on the read-only
	// root, and the controller failed at its first chart rather than at
	// install
	for _, leaf := range []string{"/var/helm/.cache/helm", "/var/helm/.local/share/helm"} {
		assert.NotContains(
			t, paths, leaf,
			"%s is covered by mounting /var/helm; mounting the leaves instead leaves the repository config read-only",
			leaf,
		)
	}

	subPaths := map[string]bool{}
	for path, subPath := range paths {
		assert.False(t, subPaths[subPath], "%s reuses a subPath, so two paths would share storage", path)
		subPaths[subPath] = true
	}
}

// TestACallerSOwnMountIsNotMountedTwice covers a component that brings a
// volume at one of the writable paths.
//
// A mount path has to be unique within a container. Two mounts at /tmp is not
// something Kubernetes resolves - it refuses the deployment with "must be
// unique" - so a module controller that mounts /tmp would not get a worse
// filesystem, it would fail to install at all.
func TestACallerSOwnMountIsNotMountedTwice(t *testing.T) {
	deployment, err := installerForHardeningTests().getControllerDeployment(
		"threeport-custom-controller",
		"threeport-control-plane",
		v0.ControlPlaneComponent{
			Name:                   "custom-controller",
			BinaryName:             "custom-controller",
			ImageName:              "threeport-custom-controller",
			ServiceAccountName:     "custom-controller",
			AdditionalVolumes:      jsonDocument(`[{"name":"scratch","emptyDir":{}}]`),
			AdditionalVolumeMounts: jsonDocument(`[{"name":"scratch","mountPath":"/tmp"}]`),
		},
	)
	require.NoError(t, err)

	containers, _, err := unstructured.NestedFieldNoCopy(
		deployment.Object, "spec", "template", "spec", "containers",
	)
	require.NoError(t, err)

	for _, raw := range containers.([]interface{}) {
		container := raw.(map[string]interface{})
		seen := map[string]string{}
		for _, rawMount := range container["volumeMounts"].([]interface{}) {
			mount := rawMount.(map[string]interface{})
			path := mount["mountPath"].(string)
			name := mount["name"].(string)
			if previous, repeated := seen[path]; repeated {
				t.Errorf(
					"%s is mounted twice, by %s and by %s; kubernetes refuses the deployment",
					path, previous, name,
				)
			}
			seen[path] = name
		}
		assert.Equal(t, "scratch", seen["/tmp"], "the caller asked for this path, so their volume keeps it")
		for _, path := range []string{"/home/nonroot", "/var/helm"} {
			assert.NotEmpty(t, seen[path], "%s is still needed and nobody else mounted it", path)
		}
	}
}

// jsonDocument is the shape the installer is handed additional volumes in.
func jsonDocument(document string) *datatypes.JSON {
	raw := datatypes.JSON(document)

	return &raw
}

// TestEveryMountNamesAVolumeThePodDeclares covers the pairing between a mount
// and the volume behind it.
//
// A mount naming a volume the pod does not declare is refused outright -
// `Not found: "writable"` - so this is not a degraded filesystem, it is a
// deployment that never installs.
//
// The case that matters is a component that mounts every writable path
// itself. The volume is the pod's, and other containers in the same pod take
// mounts from it on their own account: the api server's init containers and
// the agent's vendored sidecar both do. Deciding whether to declare it from
// one container's needs leaves those mounts pointing at nothing.
func TestEveryMountNamesAVolumeThePodDeclares(t *testing.T) {
	coversEveryWritablePath := `[
		{"name":"scratch","mountPath":"/tmp"},
		{"name":"scratch","mountPath":"/home/nonroot"},
		{"name":"scratch","mountPath":"/var/helm"}
	]`

	for name, component := range map[string]v0.ControlPlaneComponent{
		"a controller that brings nothing": {
			Name:               "plain-controller",
			BinaryName:         "plain-controller",
			ServiceAccountName: "plain-controller",
		},
		"a controller that mounts every writable path itself": {
			Name:                   "crowded-controller",
			BinaryName:             "crowded-controller",
			ServiceAccountName:     "crowded-controller",
			AdditionalVolumes:      jsonDocument(`[{"name":"scratch","emptyDir":{}}]`),
			AdditionalVolumeMounts: jsonDocument(coversEveryWritablePath),
		},
		// a volume name has to be unique within a pod, so a caller who
		// happens to use this name must not get a second one
		"a controller that brings a volume of the same name": {
			Name:               "colliding-controller",
			BinaryName:         "colliding-controller",
			ServiceAccountName: "colliding-controller",
			AdditionalVolumes:  jsonDocument(`[{"name":"writable","emptyDir":{}}]`),
		},
	} {
		t.Run(name, func(t *testing.T) {
			deployment, err := installerForHardeningTests().getControllerDeployment(
				"threeport-"+component.Name, "threeport-control-plane", component,
			)
			require.NoError(t, err)

			declared := map[string]bool{}
			volumes, found, err := unstructured.NestedFieldNoCopy(
				deployment.Object, "spec", "template", "spec", "volumes",
			)
			require.NoError(t, err)
			require.True(t, found, "the pod declares no volumes at all")
			for _, raw := range volumes.([]interface{}) {
				volumeName := raw.(map[string]interface{})["name"].(string)
				assert.False(
					t, declared[volumeName],
					"%s is declared twice; Kubernetes refuses the deployment with Duplicate value",
					volumeName,
				)
				declared[volumeName] = true
			}

			assert.True(
				t, declared[writableVolumeName],
				"the writable volume has to be declared even when this container needs no mount from it, "+
					"because the init containers and the agent's sidecar mount it on their own",
			)

			containers, _, err := unstructured.NestedFieldNoCopy(
				deployment.Object, "spec", "template", "spec", "containers",
			)
			require.NoError(t, err)
			for _, raw := range containers.([]interface{}) {
				container := raw.(map[string]interface{})
				for _, rawMount := range container["volumeMounts"].([]interface{}) {
					mount := rawMount.(map[string]interface{})
					assert.True(
						t, declared[mount["name"].(string)],
						"%s mounts %s at %s, which the pod does not declare",
						container["name"], mount["name"], mount["mountPath"],
					)
				}
			}
		})
	}
}

// TestEveryContainerThisInstallerWritesIsHardened covers the containers a
// behavioural test cannot reach.
//
// The API server and agent deployments are built inside their install
// functions, which need a cluster. A container added to one of them without a
// security context would run with the defaults - privilege escalation
// allowed, every capability held - and nothing would report it, because a
// pod that is too permissive still starts and still works.
func TestEveryContainerThisInstallerWritesIsHardened(t *testing.T) {
	fileSet := token.NewFileSet()
	file, err := parser.ParseFile(fileSet, "components.go", nil, 0)
	require.NoError(t, err)

	containers := 0
	ast.Inspect(file, func(node ast.Node) bool {
		literal, ok := node.(*ast.CompositeLit)
		if !ok {
			return true
		}
		keys := keyedStrings(literal)
		// a container is the literal that names an image to run; volumes,
		// mounts and ports carry a name and no image
		if !keys[`"image"`] || !keys[`"name"`] {
			return true
		}
		containers++
		// the helper, not any security context: a literal written here could
		// say securityContext: {} and satisfy a check for the key alone,
		// which is a weaker container than no check would have produced
		assert.True(
			t, callsHelper(keyedValue(literal, `"securityContext"`), "restrictedContainerSecurityContext"),
			"%s: a container does not take its security context from restrictedContainerSecurityContext",
			fileSet.Position(literal.Pos()),
		)

		return true
	})

	require.NotZero(t, containers, "no container declaration found to check")
}

// TestEveryHandWrittenContainerDropsPrivilege covers the containers whose
// security context is written out rather than taken from a helper: the
// support services operator's and the cluster autoscaler's, in
// support_services.go. The vendored dependencies and the reinstall jobs take
// theirs from a helper, which TestEveryVendoredContainerIsHardened requires.
//
// The values are checked, not the keys: allowPrivilegeEscalation set to true,
// or a capabilities list that drops something other than ALL, still has every
// key a key check looks for.
func TestEveryHandWrittenContainerDropsPrivilege(t *testing.T) {
	for _, path := range []string{"support_services.go"} {
		t.Run(path, func(t *testing.T) {
			fileSet := token.NewFileSet()
			file, err := parser.ParseFile(fileSet, path, nil, 0)
			require.NoError(t, err)

			containers := 0
			ast.Inspect(file, func(node ast.Node) bool {
				literal, ok := node.(*ast.CompositeLit)
				if !ok || !isContainerLiteral(literal) {
					return true
				}
				containers++

				securityContext, ok := keyedValue(literal, `"securityContext"`).(*ast.CompositeLit)
				if !assert.True(
					t, ok,
					"%s: a container is declared with no security context written out",
					fileSet.Position(literal.Pos()),
				) {
					return true
				}
				assert.True(
					t, isIdent(keyedValue(securityContext, `"allowPrivilegeEscalation"`), "false"),
					"%s: a container does not set allowPrivilegeEscalation to false",
					fileSet.Position(literal.Pos()),
				)
				assert.True(
					t, dropsAllCapabilities(securityContext),
					"%s: a container does not drop ALL capabilities",
					fileSet.Position(literal.Pos()),
				)

				return true
			})

			require.NotZero(t, containers, "no container declaration found to check")
		})
	}
}

// isContainerLiteral reports whether a composite literal declares a
// container: it names one, and an image to run as a value rather than as a
// nested object, which is how a CRD schema or a Helm value describes an image.
func isContainerLiteral(literal *ast.CompositeLit) bool {
	keys := keyedStrings(literal)
	if !keys[`"image"`] || !keys[`"name"`] {
		return false
	}
	_, nested := keyedValue(literal, `"image"`).(*ast.CompositeLit)

	return !nested
}

// isIdent reports whether an expression is the named identifier.
func isIdent(expr ast.Expr, name string) bool {
	ident, ok := expr.(*ast.Ident)

	return ok && ident.Name == name
}

// dropsAllCapabilities reports whether a security context literal drops
// exactly the capability list ALL, and adds none back.
func dropsAllCapabilities(securityContext *ast.CompositeLit) bool {
	capabilities, ok := keyedValue(securityContext, `"capabilities"`).(*ast.CompositeLit)
	if !ok || keyedValue(capabilities, `"add"`) != nil {
		return false
	}
	drop, ok := keyedValue(capabilities, `"drop"`).(*ast.CompositeLit)
	if !ok || len(drop.Elts) != 1 {
		return false
	}
	all, ok := drop.Elts[0].(*ast.BasicLit)

	return ok && all.Value == `"ALL"`
}

// TestEveryPodThisInstallerWritesSetsASeccompProfile covers the requirement
// that is set on the pod rather than on each container.
//
// A live audit found the support services operator missing exactly this and
// nothing else: its containers were hardened, its pod declared runAsNonRoot,
// and the namespace still refused to create it. Checking containers alone
// does not see it, which is why it is checked separately here.
func TestEveryPodThisInstallerWritesSetsASeccompProfile(t *testing.T) {
	for _, path := range podSourceFiles {
		t.Run(path, func(t *testing.T) {
			fileSet := token.NewFileSet()
			file, err := parser.ParseFile(fileSet, path, nil, 0)
			require.NoError(t, err)

			pods := 0
			ast.Inspect(file, func(node ast.Node) bool {
				literal, ok := node.(*ast.CompositeLit)
				if !ok {
					return true
				}
				// a pod spec is the literal that lists containers to run
				if !keyedStrings(literal)[`"containers"`] {
					return true
				}
				pods++

				securityContext := keyedValue(literal, `"securityContext"`)
				if !assert.NotNil(
					t, securityContext,
					"%s: a pod is declared with no security context",
					fileSet.Position(literal.Pos()),
				) {
					return true
				}
				// either the helper, or a literal that names the profile
				// itself - support_services.go writes one out by hand. The
				// value matters, not the key: seccompProfile set to
				// Unconfined is the regression this is here to catch.
				assert.True(
					t,
					callsHelper(securityContext, "restrictedPodSecurityContext") ||
						callsHelper(securityContext, "dependencyPodSecurityContext") ||
						callsHelper(securityContext, "natsBoxPodSecurityContext") ||
						mentions(securityContext, "RuntimeDefault"),
					"%s: a pod does not set a RuntimeDefault seccomp profile",
					fileSet.Position(literal.Pos()),
				)

				return true
			})

			require.NotZero(t, pods, "no pod declaration found to check")
		})
	}
}

// podSourceFiles are the files that write a pod spec into the control plane
// namespace: the components, the support services operator, the vendored
// dependencies, and the jobs a reinstall runs to drop the database and the
// message broker's streams.
var podSourceFiles = []string{"components.go", "support_services.go", "dependencies.go", "reinstall.go"}

// TestEveryPodThisInstallerWritesRunsAsNonRoot covers the last restricted
// requirement the vendored dependencies were missing.
//
// The restricted Pod Security Standard requires runAsNonRoot on the pod or on
// every container. The namespace refuses a pod that sets neither, so a pod
// added without it does not run less safely - it does not run at all once the
// namespace enforces restricted, which an install does not do itself and so
// would not catch.
func TestEveryPodThisInstallerWritesRunsAsNonRoot(t *testing.T) {
	for _, path := range podSourceFiles {
		t.Run(path, func(t *testing.T) {
			fileSet := token.NewFileSet()
			file, err := parser.ParseFile(fileSet, path, nil, 0)
			require.NoError(t, err)

			pods := 0
			ast.Inspect(file, func(node ast.Node) bool {
				literal, ok := node.(*ast.CompositeLit)
				if !ok || !keyedStrings(literal)[`"containers"`] {
					return true
				}
				pods++

				podContext := keyedValue(literal, `"securityContext"`)
				if callsHelper(podContext, "restrictedPodSecurityContext") ||
					callsHelper(podContext, "dependencyPodSecurityContext") ||
					callsHelper(podContext, "natsBoxPodSecurityContext") ||
					setsRunAsNonRoot(podContext) {
					return true
				}

				// otherwise every container has to set it itself
				containers, ok := keyedValue(literal, `"containers"`).(*ast.CompositeLit)
				require.True(t, ok, "%s: containers is not a literal", fileSet.Position(literal.Pos()))
				for _, element := range containers.Elts {
					container, ok := element.(*ast.CompositeLit)
					if !ok {
						continue
					}
					containerContext := keyedValue(container, `"securityContext"`)
					assert.True(
						t,
						callsHelper(containerContext, "restrictedContainerSecurityContext") || setsRunAsNonRoot(containerContext),
						"%s: neither the pod nor this container sets runAsNonRoot",
						fileSet.Position(container.Pos()),
					)
				}

				return true
			})

			require.NotZero(t, pods, "no pod declaration found to check")
		})
	}
}

// setsRunAsNonRoot reports whether a security context literal sets
// runAsNonRoot to true.
func setsRunAsNonRoot(securityContext ast.Expr) bool {
	literal, ok := securityContext.(*ast.CompositeLit)
	if !ok {
		return false
	}
	value, ok := keyedValue(literal, `"runAsNonRoot"`).(*ast.Ident)

	return ok && value.Name == "true"
}

// TestEveryVendoredContainerIsHardened covers the containers of the
// vendored dependencies and of the jobs a reinstall runs, whose images
// threeport does not build.
//
// They take their security context from a helper, as the threeport
// components do, so each one has a read-only root filesystem and every
// restricted container requirement. A literal written out here could leave
// one of them off and still start - which is how the database and the
// message broker ran without runAsNonRoot until #635.
func TestEveryVendoredContainerIsHardened(t *testing.T) {
	for _, path := range []string{"dependencies.go", "reinstall.go"} {
		t.Run(path, func(t *testing.T) {
			fileSet := token.NewFileSet()
			file, err := parser.ParseFile(fileSet, path, nil, 0)
			require.NoError(t, err)

			containers := 0
			ast.Inspect(file, func(node ast.Node) bool {
				literal, ok := node.(*ast.CompositeLit)
				if !ok {
					return true
				}
				keys := keyedStrings(literal)
				if !keys[`"image"`] || !keys[`"name"`] {
					return true
				}
				containers++
				securityContext := keyedValue(literal, `"securityContext"`)
				assert.True(
					t,
					callsHelper(securityContext, "restrictedContainerSecurityContext") ||
						callsHelper(securityContext, "vendoredContainerSecurityContext"),
					"%s: a container does not take its security context from a hardening helper",
					fileSet.Position(literal.Pos()),
				)

				return true
			})

			require.NotZero(t, containers, "no container declaration found to check")
		})
	}
}

// TestTheDependencyContextsMeetTheStandard covers the values the vendored
// dependencies run with, which the source checks above only see by name.
func TestTheDependencyContextsMeetTheStandard(t *testing.T) {
	pod := dependencyPodSecurityContext()
	assert.Equal(t, true, pod["runAsNonRoot"])
	assert.Equal(t, nonRootUID, pod["runAsUser"])
	assert.Equal(t, nonRootGID, pod["runAsGroup"])
	assert.Equal(t, nonRootGID, pod["fsGroup"], "the data volume and the database's certificates are given to this group")
	profile, _, err := unstructured.NestedString(pod, "seccompProfile", "type")
	require.NoError(t, err)
	assert.Equal(t, "RuntimeDefault", profile)

	// an image that declares its own user keeps it
	container := vendoredContainerSecurityContext()
	assert.Equal(t, false, container["allowPrivilegeEscalation"])
	assert.Equal(t, true, container["readOnlyRootFilesystem"])
	drop, _, err := unstructured.NestedStringSlice(container, "capabilities", "drop")
	require.NoError(t, err)
	assert.Equal(t, []string{"ALL"}, drop)
	assert.NotContains(t, container, "runAsUser", "the image's own user is the one the pod names")
}

// TestNatsBoxPinsTheNumericUserItsImageDeclaresByName covers the check a
// named USER defeats on its own.
//
// A review on #618 found nats-box set runAsNonRoot with no runAsUser: the
// kubelet does not resolve a named USER against the image's /etc/passwd, so
// runAsNonRoot alone refuses every container in the pod with
// CreateContainerConfigError, "image has non-numeric user (nats), cannot
// verify user is non-root" - confirmed on a running kind cluster, not assumed
// from the image's declared USER. That failure is invisible to every other
// check here: the security context is present, drops every capability, and
// sets RuntimeDefault - it just names a requirement the kubelet cannot
// enforce on its own.
//
// This is deliberately not a blanket rule that every hand-written
// runAsNonRoot needs a sibling runAsUser: two containers in
// support_services.go set runAsNonRoot with none, and both of their images
// were confirmed, the same way nats-box was, to already declare a numeric
// USER - which the kubelet can resolve unaided, and pinning one here would
// only let that image's default drift from what the manifest claims. Whether
// a container needs the numeric id is a fact about its image, not about the
// source, and nothing here can see that - so this checks only the image that
// was confirmed to need it.
func TestNatsBoxPinsTheNumericUserItsImageDeclaresByName(t *testing.T) {
	securityContext := natsBoxPodSecurityContext()
	assert.Equal(t, true, securityContext["runAsNonRoot"])
	// confirmed against the image with
	// `docker run --entrypoint id natsio/nats-box:0.16.0-nonroot`
	assert.Equal(t, int64(1000), securityContext["runAsUser"],
		"nats-box's image declares USER nats:nats by name, which the kubelet cannot resolve on its own")
	assert.Equal(t, int64(1000), securityContext["runAsGroup"])
	profile, _, err := unstructured.NestedString(securityContext, "seccompProfile", "type")
	require.NoError(t, err)
	assert.Equal(t, "RuntimeDefault", profile)
}

// TestEveryNatsBoxPodTakesItsSecurityContextFromTheHelper covers every pod
// that runs the nats-box image, wherever in the installer it is written: the
// utility pod and the job a reinstall runs to drop the message broker's
// streams both do.  Each takes its security context from
// natsBoxPodSecurityContext, which the test above checks, so a copy written
// out by hand cannot drift from it, and a new nats-box pod is held to it too.
func TestEveryNatsBoxPodTakesItsSecurityContextFromTheHelper(t *testing.T) {
	paths, err := filepath.Glob("*.go")
	require.NoError(t, err)

	pods := 0
	for _, path := range paths {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		fileSet := token.NewFileSet()
		file, err := parser.ParseFile(fileSet, path, nil, 0)
		require.NoError(t, err)

		ast.Inspect(file, func(node ast.Node) bool {
			literal, ok := node.(*ast.CompositeLit)
			if !ok || !mentions(keyedValue(literal, `"image"`), "natsBoxImage") {
				return true
			}
			pods++

			pod := enclosingPodLiteral(file, literal)
			if !assert.NotNil(t, pod, "%s: a nats-box container is not inside a pod spec", fileSet.Position(literal.Pos())) {
				return true
			}
			assert.True(
				t, callsHelper(keyedValue(pod, `"securityContext"`), "natsBoxPodSecurityContext"),
				"%s: a nats-box pod does not take its security context from natsBoxPodSecurityContext",
				fileSet.Position(pod.Pos()),
			)

			return true
		})
	}

	require.GreaterOrEqual(t, pods, 2, "the nats-box utility pod and the message broker drop job were not both found")
}

// enclosingPodLiteral returns the nearest composite literal that lists
// containers and contains the given one.
func enclosingPodLiteral(file *ast.File, inner *ast.CompositeLit) *ast.CompositeLit {
	var pods []*ast.CompositeLit
	ast.Inspect(file, func(node ast.Node) bool {
		literal, ok := node.(*ast.CompositeLit)
		if ok && keyedStrings(literal)[`"containers"`] {
			pods = append(pods, literal)
		}

		return true
	})

	for _, pod := range pods {
		if pod.Pos() <= inner.Pos() && inner.End() <= pod.End() {
			return pod
		}
	}

	return nil
}

// keyedStrings returns the string keys of a composite literal, as written.
func keyedStrings(literal *ast.CompositeLit) map[string]bool {
	keys := map[string]bool{}
	for _, element := range literal.Elts {
		keyed, ok := element.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		if basic, ok := keyed.Key.(*ast.BasicLit); ok {
			keys[basic.Value] = true
		}
	}

	return keys
}

// keyedValue returns the value a composite literal gives one string key.
func keyedValue(literal *ast.CompositeLit, key string) ast.Expr {
	for _, element := range literal.Elts {
		keyed, ok := element.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		if basic, ok := keyed.Key.(*ast.BasicLit); ok && basic.Value == key {
			return keyed.Value
		}
	}

	return nil
}

// mentions reports whether any of the names appears inside, as an identifier
// or as a string literal - the security context is either built by a helper
// or written out, and both have to count.
func mentions(node ast.Node, names ...string) bool {
	// ast.Walk panics on a nil node rather than reporting no match. keyedValue
	// returns nil for a key that is not there, which every caller treats as
	// "nothing to check" - a panic here would mean a literal that simply
	// lacks the key crashes the whole file's checks instead of failing just
	// the one assertion that cares.
	if node == nil || reflect.ValueOf(node).IsNil() {
		return false
	}

	found := false
	ast.Inspect(node, func(inner ast.Node) bool {
		switch typed := inner.(type) {
		case *ast.Ident:
			for _, name := range names {
				if typed.Name == name {
					found = true
				}
			}
		case *ast.BasicLit:
			for _, name := range names {
				if typed.Value == `"`+name+`"` {
					found = true
				}
			}
		}

		return !found
	})

	return found
}

// callsHelper reports whether an expression is a call to the named function.
//
// Checking for the call rather than for the fields it returns is what makes
// these source checks mean anything: the helper's values are asserted in
// TestTheRestrictedContextMeetsTheStandard, so a site that uses it inherits
// them, and a site that writes its own literal is reported here rather than
// silently setting something weaker.
func callsHelper(expr ast.Expr, name string) bool {
	call, ok := expr.(*ast.CallExpr)
	if !ok {
		return false
	}
	function, ok := call.Fun.(*ast.Ident)

	return ok && function.Name == name
}
