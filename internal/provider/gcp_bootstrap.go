package provider

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"google.golang.org/api/cloudresourcemanager/v1"
	"google.golang.org/api/iam/v1"
	"google.golang.org/api/option"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	installer "github.com/threeport/threeport/pkg/threeport-installer/v0"
)

// GCP resource naming constants to ensure consistency between create and delete operations
const (
	serviceAccountNameFormat    = "threeport-svc-%s"
	serviceAccountDisplayFormat = "Threeport Service Account for %s"
	workloadIdentityPoolFormat  = "%s.svc.id.goog"
)

// GCP IAM roles required for Threeport to manage GCP resources
var threeportServiceAccountRoles = []string{
	// GKE cluster management
	"roles/container.admin",
	// Compute Engine resources (for VPC, subnets, firewalls)
	"roles/compute.networkAdmin",
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
	serviceAccountID := i.getServiceAccountID()
	serviceAccountEmail := i.getServiceAccountEmail()

	// Check if service account already exists
	existingAccount, err := iamService.Projects.ServiceAccounts.Get(
		fmt.Sprintf("projects/%s/serviceAccounts/%s", i.ProjectID, serviceAccountEmail),
	).Do()
	if err == nil {
		// Service account exists, store the info
		i.ServiceAccountEmail = existingAccount.Email
		return nil
	}

	// Check if error is "not found" - if so, create the account
	if !isNotFoundError(err) {
		return fmt.Errorf("failed to check for existing service account: %w", err)
	}

	// Create new service account
	createRequest := &iam.CreateServiceAccountRequest{
		AccountId: serviceAccountID,
		ServiceAccount: &iam.ServiceAccount{
			DisplayName: fmt.Sprintf(serviceAccountDisplayFormat, i.RuntimeInstanceName),
			Description: fmt.Sprintf("Service account for Threeport instance %s to manage GCP resources", i.RuntimeInstanceName),
		},
	}

	account, err := iamService.Projects.ServiceAccounts.Create(
		fmt.Sprintf("projects/%s", i.ProjectID),
		createRequest,
	).Do()
	if err != nil {
		return fmt.Errorf("failed to create service account: %w", err)
	}

	i.ServiceAccountEmail = account.Email
	return nil
}

// grantServiceAccountRoles grants the necessary IAM roles to the Threeport service account.
func (i *KubernetesRuntimeInfraGKE) grantServiceAccountRoles(crmService *cloudresourcemanager.Service) error {
	// Get current IAM policy
	policy, err := crmService.Projects.GetIamPolicy(i.ProjectID, &cloudresourcemanager.GetIamPolicyRequest{}).Do()
	if err != nil {
		return fmt.Errorf("failed to get IAM policy: %w", err)
	}

	member := fmt.Sprintf("serviceAccount:%s", i.ServiceAccountEmail)

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

	// Set the updated policy
	_, err = crmService.Projects.SetIamPolicy(i.ProjectID, &cloudresourcemanager.SetIamPolicyRequest{
		Policy: policy,
	}).Do()
	if err != nil {
		return fmt.Errorf("failed to set IAM policy: %w", err)
	}

	return nil
}

// configureWorkloadIdentityBinding creates the IAM binding that allows Kubernetes
// service accounts to impersonate the GCP service account via Workload Identity.
// This should be called after the GKE cluster is created.
func (i *KubernetesRuntimeInfraGKE) configureWorkloadIdentityBinding(iamService *iam.Service) error {
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
func (i *KubernetesRuntimeInfraGKE) validateGCPServiceAccountPropagation(iamService *iam.Service, crmService *cloudresourcemanager.Service) error {
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

// DeleteGCPResources deletes all GCP resources created for this Threeport instance.
func (i *KubernetesRuntimeInfraGKE) DeleteGCPResources() error {
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

	// Remove IAM role bindings
	if err := i.removeServiceAccountRoles(crmService); err != nil {
		return fmt.Errorf("failed to remove IAM roles: %w", err)
	}

	// Delete the service account
	if err := i.deleteGCPServiceAccount(iamService); err != nil {
		return fmt.Errorf("failed to delete service account: %w", err)
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
	// GCP service account IDs must be 6-30 characters, contain only lowercase
	// letters, digits, and hyphens, and start with a letter
	name := fmt.Sprintf(serviceAccountNameFormat, i.RuntimeInstanceName)
	// Truncate if necessary (6-30 char limit)
	if len(name) > 30 {
		name = name[:30]
	}
	// Ensure it ends with alphanumeric (remove trailing hyphens)
	name = strings.TrimRight(name, "-")
	return strings.ToLower(name)
}

// getServiceAccountEmail returns the full service account email address.
func (i *KubernetesRuntimeInfraGKE) getServiceAccountEmail() string {
	return fmt.Sprintf("%s@%s.iam.gserviceaccount.com", i.getServiceAccountID(), i.ProjectID)
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
