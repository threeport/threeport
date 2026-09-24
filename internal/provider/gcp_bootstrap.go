package provider

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"google.golang.org/api/cloudresourcemanager/v1"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/iam/v1"
	"google.golang.org/api/option"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	gcpauth "github.com/threeport/threeport/pkg/auth/v0"
	installer "github.com/threeport/threeport/pkg/threeport-installer/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// GCP resource naming constants to ensure consistency between create and delete operations
const (
	// serviceAccountNameFormat is the format for all Threeport GCP service accounts
	serviceAccountNameFormat = "threeport-svc-%s"
	// serviceAccountDisplayFormat is the display name format for all Threeport service accounts
	serviceAccountDisplayFormat = "Threeport Service Account for %s"
	// workloadIdentityPoolFormat is the format for Workload Identity pool names
	workloadIdentityPoolFormat = "%s.svc.id.goog"
)

// GCP IAM roles required for Threeport to manage GCP resources
var threeportServiceAccountRoles = []string{
	// GKE cluster management
	"roles/container.admin",
	// VPC, subnets, and routes
	"roles/compute.networkAdmin",
	// GCE VMs for the machine provider
	"roles/compute.instanceAdmin.v1",
	// firewall rules; networkAdmin does not grant them
	"roles/compute.securityAdmin",
	// IAM management for creating service accounts for workloads
	"roles/iam.serviceAccountAdmin",
	"roles/iam.serviceAccountUser",
	// Resource Manager for project access
	"roles/resourcemanager.projectIamAdmin",
	// Service account token creator for Workload Identity
	"roles/iam.workloadIdentityUser",
}

// ServiceAccountInfo holds information about the created GCP service account
type ServiceAccountInfo struct {
	// The email address of the service account
	Email string
	// The unique ID of the service account
	UniqueID string
	// The full resource name of the service account
	Name string
}

// GCPServiceStatus represents the current status of a GCP service propagation check
type GCPServiceStatus struct {
	Name                 string
	ConsecutiveSuccesses int
	Attempts             int
	LastError            error
	Completed            bool
	Failed               bool
}

// GCPServiceAccountWithKey holds the service account info and its exported key
type GCPServiceAccountWithKey struct {
	// The email address of the service account
	Email string
	// The JSON key for the service account (base64 decoded)
	KeyJSON string
}

// createGCPServiceAccountAndCredentials creates a GCP service account with the necessary
// permissions for Threeport to manage GCP resources. It follows GKE best practices by
// using Workload Identity for authentication instead of service account keys.
func (i *KubernetesRuntimeInfraGKE) createGCPServiceAccountAndCredentials() error {
	ctx := context.Background()

	// Create IAM service client
	iamService, err := iam.NewService(ctx, option.WithScopes(iam.CloudPlatformScope))
	if err != nil {
		return fmt.Errorf("failed to create IAM service client: %w", err)
	}

	// Create Cloud Resource Manager service client for IAM bindings
	crmService, err := cloudresourcemanager.NewService(ctx, option.WithScopes(cloudresourcemanager.CloudPlatformScope))
	if err != nil {
		return fmt.Errorf("failed to create Cloud Resource Manager service client: %w", err)
	}

	// Create the service account
	if err := i.createGCPServiceAccount(iamService); err != nil {
		return fmt.Errorf("failed to create service account: %w", err)
	}

	// Grant IAM roles to the service account
	if err := i.grantServiceAccountRoles(crmService); err != nil {
		return fmt.Errorf("failed to grant IAM roles: %w", err)
	}

	// Validate service account propagation
	if err := i.validateGCPServiceAccountPropagation(iamService, crmService); err != nil {
		return fmt.Errorf("failed to validate service account propagation: %w", err)
	}

	return nil
}

// createGCPServiceAccount creates a new GCP service account for Threeport operations.
func (i *KubernetesRuntimeInfraGKE) createGCPServiceAccount(iamService *iam.Service) error {
	// reject a runtime name that would collide after service-account ID truncation
	if !canonicalGCPAccountName(i.RuntimeInstanceName) {
		return fmt.Errorf("GCP runtime instance name %q is not a unique service-account identity; use lowercase letters, digits, and hyphens", i.RuntimeInstanceName)
	}

	serviceAccountID := i.getServiceAccountID()
	displayName := fmt.Sprintf(serviceAccountDisplayFormat, i.RuntimeInstanceName)
	description := gkeServiceAccountDescription(i.RuntimeInstanceName)

	// create or reuse the named account
	account, existed, err := createServiceAccountForProject(iamService, i.ProjectID, serviceAccountID, displayName, description)
	if err != nil {
		return err
	}

	// refuse an existing account this runtime did not create
	if err := refuseUnownedExistingGCPServiceAccount(account, existed, i.RuntimeInstanceName); err != nil {
		return err
	}

	i.ServiceAccountEmail = account.Email
	return nil
}

