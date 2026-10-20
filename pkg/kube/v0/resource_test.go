package v0

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	kubeerr "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	kubemetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/client-go/dynamic"
)

// TestDeleteResourceTreatsMissingCRDAsSuccess covers the case where the
// target cluster has no CRD for the resource's kind: DeleteResource should
// return nil (treat as already-deleted) so the workload-instance reconciler
// does not hang on a resource that cannot possibly exist. The case arises
// when a gateway controller materializes a VirtualService (gateway.solo.io)
// and records it in the threeport database, but the CRD was never installed
// in the target cluster.
func TestDeleteResourceTreatsMissingCRDAsSuccess(t *testing.T) {
	// an empty mapper knows about zero kinds, so any RESTMapping lookup
	// returns *meta.NoKindMatchError, the same error a cluster with no CRD
	// for the kind produces. getResourceMapping wraps that error before
	// DeleteResource inspects it, so this also covers the wrapped case.
	emptyMapper := meta.NewDefaultRESTMapper(nil)

	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "gateway.solo.io",
		Version: "v1",
		Kind:    "VirtualService",
	})
	obj.SetName("does-not-matter")
	obj.SetNamespace("default")

	// dynamic client stays nil on purpose: DeleteResource short-circuits on
	// the mapper miss and never dereferences the client. Should it stop
	// short-circuiting, the call reaches the client, panics on nil, and this
	// test fails loudly instead of appearing to pass.
	err := DeleteResource(obj, nil, emptyMapper)
	if err != nil {
		t.Fatalf("DeleteResource with missing-CRD kind: got err %v, want nil", err)
	}
}

// TestDeleteResourceForwardsOtherMapperErrors covers the negative case: a
// mapper error that is NOT a "no match for kind" (e.g. a wrapped error
// with a different underlying cause) should still surface as an error so
// callers aren't silently masked from unrelated failures.
func TestDeleteResourceForwardsOtherMapperErrors(t *testing.T) {
	// wrap a non-NoMatch error in a mapper so getResourceMapping returns it
	// via the normal path
	mapper := &errMapper{err: errOtherMapperFailure}

	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "apps",
		Version: "v1",
		Kind:    "Deployment",
	})

	err := DeleteResource(obj, nil, mapper)
	if err == nil {
		t.Fatalf("DeleteResource with non-NoMatch mapper error: got nil, want error")
	}
}

// TestIsTransientKubeError covers which apiserver responses CreateResource
// is willing to retry. The transient cases all mean the server is behind
// rather than the request being wrong; the deterministic cases mean the
// request will fail the same way no matter how many times it is sent.
func TestIsTransientKubeError(t *testing.T) {
	deployments := schema.GroupResource{Group: "apps", Resource: "deployments"}

	cases := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "nil error",
			err:  nil,
			want: false,
		},
		{
			name: "server timeout",
			err:  kubeerr.NewServerTimeout(deployments, "create", 1),
			want: true,
		},
		{
			name: "request timeout",
			err:  kubeerr.NewTimeoutError("request timed out", 1),
			want: true,
		},
		{
			name: "service unavailable",
			err:  kubeerr.NewServiceUnavailable("no backends available"),
			want: true,
		},
		{
			name: "too many requests",
			err:  kubeerr.NewTooManyRequestsError("slow down"),
			want: true,
		},
		{
			name: "quota evaluator timeout",
			err:  kubeerr.NewInternalError(errors.New("resource quota evaluation timed out")),
			want: true,
		},
		{
			name: "internal error with an unrelated cause",
			err:  kubeerr.NewInternalError(errors.New("something else went wrong")),
			want: false,
		},
		{
			name: "already exists",
			err:  kubeerr.NewAlreadyExists(deployments, "my-app"),
			want: false,
		},
		{
			name: "not found",
			err:  kubeerr.NewNotFound(deployments, "my-app"),
			want: false,
		},
		{
			name: "forbidden",
			err:  kubeerr.NewForbidden(deployments, "my-app", errors.New("not allowed")),
			want: false,
		},
		{
			name: "invalid",
			err:  kubeerr.NewInvalid(schema.GroupKind{Group: "apps", Kind: "Deployment"}, "my-app", field.ErrorList{}),
			want: false,
		},
		{
			name: "error that did not come from the kubernetes API",
			err:  errors.New("dial tcp: connection refused"),
			want: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isTransientKubeError(tc.err); got != tc.want {
				t.Fatalf("isTransientKubeError(%v): got %t, want %t", tc.err, got, tc.want)
			}
		})
	}
}

