package v0

import (
	"testing"

	kubeerr "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

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