// grantServiceAccountRoles grants the necessary IAM roles to the Threeport service account.
func (i *KubernetesRuntimeInfraGKE) grantServiceAccountRoles(crmService *cloudresourcemanager.Service) error {
	return grantServiceAccountRolesForProject(crmService, i.ProjectID, i.ServiceAccountEmail)
}

// configureWorkloadIdentityBinding creates the IAM binding that allows Kubernetes
// service accounts to impersonate the GCP service account via Workload Identity.
// This should be called after the GKE cluster is created.
// requireServiceAccountEmail reports whether the account a workload identity
// binding names is known.
//
// It is derived when the infra is built, from the provider's stored credentials
// or from the ambient identity this controller runs as. Reaching here without it
// means neither was available, and the binding would otherwise be attempted
// against a resource path with nothing in it.
func (i *KubernetesRuntimeInfraGKE) requireServiceAccountEmail() error {
	if i.ServiceAccountEmail == "" {
		return errors.New(
			"no GCP service account is known for this runtime, so the workload identity binding has no account to name",
		)
	}

	return nil
}

func (i *KubernetesRuntimeInfraGKE) configureWorkloadIdentityBinding(iamService *iam.Service) error {
	// An empty email builds "projects/<project>/serviceAccounts/" and asks GCP
	// about a resource that does not name anything, which comes back as a 404
	// describing nothing. Say what is actually missing instead.
	if err := i.requireServiceAccountEmail(); err != nil {
		return err
	}

	// Get the current IAM policy for the service account
	serviceAccountResource := fmt.Sprintf("projects/%s/serviceAccounts/%s", i.ProjectID, i.ServiceAccountEmail)

	policy, err := iamService.Projects.ServiceAccounts.GetIamPolicy(serviceAccountResource).Do()
	if err != nil {
		return fmt.Errorf("failed to get service account IAM policy: %w", err)
	}

	// Construct the Workload Identity member
	// Format: serviceAccount:PROJECT_ID.svc.id.goog[NAMESPACE/KSA_NAME]
	workloadIdentityPool := fmt.Sprintf(workloadIdentityPoolFormat, i.ProjectID)

	// Add bindings for the Threeport controller service accounts
	workloadIdentityMembers := i.getWorkloadIdentityMembers(workloadIdentityPool)

	// Check if workloadIdentityUser binding already exists
	var workloadIdentityBinding *iam.Binding
	for _, binding := range policy.Bindings {
		if binding.Role == "roles/iam.workloadIdentityUser" {
			workloadIdentityBinding = binding
			break
		}
	}

	if workloadIdentityBinding == nil {
		// Create new binding
		workloadIdentityBinding = &iam.Binding{
			Role:    "roles/iam.workloadIdentityUser",
			Members: []string{},
		}
		policy.Bindings = append(policy.Bindings, workloadIdentityBinding)
	}

	// Add any missing members
	for _, member := range workloadIdentityMembers {
		memberExists := false
		for _, existingMember := range workloadIdentityBinding.Members {
			if existingMember == member {
				memberExists = true
				break
			}
		}
		if !memberExists {
			workloadIdentityBinding.Members = append(workloadIdentityBinding.Members, member)
		}
	}

	// Set the updated policy
	_, err = iamService.Projects.ServiceAccounts.SetIamPolicy(
		serviceAccountResource,
		&iam.SetIamPolicyRequest{Policy: policy},
	).Do()
	if err != nil {
		return fmt.Errorf("failed to set service account IAM policy: %w", err)
	}

	return nil
}

