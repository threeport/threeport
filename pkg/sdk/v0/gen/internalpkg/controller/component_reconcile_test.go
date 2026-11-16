package controller

import (
	"strings"
	"testing"

	. "github.com/dave/jennifer/jen"

	"github.com/threeport/threeport/pkg/sdk/v0/gen"
)

// TestSupportComponentsReadsWorkloadFields checks the field scan.
func TestSupportComponentsReadsWorkloadFields(t *testing.T) {
	// the fixture declares a runtime and all four workloads
	tags := map[string]map[string]map[string]string{
		"ReconcilerTestInstance": {
			"KubernetesRuntimeInstanceID": {"relationship": "requires"},
			"IngressComponentInstanceID": {
				"relationship": "requires;type:KubernetesWorkloadInstance",
			},
			"CertificatesComponentInstanceID": {
				"relationship": "requires;type:KubernetesWorkloadInstance",
			},
			"SecretsComponentInstanceID": {
				"relationship": "requires;type:KubernetesWorkloadInstance",
			},
			"DatabaseComponentInstanceID": {
				"relationship": "requires;type:KubernetesWorkloadInstance",
			},
		},
	}
	got := supportComponents(tags, "ReconcilerTestInstance")
	want := []string{"ingress", "certificates", "secrets", "database"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("supportComponents() = %v, want %v", got, want)
	}

	// a workload field without a runtime is not enough to install
	delete(tags["ReconcilerTestInstance"], "KubernetesRuntimeInstanceID")
	if got := supportComponents(tags, "ReconcilerTestInstance"); len(got) != 0 {
		t.Fatalf("supportComponents() without a runtime = %v", got)
	}
}

// TestEmitSupportComponentsNamesTheCalls checks the generated call names.
func TestEmitSupportComponentsNamesTheCalls(t *testing.T) {
	components := []string{"ingress", "certificates", "secrets", "database"}
	ensure := renderComponents(t, components, false)
	for _, name := range []string{
		"EnsureIngressComponent",
		"EnsureCertificatesComponent",
		"EnsureSecretsComponent",
		"EnsureDatabaseComponent",
		"UpdateReconcilerTestInstance",
	} {
		if !strings.Contains(ensure, name) {
			t.Fatalf("ensure output missing %s:\n%s", name, ensure)
		}
	}
	release := renderComponents(t, components, true)
	for _, name := range []string{
		"ReleaseIngressComponent",
		"ReleaseCertificatesComponent",
		"ReleaseSecretsComponent",
		"ReleaseDatabaseComponent",
	} {
		if !strings.Contains(release, name) {
			t.Fatalf("release output missing %s:\n%s", name, release)
		}
	}
	if strings.Contains(release, "EnsureIngressComponent") {
		t.Fatalf("release output called ensure:\n%s", release)
	}

	// a failed runtime load skips the component calls without an else
	runtimeErr := `operationErr = fmt.Errorf("failed to get kubernetes runtime instance: %w", err)`
	ingressAt := strings.Index(ensure, "EnsureIngressComponent")
	runtimeAt := strings.Index(ensure, runtimeErr)
	if runtimeAt < 0 || ingressAt < 0 || runtimeAt > ingressAt {
		t.Fatalf("ensure output ordered the runtime load after ingress:\n%s", ensure)
	}
	if strings.Contains(ensure[runtimeAt:ingressAt], "else") {
		t.Fatalf("runtime load wrapped the component calls in else:\n%s", ensure[runtimeAt:ingressAt])
	}

	// ensure passes the attached instance's field address and release does not
	for _, field := range []string{
		"&componentObject.IngressComponentInstanceID",
		"&componentObject.CertificatesComponentInstanceID",
		"&componentObject.SecretsComponentInstanceID",
		"&componentObject.DatabaseComponentInstanceID",
	} {
		if !strings.Contains(ensure, field) {
			t.Fatalf("ensure output missing %s:\n%s", field, ensure)
		}
		if strings.Contains(release, field) {
			t.Fatalf("release output passed %s:\n%s", field, release)
		}
	}
}

// TestOperationCaseSeparatesHandlerStatements checks the handler lines stay separate.
func TestOperationCaseSeparatesHandlerStatements(t *testing.T) {
	obj := &gen.ReconciledObject{
		Name:     "ReconcilerTestInstance",
		Versions: []string{"v0"},
	}
	modulePath := "github.com/threeport/threeport/internal/reconcilertest"
	components := []string{"ingress", "certificates", "secrets", "database"}

	// delete runs the handler before it releases the components
	deleted := renderOperation(t, "delete", obj, modulePath, components)
	if !strings.Contains(deleted, "customRequeueDelay = requeueDelay\n") {
		t.Fatalf("delete handler statements were joined:\n%s", deleted)
	}
	if strings.Index(deleted, "v0ReconcilerTestInstanceDeleted") > strings.Index(deleted, "ReleaseIngressComponent") {
		t.Fatalf("delete released the component before the handler:\n%s", deleted)
	}

	// create installs the components before it runs the handler
	created := renderOperation(t, "create", obj, modulePath, components)
	if strings.Index(created, "EnsureIngressComponent") > strings.Index(created, "v0ReconcilerTestInstanceCreated") {
		t.Fatalf("create ran the handler before the component:\n%s", created)
	}

	// an object with no component fields keeps the three handler statements
	plain := renderOperation(t, "update", obj, modulePath, nil)
	if !strings.Contains(plain, "customRequeueDelay = requeueDelay\n") {
		t.Fatalf("plain handler statements were joined:\n%s", plain)
	}
	if strings.Contains(plain, "EnsureIngressComponent") {
		t.Fatalf("plain update installed a component:\n%s", plain)
	}
}

// renderOperation returns one generated operation case.
func renderOperation(
	t *testing.T,
	op string,
	obj *gen.ReconciledObject,
	modulePath string,
	components []string,
) string {
	t.Helper()

	// render the case inside a switch so the case keyword is valid
	f := NewFile("fixture")
	f.Func().Id("generated").Params(Id("op").String()).BlockFunc(func(g *Group) {
		g.Switch(Id("op")).BlockFunc(func(h *Group) {
			operationCase(h, op, obj, "reconcilerTestInstance", modulePath, components)
		})
	})
	var buf strings.Builder
	if err := f.Render(&buf); err != nil {
		t.Fatalf("render %s case: %v", op, err)
	}
	return buf.String()
}

// renderComponents returns the generated component calls.
func renderComponents(t *testing.T, components []string, release bool) string {
	t.Helper()

	// render the calls inside a function so jennifer has a block
	f := NewFile("fixture")
	f.Func().Id("generated").Params().BlockFunc(func(g *Group) {
		emitSupportComponents(
			g,
			components,
			release,
			"reconcilerTestInstance",
			"ReconcilerTestInstance",
			"v0",
			"github.com/threeport/threeport/internal/reconcilertest",
		)
	})
	var buf strings.Builder
	if err := f.Render(&buf); err != nil {
		t.Fatalf("render component calls: %v", err)
	}
	return buf.String()
}
