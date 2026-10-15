package v0

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

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
		assert.True(
			t, keys[`"securityContext"`],
			"%s: a container is declared without a security context",
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
				assert.True(
					t, mentions(securityContext, "restrictedPodSecurityContext", "seccompProfile"),
					"%s: a pod is declared without a seccomp profile",
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