// getWorkloadIdentityMembers returns the list of Kubernetes service accounts
// that should be allowed to impersonate the GCP service account.
func (i *KubernetesRuntimeInfraGKE) getWorkloadIdentityMembers(workloadIdentityPool string) []string {
	// These are the Threeport controller service accounts that need GCP access
	controllerServiceAccounts := []string{
		installer.ThreeportGcpControllerName,
	}

	members := make([]string, 0, len(controllerServiceAccounts))
	for _, sa := range controllerServiceAccounts {
		// Format: serviceAccount:PROJECT_ID.svc.id.goog[NAMESPACE/KSA_NAME]
		member := fmt.Sprintf("serviceAccount:%s[%s/%s]", workloadIdentityPool, installer.ControlPlaneNamespace, sa)
		members = append(members, member)
	}

	return members
}

// validateGCPServiceAccountPropagation validates that the service account and its
// permissions have been fully propagated across GCP services.
func (i *KubernetesRuntimeInfraGKE) validateGCPServiceAccountPropagation(
	iamService *iam.Service,
	crmService *cloudresourcemanager.Service,
) error {
	const requiredConsecutiveSuccesses = 3
	const maxAttempts = 60
	const retryDelay = 2 * time.Second

	services := []struct {
		id   string
		name string
		test func() error
	}{
		{
			id:   "iam",
			name: "IAM service",
			test: func() error {
				_, err := iamService.Projects.ServiceAccounts.Get(
					fmt.Sprintf("projects/%s/serviceAccounts/%s", i.ProjectID, i.ServiceAccountEmail),
				).Do()
				return err
			},
		},
		{
			id:   "crm",
			name: "Resource Manager service",
			test: func() error {
				_, err := crmService.Projects.GetIamPolicy(i.ProjectID, &cloudresourcemanager.GetIamPolicyRequest{}).Do()
				return err
			},
		},
	}

	// Initialize status map
	statusMap := make(map[string]*GCPServiceStatus)
	for _, service := range services {
		statusMap[service.id] = &GCPServiceStatus{
			Name: service.name,
		}
	}

	// Channel for status updates
	statusChan := make(chan struct {
		serviceID string
		status    GCPServiceStatus
	}, 100)

	// Start all services in parallel
	var wg sync.WaitGroup
	for _, service := range services {
		wg.Add(1)
		go func(svc struct {
			id   string
			name string
			test func() error
		}) {
			defer wg.Done()

			consecutiveSuccesses := 0
			attempts := 0

			for consecutiveSuccesses < requiredConsecutiveSuccesses && attempts < maxAttempts {
				attempts++

				err := svc.test()
				if err != nil {
					consecutiveSuccesses = 0 // reset on failure
					statusChan <- struct {
						serviceID string
						status    GCPServiceStatus
					}{
						serviceID: svc.id,
						status: GCPServiceStatus{
							Name:                 svc.name,
							ConsecutiveSuccesses: consecutiveSuccesses,
							Attempts:             attempts,
							LastError:            err,
							Completed:            false,
							Failed:               false,
						},
					}
					time.Sleep(retryDelay)
				} else {
					consecutiveSuccesses++
					completed := consecutiveSuccesses >= requiredConsecutiveSuccesses
					statusChan <- struct {
						serviceID string
						status    GCPServiceStatus
					}{
						serviceID: svc.id,
						status: GCPServiceStatus{
							Name:                 svc.name,
							ConsecutiveSuccesses: consecutiveSuccesses,
							Attempts:             attempts,
							LastError:            nil,
							Completed:            completed,
							Failed:               false,
						},
					}
					if !completed {
						time.Sleep(1 * time.Second)
					}
				}
			}

			// Mark as failed if max attempts reached
			if consecutiveSuccesses < requiredConsecutiveSuccesses {
				statusChan <- struct {
					serviceID string
					status    GCPServiceStatus
				}{
					serviceID: svc.id,
					status: GCPServiceStatus{
						Name:                 svc.name,
						ConsecutiveSuccesses: consecutiveSuccesses,
						Attempts:             attempts,
						LastError:            fmt.Errorf("max attempts reached"),
						Completed:            false,
						Failed:               true,
					},
				}
			}
		}(service)
	}

	// Close channel when all goroutines are done
	go func() {
		wg.Wait()
		close(statusChan)
	}()

	// Check for failures
	for _, status := range statusMap {
		if status.Failed {
			return fmt.Errorf("%s failed to propagate", status.Name)
		}
	}

	return nil
}