// TestCreateResourceRetriesUntilTransientErrorClears covers the case the
// retry loop exists for: an apiserver under load rejects the first
// attempts with a quota evaluator timeout, then accepts the resource once
// it catches up. CreateResource should keep trying and return the created
// object rather than failing the install.
func TestCreateResourceRetriesUntilTransientErrorClears(t *testing.T) {
	shrinkCreateRetryDelay(t)

	client := &stubDynamicClient{
		resourceClient: &stubResourceClient{
			results: []stubCreateResult{
				{err: kubeerr.NewInternalError(errors.New("resource quota evaluation timed out"))},
				{err: kubeerr.NewServerTimeout(schema.GroupResource{Group: "apps", Resource: "deployments"}, "create", 1)},
				{object: testDeployment()},
			},
		},
	}

	result, err := CreateResource(testDeployment(), client, testDeploymentMapper())
	if err != nil {
		t.Fatalf("CreateResource with two transient failures: got err %v, want nil", err)
	}
	if result == nil {
		t.Fatal("CreateResource with two transient failures: got nil object, want the created resource")
	}
	if client.resourceClient.attempts != 3 {
		t.Fatalf("CreateResource with two transient failures: got %d attempts, want 3", client.resourceClient.attempts)
	}
}

// TestCreateResourceGivesUpOnPersistentTransientError covers the ceiling on
// retries: an apiserver that never recovers gets a bounded number of
// attempts, and the caller sees the last error rather than the call
// blocking indefinitely.
func TestCreateResourceGivesUpOnPersistentTransientError(t *testing.T) {
	shrinkCreateRetryDelay(t)

	client := &stubDynamicClient{
		resourceClient: &stubResourceClient{
			alwaysErr: kubeerr.NewInternalError(errors.New("resource quota evaluation timed out")),
		},
	}

	_, err := CreateResource(testDeployment(), client, testDeploymentMapper())
	if err == nil {
		t.Fatal("CreateResource against an apiserver that never recovers: got nil, want error")
	}
	if !strings.Contains(err.Error(), "resource quota evaluation timed out") {
		t.Fatalf("CreateResource against an apiserver that never recovers: error %q does not carry the apiserver's message", err)
	}
	if client.resourceClient.attempts != createRetryAttempts {
		t.Fatalf(
			"CreateResource against an apiserver that never recovers: got %d attempts, want %d",
			client.resourceClient.attempts, createRetryAttempts,
		)
	}
}

// TestCreateResourceDoesNotRetryDeterministicErrors covers the errors that
// mean the request itself is wrong. Sending them again produces the same
// answer, so CreateResource must return after a single attempt instead of
// spending the backoff on a foregone conclusion.
func TestCreateResourceDoesNotRetryDeterministicErrors(t *testing.T) {
	deployments := schema.GroupResource{Group: "apps", Resource: "deployments"}

	cases := []struct {
		name    string
		err     error
		wantErr bool
	}{
		{
			name:    "invalid",
			err:     kubeerr.NewInvalid(schema.GroupKind{Group: "apps", Kind: "Deployment"}, "my-app", field.ErrorList{}),
			wantErr: true,
		},
		{
			name:    "forbidden",
			err:     kubeerr.NewForbidden(deployments, "my-app", errors.New("not allowed")),
			wantErr: true,
		},
		{
			name:    "not found",
			err:     kubeerr.NewNotFound(deployments, "my-app"),
			wantErr: true,
		},
		{
			// an existing resource is the one deterministic response
			// CreateResource reports as success, since the caller's
			// intent is already satisfied.
			name:    "already exists",
			err:     kubeerr.NewAlreadyExists(deployments, "my-app"),
			wantErr: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			shrinkCreateRetryDelay(t)

			client := &stubDynamicClient{
				resourceClient: &stubResourceClient{alwaysErr: tc.err},
			}

			_, err := CreateResource(testDeployment(), client, testDeploymentMapper())
			if tc.wantErr && err == nil {
				t.Fatalf("CreateResource with a %s response: got nil, want error", tc.name)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("CreateResource with a %s response: got err %v, want nil", tc.name, err)
			}
			if client.resourceClient.attempts != 1 {
				t.Fatalf(
					"CreateResource with a %s response: got %d attempts, want 1",
					tc.name, client.resourceClient.attempts,
				)
			}
		})
	}
}

