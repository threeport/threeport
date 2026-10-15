package v0

import (
	"go/ast"
	"go/parser"
	"go/token"
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

// TestEveryPodThisInstallerWritesSetsASeccompProfile covers the requirement
// that is set on the pod rather than on each container.
//
// A live audit found the support services operator missing exactly this and
// nothing else: its containers were hardened, its pod declared runAsNonRoot,
// and the namespace still refused to create it. Checking containers alone
// does not see it, which is why it is checked separately here.
func TestEveryPodThisInstallerWritesSetsASeccompProfile(t *testing.T) {
	for _, path := range []string{"components.go", "support_services.go"} {
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