// removeServiceAccountRoles removes the IAM roles granted to the Threeport service account.
func (i *KubernetesRuntimeInfraGKE) removeServiceAccountRoles(crmService *cloudresourcemanager.Service) error {
	serviceAccountEmail := i.getServiceAccountEmail()
	member := fmt.Sprintf("serviceAccount:%s", serviceAccountEmail)

	// Get current IAM policy
	policy, err := crmService.Projects.GetIamPolicy(i.ProjectID, &cloudresourcemanager.GetIamPolicyRequest{}).Do()
	if err != nil {
		return fmt.Errorf("failed to get IAM policy: %w", err)
	}

	// Remove the service account from all bindings
	for _, binding := range policy.Bindings {
		newMembers := make([]string, 0, len(binding.Members))
		for _, m := range binding.Members {
			if m != member {
				newMembers = append(newMembers, m)
			}
		}
		binding.Members = newMembers
	}

	// Remove empty bindings
	newBindings := make([]*cloudresourcemanager.Binding, 0, len(policy.Bindings))
	for _, binding := range policy.Bindings {
		if len(binding.Members) > 0 {
			newBindings = append(newBindings, binding)
		}
	}
	policy.Bindings = newBindings

	// Set the updated policy
	_, err = crmService.Projects.SetIamPolicy(i.ProjectID, &cloudresourcemanager.SetIamPolicyRequest{
		Policy: policy,
	}).Do()
	if err != nil {
		return fmt.Errorf("failed to set IAM policy: %w", err)
	}

	return nil
}

// deleteGCPServiceAccount deletes the GCP service account.
func (i *KubernetesRuntimeInfraGKE) deleteGCPServiceAccount(iamService *iam.Service) error {
	serviceAccountEmail := i.getServiceAccountEmail()

	_, err := iamService.Projects.ServiceAccounts.Delete(
		fmt.Sprintf("projects/%s/serviceAccounts/%s", i.ProjectID, serviceAccountEmail),
	).Do()
	if err != nil {
		if isNotFoundError(err) {
			// Service account not found, skipping deletion
			return nil
		}
		return fmt.Errorf("failed to delete service account: %w", err)
	}

	return nil
}

// getServiceAccountID returns the service account ID (without the email domain).
func (i *KubernetesRuntimeInfraGKE) getServiceAccountID() string {
	return formatServiceAccountID(serviceAccountNameFormat, i.RuntimeInstanceName)
}

// getServiceAccountEmail returns the full service account email address.
func (i *KubernetesRuntimeInfraGKE) getServiceAccountEmail() string {
	return fmt.Sprintf("%s@%s.iam.gserviceaccount.com", i.getServiceAccountID(), i.ProjectID)
}