// shrinkCreateRetryDelay drops the backoff between create attempts to
// something a test can wait out, and restores the production value when the
// test finishes.
func shrinkCreateRetryDelay(t *testing.T) {
	t.Helper()

	original := createRetryBaseDelay
	createRetryBaseDelay = time.Millisecond
	t.Cleanup(func() { createRetryBaseDelay = original })
}

// testDeployment returns a minimal deployment for the create path to send.
func testDeployment() *unstructured.Unstructured {
	deployment := &unstructured.Unstructured{}
	deployment.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "apps",
		Version: "v1",
		Kind:    "Deployment",
	})
	deployment.SetName("my-app")
	deployment.SetNamespace("my-app-namespace")

	return deployment
}

// testDeploymentMapper returns a mapper that resolves the Deployment kind,
// the only kind the create path tests send.
func testDeploymentMapper() meta.RESTMapper {
	mapper := meta.NewDefaultRESTMapper([]schema.GroupVersion{{Group: "apps", Version: "v1"}})
	mapper.Add(
		schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"},
		meta.RESTScopeNamespace,
	)

	return mapper
}

// stubCreateResult is what the stub client hands back for one create call.
type stubCreateResult struct {
	object *unstructured.Unstructured
	err    error
}

// stubDynamicClient hands every resource lookup the same stub resource
// client so a test can count create attempts. The embedded interface is
// unimplemented on purpose: any method the create path is not expected to
// call panics rather than quietly returning a zero value.
type stubDynamicClient struct {
	dynamic.Interface
	resourceClient *stubResourceClient
}

func (c *stubDynamicClient) Resource(resource schema.GroupVersionResource) dynamic.NamespaceableResourceInterface {
	return c.resourceClient
}

// stubResourceClient answers create calls from a scripted list of results,
// or with the same error every time when alwaysErr is set, and records how
// many attempts it saw.
type stubResourceClient struct {
	dynamic.NamespaceableResourceInterface
	results   []stubCreateResult
	alwaysErr error
	attempts  int
	existing  *unstructured.Unstructured
	updates   []*unstructured.Unstructured
	created   []*unstructured.Unstructured
}

func (r *stubResourceClient) Namespace(namespace string) dynamic.ResourceInterface {
	return r
}

func (r *stubResourceClient) Create(
	ctx context.Context,
	object *unstructured.Unstructured,
	options kubemetav1.CreateOptions,
	subresources ...string,
) (*unstructured.Unstructured, error) {
	r.attempts++

	// record the object that was sent
	r.created = append(r.created, object.DeepCopy())

	if r.alwaysErr != nil {
		return nil, r.alwaysErr
	}
	if len(r.results) == 0 {
		return nil, errors.New("stub resource client ran out of scripted results")
	}

	result := r.results[0]
	r.results = r.results[1:]

	return result.object, result.err
}

// Get returns the account stored for an already-exists update.
func (r *stubResourceClient) Get(
	ctx context.Context,
	name string,
	options kubemetav1.GetOptions,
	subresources ...string,
) (*unstructured.Unstructured, error) {
	// return the stored account
	if r.existing == nil {
		return nil, fmt.Errorf("service account %s not found", name)
	}
	return r.existing.DeepCopy(), nil
}