// CreateGCPServiceAccountWithKey creates a GCP service account with the necessary
// permissions for Threeport to manage GCP resources, and exports a JSON key.
// This is used when creating a GcpProvider via tptctl, where we need to store
// the service account credentials for use by controllers running outside GCP.
//
// Parameters:
//   - projectID: The GCP project ID where the service account will be created
//   - accountName: A name to use for the service account (will be prefixed with "threeport-")
//
// Returns the service account info including the JSON key credentials.
func CreateGCPServiceAccountWithKey(projectID, accountName string) (*GCPServiceAccountWithKey, error) {
	ctx := context.Background()

	// Ensure GCP authentication is in place (uses browser flow if needed for CLI)
	if err := gcpauth.EnsureGCPAuth(""); err != nil {
		return nil, fmt.Errorf("failed to ensure GCP authentication: %w", err)
	}

	// Create IAM service client
	iamService, err := iam.NewService(ctx, option.WithScopes(iam.CloudPlatformScope))
	if err != nil {
		return nil, fmt.Errorf("failed to create IAM service client: %w", err)
	}

	// Create Cloud Resource Manager service client for IAM bindings
	crmService, err := cloudresourcemanager.NewService(ctx, option.WithScopes(cloudresourcemanager.CloudPlatformScope))
	if err != nil {
		return nil, fmt.Errorf("failed to create Cloud Resource Manager service client: %w", err)
	}

	if !canonicalGCPAccountName(accountName) {
		return nil, fmt.Errorf("GCP provider name %q is not a unique service-account identity; use lowercase letters, digits, and hyphens", accountName)
	}

	// Generate service account ID and create the service account
	serviceAccountID := generateServiceAccountID(accountName)
	displayName := fmt.Sprintf(serviceAccountDisplayFormat, accountName)
	description := fmt.Sprintf(
		"Service account for Threeport GcpProvider %s to manage GCP resources; %s",
		accountName,
		GcpOwnershipDescription(accountName),
	)

	account, existed, err := createServiceAccountForProject(iamService, projectID, serviceAccountID, displayName, description)
	if err != nil {
		return nil, fmt.Errorf("failed to create service account: %w", err)
	}

	if err := refuseUnownedExistingGCPServiceAccount(account, existed, accountName); err != nil {
		return nil, err
	}

	// grant IAM roles to the service account
	if err := grantServiceAccountRolesForProject(crmService, projectID, account.Email); err != nil {
		return nil, fmt.Errorf("failed to grant IAM roles: %w", err)
	}

	// create JSON key for controllers running outside GCP
	keyRequest := &iam.CreateServiceAccountKeyRequest{
		PrivateKeyType: "TYPE_GOOGLE_CREDENTIALS_FILE",
	}

	key, err := iamService.Projects.ServiceAccounts.Keys.Create(
		fmt.Sprintf("projects/%s/serviceAccounts/%s", projectID, account.Email),
		keyRequest,
	).Do()
	if err != nil {
		return nil, fmt.Errorf("failed to create service account key: %w", err)
	}

	// The key is base64 encoded, decode it
	keyJSON, err := util.Base64Decode(key.PrivateKeyData)
	if err != nil {
		return nil, fmt.Errorf("failed to decode service account key: %w", err)
	}

	util.CliOutputInfo("Created and exported GCP service account key")

	return &GCPServiceAccountWithKey{
		Email:   account.Email,
		KeyJSON: keyJSON,
	}, nil
}

// DeleteGCPServiceAccountWithKey deletes a GCP service account that was created
// for a GcpProvider. This removes the IAM role bindings and deletes the service account.
// This is used when deleting a GcpProvider via tptctl.
//
// Parameters:
//   - projectID: The GCP project ID where the service account exists
//   - accountName: The name used when creating the service account (will be prefixed with "threeport-")
func DeleteGCPServiceAccountWithKey(projectID, accountName string) error {
	ctx := context.Background()

	// Ensure GCP authentication is in place (uses browser flow if needed for CLI)
	if err := gcpauth.EnsureGCPAuth(""); err != nil {
		return fmt.Errorf("failed to ensure GCP authentication: %w", err)
	}

	// Create IAM service client
	iamService, err := iam.NewService(ctx, option.WithScopes(iam.CloudPlatformScope))
	if err != nil {
		return fmt.Errorf("failed to create IAM service client: %w", err)
	}

	// Create Cloud Resource Manager service client for IAM bindings
	crmService, err := cloudresourcemanager.NewService(ctx, option.WithScopes(cloudresourcemanager.CloudPlatformScope))
	if err != nil {
		return fmt.Errorf("failed to create Cloud Resource Manager service client: %w", err)
	}

	// Generate service account ID and email
	serviceAccountID := generateServiceAccountID(accountName)
	serviceAccountEmail := fmt.Sprintf("%s@%s.iam.gserviceaccount.com", serviceAccountID, projectID)

	// Remove IAM role bindings for the service account
	if err := removeServiceAccountRolesForProject(crmService, projectID, serviceAccountEmail); err != nil {
		return fmt.Errorf("failed to remove IAM roles: %w", err)
	}

	// Delete the service account
	if err := deleteServiceAccountForProject(iamService, projectID, serviceAccountEmail); err != nil {
		return fmt.Errorf("failed to delete service account: %w", err)
	}

	return nil
}

// createServiceAccountForProject returns the named service account, creating it
// when Get reports not found. The bool is true when the account already existed.
func createServiceAccountForProject(
	iamService *iam.Service,
	projectID string,
	serviceAccountID string,
	displayName string,
	description string,
) (*iam.ServiceAccount, bool, error) {
	serviceAccountEmail := fmt.Sprintf("%s@%s.iam.gserviceaccount.com", serviceAccountID, projectID)
	serviceAccountResource := fmt.Sprintf("projects/%s/serviceAccounts/%s", projectID, serviceAccountEmail)

	// report an existing account; CreateGCPServiceAccountWithKey reuses an owned one
	existingAccount, err := iamService.Projects.ServiceAccounts.Get(serviceAccountResource).Do()
	if err == nil {
		return existingAccount, true, nil
	}

	// treat only not-found as a create; other Get errors fail the call
	if !isNotFoundError(err) {
		return nil, false, fmt.Errorf("failed to check for existing service account: %w", err)
	}

	// create the service account
	createRequest := &iam.CreateServiceAccountRequest{
		AccountId: serviceAccountID,
		ServiceAccount: &iam.ServiceAccount{
			DisplayName: displayName,
			Description: description,
		},
	}

	account, err := iamService.Projects.ServiceAccounts.Create(
		fmt.Sprintf("projects/%s", projectID),
		createRequest,
	).Do()
	if err != nil {
		return nil, false, wrapIAMServiceAccountCreateError(projectID, err)
	}

	fmt.Printf("Created GCP service account: %s\n", account.Email)

	return account, false, nil
}

// serviceAccountOwnedBy reports whether the account description records the
// same ownership pair GcpResourceLabels would put on a Compute resource.
func serviceAccountOwnedBy(account *iam.ServiceAccount, ownerName string) bool {
	if account == nil {
		return false
	}
	return strings.HasSuffix(account.Description, GcpOwnershipDescription(ownerName))
}

// gkeServiceAccountDescription is the IAM description written for a GKE bootstrap account.
func gkeServiceAccountDescription(runtimeInstanceName string) string {
	return fmt.Sprintf(
		"Service account for Threeport instance %s to manage GCP resources; %s",
		runtimeInstanceName,
		GcpOwnershipDescription(runtimeInstanceName),
	)
}

// refuseUnownedExistingGCPServiceAccount rejects reuse of an account this owner did not create.
func refuseUnownedExistingGCPServiceAccount(account *iam.ServiceAccount, existed bool, ownerName string) error {
	if !existed {
		return nil
	}
	if serviceAccountOwnedBy(account, ownerName) {
		return nil
	}

	email := ""
	if account != nil {
		email = account.Email
	}
	return fmt.Errorf("GCP service account %s already exists; pick a different name", email)
}

// removeServiceAccountRolesForProject removes all IAM roles granted to a service account.
func removeServiceAccountRolesForProject(crmService *cloudresourcemanager.Service, projectID, serviceAccountEmail string) error {
	member := fmt.Sprintf("serviceAccount:%s", serviceAccountEmail)

	// Get current IAM policy
	policy, err := crmService.Projects.GetIamPolicy(projectID, &cloudresourcemanager.GetIamPolicyRequest{}).Do()
	if err != nil {
		return fmt.Errorf("failed to get IAM policy: %w", err)
	}

	// Remove the service account from all bindings
	for _, binding := range policy.Bindings {
		newMembers := make([]string, 0, len(binding.Members))
		for _, m := range binding.Members {
			if m != member {
				newMembers = append(newMembers, m)
			}
		}
		binding.Members = newMembers
	}

	// Remove empty bindings
	newBindings := make([]*cloudresourcemanager.Binding, 0, len(policy.Bindings))
	for _, binding := range policy.Bindings {
		if len(binding.Members) > 0 {
			newBindings = append(newBindings, binding)
		}
	}
	policy.Bindings = newBindings

	// Set the updated policy
	_, err = crmService.Projects.SetIamPolicy(projectID, &cloudresourcemanager.SetIamPolicyRequest{
		Policy: policy,
	}).Do()
	if err != nil {
		return fmt.Errorf("failed to set IAM policy: %w", err)
	}

	fmt.Printf("Removed IAM roles from service account %s\n", serviceAccountEmail)
	return nil
}

// deleteServiceAccountForProject deletes a GCP service account.
func deleteServiceAccountForProject(iamService *iam.Service, projectID, serviceAccountEmail string) error {
	_, err := iamService.Projects.ServiceAccounts.Delete(
		fmt.Sprintf("projects/%s/serviceAccounts/%s", projectID, serviceAccountEmail),
	).Do()
	if err != nil {
		if isNotFoundError(err) {
			// Service account not found, skipping deletion
			fmt.Printf("Service account %s not found, skipping deletion\n", serviceAccountEmail)
			return nil
		}
		return fmt.Errorf("failed to delete service account: %w", err)
	}

	fmt.Printf("Deleted GCP service account %s\n", serviceAccountEmail)
	return nil
}