// Update records the account written after an already-exists create.
func (r *stubResourceClient) Update(
	ctx context.Context,
	obj *unstructured.Unstructured,
	options kubemetav1.UpdateOptions,
	subresources ...string,
) (*unstructured.Unstructured, error) {
	// record the written account
	copied := obj.DeepCopy()
	r.updates = append(r.updates, copied)
	return copied, nil
}

// errOtherMapperFailure is a distinct error value we can check for by identity.
var errOtherMapperFailure = &distinctErr{msg: "some other mapper failure"}

type distinctErr struct{ msg string }

func (e *distinctErr) Error() string { return e.msg }

// errMapper is a minimal RESTMapper that always returns a fixed error on
// RESTMapping. All other methods are unimplemented (they return zero
// values / nils) because DeleteResource only calls RESTMapping.
type errMapper struct {
	meta.RESTMapper
	err error
}

// RESTMapping returns the error stored on the mapper.
func (m *errMapper) RESTMapping(gk schema.GroupKind, versions ...string) (*meta.RESTMapping, error) {
	return nil, m.err
}

// testServiceAccount returns a ServiceAccount.
func testServiceAccount(name, namespace, role string) *unstructured.Unstructured {
	// set the account identity and its role annotation
	account := &unstructured.Unstructured{}
	account.SetAPIVersion("v1")
	account.SetKind("ServiceAccount")
	account.SetName(name)
	account.SetNamespace(namespace)
	if role != "" {
		account.SetAnnotations(map[string]string{"eks.amazonaws.com/role-arn": role})
	}
	return account
}

// testServiceAccountMapper maps a core ServiceAccount.
func testServiceAccountMapper() meta.RESTMapper {
	// register the core service account kind
	mapper := meta.NewDefaultRESTMapper([]schema.GroupVersion{{Version: "v1"}})
	mapper.Add(schema.GroupVersionKind{Version: "v1", Kind: "ServiceAccount"}, meta.RESTScopeNamespace)
	return mapper
}

// TestCreateWorkloadResourceSetsExistingRole updates an operator account that already exists.
func TestCreateWorkloadResourceSetsExistingRole(t *testing.T) {
	const role = "arn:aws:iam::123:role/dns01"
	accounts := []struct {
		name      string
		namespace string
	}{
		{name: "cert-manager", namespace: "nukleros-certs-system"},
		{name: "external-secrets", namespace: "nukleros-secrets-system"},
	}
	for _, account := range accounts {
		t.Run(account.namespace+"/"+account.name, func(t *testing.T) {
			// store the account the operator already created
			existing := testServiceAccount(account.name, account.namespace, "")
			existing.SetAnnotations(map[string]string{"kept": "yes"})
			desired := testServiceAccount(account.name, account.namespace, role)
			resourceClient := &stubResourceClient{
				results: []stubCreateResult{{
					err: kubeerr.NewAlreadyExists(schema.GroupResource{Resource: "serviceaccounts"}, account.name),
				}},
				existing: existing,
			}
			client := &stubDynamicClient{resourceClient: resourceClient}

			// set the role on the account that already exists
			_, err := CreateWorkloadResource(desired, client, testServiceAccountMapper())
			if err != nil {
				t.Fatalf("CreateWorkloadResource returned %v", err)
			}

			// the written account keeps its existing fields and gains the role
			if len(resourceClient.updates) != 1 {
				t.Fatalf("updates = %d, want 1", len(resourceClient.updates))
			}
			written := resourceClient.updates[0]
			if written.GetAnnotations()["eks.amazonaws.com/role-arn"] != role {
				t.Fatalf("role annotation = %q", written.GetAnnotations()["eks.amazonaws.com/role-arn"])
			}
			if written.GetAnnotations()["kept"] != "yes" {
				t.Fatalf("existing annotation was dropped: %v", written.GetAnnotations())
			}
		})
	}

	// an already-exists create through CreateResource does not update the account
	account := testServiceAccount("cert-manager", "nukleros-certs-system", role)
	plain := &stubResourceClient{
		results: []stubCreateResult{{
			err: kubeerr.NewAlreadyExists(schema.GroupResource{Resource: "serviceaccounts"}, "cert-manager"),
		}},
		existing: testServiceAccount("cert-manager", "nukleros-certs-system", ""),
	}
	_, err := CreateResource(account, &stubDynamicClient{resourceClient: plain}, testServiceAccountMapper())
	if err != nil {
		t.Fatalf("CreateResource returned %v", err)
	}
	if len(plain.updates) != 0 {
		t.Fatalf("CreateResource updated the account")
	}

	// a deployment that already exists is not updated
	deploymentClient := &stubResourceClient{
		results: []stubCreateResult{{
			err: kubeerr.NewAlreadyExists(schema.GroupResource{Group: "apps", Resource: "deployments"}, "app"),
		}},
	}
	_, err = CreateWorkloadResource(
		testDeployment(),
		&stubDynamicClient{resourceClient: deploymentClient},
		testDeploymentMapper(),
	)
	if err != nil {
		t.Fatalf("CreateWorkloadResource deployment returned %v", err)
	}
	if len(deploymentClient.updates) != 0 {
		t.Fatalf("deployment was updated")
	}
}

// TestCreateWorkloadResourceCreatesMissingNamespace creates the operator namespace and then the account.
func TestCreateWorkloadResourceCreatesMissingNamespace(t *testing.T) {
	const role = "arn:aws:iam::123:role/dns01"
	accounts := []struct {
		name      string
		namespace string
	}{
		{name: "cert-manager", namespace: "nukleros-certs-system"},
		{name: "external-secrets", namespace: "nukleros-secrets-system"},
	}
	for _, account := range accounts {
		t.Run(account.namespace+"/"+account.name, func(t *testing.T) {
			// the first create finds no namespace, then the namespace and the account succeed
			desired := testServiceAccount(account.name, account.namespace, role)
			resourceClient := &stubResourceClient{
				results: []stubCreateResult{
					{err: kubeerr.NewNotFound(schema.GroupResource{Resource: "namespaces"}, account.namespace)},
					{},
					{object: desired},
				},
			}

			// create the namespace, then the annotated account
			_, err := CreateWorkloadResource(desired, &stubDynamicClient{resourceClient: resourceClient}, testServiceAccountMapper())
			if err != nil {
				t.Fatalf("CreateWorkloadResource returned %v", err)
			}
			if len(resourceClient.created) != 3 {
				t.Fatalf("creates = %d, want 3", len(resourceClient.created))
			}
			namespace := resourceClient.created[1]
			if namespace.GetKind() != "Namespace" || namespace.GetName() != account.namespace {
				t.Fatalf("namespace = %s/%s", namespace.GetKind(), namespace.GetName())
			}
			if resourceClient.created[2].GetAnnotations()["eks.amazonaws.com/role-arn"] != role {
				t.Fatalf("role annotation = %q", resourceClient.created[2].GetAnnotations()["eks.amazonaws.com/role-arn"])
			}
		})
	}

	// a deployment whose namespace is missing stays a deployment error
	deploymentClient := &stubResourceClient{
		results: []stubCreateResult{{
			err: kubeerr.NewNotFound(schema.GroupResource{Resource: "namespaces"}, "my-app-namespace"),
		}},
	}
	_, err := CreateWorkloadResource(
		testDeployment(),
		&stubDynamicClient{resourceClient: deploymentClient},
		testDeploymentMapper(),
	)
	if err == nil {
		t.Fatalf("CreateWorkloadResource returned nil, want error")
	}
	for _, object := range deploymentClient.created {
		if object.GetKind() == "Namespace" {
			t.Fatalf("created a namespace for a deployment")
		}
	}
}