func wrapIAMServiceAccountCreateError(projectID string, invokeErr error) error {
	if invokeErr == nil {
		return nil
	}
	if isIAMServiceAccountCreatePermissionDenied(invokeErr) {
		return fmt.Errorf("failed to create service account: %w%s", invokeErr, formatIAMServiceAccountCreate403Hint(projectID))
	}
	return fmt.Errorf("failed to create service account: %w", invokeErr)
}

func isIAMServiceAccountCreatePermissionDenied(err error) bool {
	var gerr *googleapi.Error
	if !errors.As(err, &gerr) || gerr.Code != 403 {
		return false
	}
	aggregate := strings.ToLower(gerr.Message)
	for _, e := range gerr.Errors {
		aggregate += " " + strings.ToLower(e.Message) + " " + strings.ToLower(e.Reason)
	}
	return strings.Contains(aggregate, "iam.serviceaccounts.create")
}

func formatIAMServiceAccountCreate403Hint(projectID string) string {
	return fmt.Sprintf(
		"\n\nHint: Your credentials may lack permission to create service accounts in project %q. "+
			"This commonly happens when the Google account used for Application Default Credentials "+
			"does not match the account in your active gcloud configuration (`gcloud config get-value account`) "+
			"while project and region are read from gcloud, or when the principal needs roles such as Owner, "+
			"Editor, or IAM Administrator. Confirm the project ID and pass --gcp-project-id if needed.",
		projectID,
	)
}

// formatServiceAccountID generates a valid GCP service account ID from the given
// format string and name. GCP service account IDs must be 6-30 characters,
// contain only lowercase letters, digits, and hyphens, and start with a letter.
func formatServiceAccountID(format, name string) string {
	id := fmt.Sprintf(format, name)
	// Convert to lowercase
	id = strings.ToLower(id)
	// Replace invalid characters with hyphens
	var result strings.Builder
	for _, c := range id {
		if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '-' {
			result.WriteRune(c)
		} else {
			result.WriteRune('-')
		}
	}
	id = result.String()
	// Truncate if necessary (6-30 char limit)
	if len(id) > 30 {
		id = id[:30]
	}
	// Ensure it ends with alphanumeric (remove trailing hyphens)
	id = strings.TrimRight(id, "-")
	// Ensure minimum length (6 chars required)
	if len(id) < 6 {
		id = id + "-svc"
	}
	return id
}

// isNotFoundError checks if the error is a "not found" error.
func isNotFoundError(err error) bool {
	if err == nil {
		return false
	}

	// Check for gRPC status code
	if s, ok := status.FromError(err); ok {
		return s.Code() == codes.NotFound
	}

	// Check for HTTP 404 in error message
	errStr := err.Error()
	return strings.Contains(errStr, "404") || strings.Contains(errStr, "notFound") || strings.Contains(errStr, "Not Found")
}

// generateServiceAccountID generates a valid GCP service account ID from the given name.
// This is used when creating service accounts via tptctl create gcp-provider.
func generateServiceAccountID(name string) string {
	return formatServiceAccountID(serviceAccountNameFormat, name)
}

// canonicalGCPAccountName reports whether name is already the unique
// lowercase [a-z0-9-] form, and whether generateServiceAccountID keeps it
// without truncating to 30 characters.
func canonicalGCPAccountName(name string) bool {
	if name == "" {
		return false
	}
	for _, r := range name {
		if (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '-' {
			return false
		}
	}
	id := generateServiceAccountID(name)
	full := strings.ToLower(fmt.Sprintf(serviceAccountNameFormat, name))
	return id == full
}

// grantServiceAccountRolesForProject grants the necessary IAM roles to a service account.
// It includes retry logic for the SetIamPolicy call to handle GCP's eventual consistency
// when the service account may not have fully propagated yet.
func grantServiceAccountRolesForProject(crmService *cloudresourcemanager.Service, projectID, serviceAccountEmail string) error {
	const maxRetries = 20
	const retryDelay = 3 * time.Second

	// Get current IAM policy
	policy, err := crmService.Projects.GetIamPolicy(projectID, &cloudresourcemanager.GetIamPolicyRequest{}).Do()
	if err != nil {
		return fmt.Errorf("failed to get IAM policy: %w", err)
	}

	member := fmt.Sprintf("serviceAccount:%s", serviceAccountEmail)

	// Check and add each required role
	for _, role := range threeportServiceAccountRoles {
		// Check if binding already exists
		bindingExists := false
		for _, binding := range policy.Bindings {
			if binding.Role == role {
				// Check if member is already in the binding
				for _, m := range binding.Members {
					if m == member {
						bindingExists = true
						break
					}
				}
				if !bindingExists {
					// Add member to existing binding
					binding.Members = append(binding.Members, member)
					bindingExists = true
				}
				break
			}
		}

		// If no binding exists for this role, create one
		if !bindingExists {
			policy.Bindings = append(policy.Bindings, &cloudresourcemanager.Binding{
				Role:    role,
				Members: []string{member},
			})
		}
	}

	// Set the updated policy with retry logic to handle service account propagation delays
	var lastErr error
	for attempt := 1; attempt <= maxRetries; attempt++ {
		_, err = crmService.Projects.SetIamPolicy(projectID, &cloudresourcemanager.SetIamPolicyRequest{
			Policy: policy,
		}).Do()
		if err == nil {
			fmt.Printf("Granted IAM roles to service account %s\n", serviceAccountEmail)
			return nil
		}

		lastErr = err
		if attempt < maxRetries {
			fmt.Printf("SetIamPolicy failed (attempt %d/%d): %v, retrying in %v...\n", attempt, maxRetries, err, retryDelay)
			time.Sleep(retryDelay)
		}
	}

	return fmt.Errorf("failed to set IAM policy after %d attempts: %w", maxRetries, lastErr)
}

// workloadIdentityBindingOutlivesRuntime reports whether the account a runtime's
// workload identity binding was made on survives the runtime's deletion.
//
// The account deleted with a runtime is derived from its name, and its policy
// goes with it. Any other account - the identity this control plane runs as,
// when the provider stored no credentials - stays, and so does the grant.
func (i *KubernetesRuntimeInfraGKE) workloadIdentityBindingOutlivesRuntime() bool {
	return i.ServiceAccountEmail != "" && i.ServiceAccountEmail != i.getServiceAccountEmail()
}

// revokeWorkloadIdentityBinding removes this runtime's workload identity members
// from the account they were granted on, when that account is not one this
// delete removes outright.
func (i *KubernetesRuntimeInfraGKE) revokeWorkloadIdentityBinding(iamService *iam.Service) error {
	if !i.workloadIdentityBindingOutlivesRuntime() {
		return nil
	}

	serviceAccountResource := fmt.Sprintf("projects/%s/serviceAccounts/%s", i.ProjectID, i.ServiceAccountEmail)
	policy, err := iamService.Projects.ServiceAccounts.GetIamPolicy(serviceAccountResource).Do()
	if err != nil {
		if isNotFoundError(err) {
			return nil
		}
		return fmt.Errorf("failed to get service account IAM policy: %w", err)
	}

	workloadIdentityPool := fmt.Sprintf(workloadIdentityPoolFormat, i.ProjectID)
	revoked := removeWorkloadIdentityMembers(policy, i.getWorkloadIdentityMembers(workloadIdentityPool))
	if !revoked {
		return nil
	}

	if _, err := iamService.Projects.ServiceAccounts.SetIamPolicy(
		serviceAccountResource,
		&iam.SetIamPolicyRequest{Policy: policy},
	).Do(); err != nil {
		return fmt.Errorf("failed to set service account IAM policy: %w", err)
	}

	return nil
}

// removeWorkloadIdentityMembers drops the given members from a policy's
// workloadIdentityUser binding and reports whether anything changed. Members
// another runtime put there are left alone.
func removeWorkloadIdentityMembers(policy *iam.Policy, members []string) bool {
	drop := make(map[string]struct{}, len(members))
	for _, member := range members {
		drop[member] = struct{}{}
	}

	changed := false
	for _, binding := range policy.Bindings {
		if binding.Role != "roles/iam.workloadIdentityUser" {
			continue
		}
		kept := make([]string, 0, len(binding.Members))
		for _, member := range binding.Members {
			if _, remove := drop[member]; remove {
				changed = true
				continue
			}
			kept = append(kept, member)
		}
		binding.Members = kept
	}

	return changed
}
