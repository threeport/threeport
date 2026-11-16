package provider

import (
	"context"
	"fmt"
	"slices"

	registryapi "google.golang.org/api/artifactregistry/v1"
	"google.golang.org/api/option"
)

// imageRepositoryManagerRole lets an account change who can use one
// repository, which a cluster's stack needs to grant its nodes read access.
const imageRepositoryManagerRole = "roles/artifactregistry.admin"

// repositorySetPolicyPermission is the permission a cluster's stack needs on
// the image repository to grant its nodes read access.
const repositorySetPolicyPermission = "artifactregistry.repositories.setIamPolicy"

// resourceName returns the API name of the repository.
func (r artifactRegistryRepository) resourceName() string {
	return fmt.Sprintf("projects/%s/locations/%s/repositories/%s", r.project, r.location, r.name)
}

// GrantImageRepositoryManager lets a service account manage access to the
// Artifact Registry repository the image namespace names, so the stacks it
// runs can grant their nodes read access. It does nothing for a namespace on
// another registry or an empty one.
func GrantImageRepositoryManager(imageNamespace, serviceAccountEmail string) error {
	repository, ok := parseArtifactRegistryNamespace(imageNamespace)
	if !ok {
		return nil
	}

	return grantRepositoryRole(
		context.Background(),
		[]option.ClientOption{option.WithScopes(registryapi.CloudPlatformScope)},
		repository,
		imageRepositoryManagerRole,
		"serviceAccount:"+serviceAccountEmail,
	)
}

// CanManageImageRepositoryAccess reports whether the credentials can change
// access to the Artifact Registry repository the image namespace names. Empty
// credentials use the ambient identity. It reports false for a namespace on
// another registry.
func CanManageImageRepositoryAccess(imageNamespace, credentials string) (bool, error) {
	repository, ok := parseArtifactRegistryNamespace(imageNamespace)
	if !ok {
		return false, nil
	}

	clientOptions := []option.ClientOption{option.WithScopes(registryapi.CloudPlatformScope)}
	if credentials != "" {
		clientOptions = append(clientOptions, option.WithCredentialsJSON([]byte(credentials)))
	}

	return holdsRepositoryPermission(
		context.Background(),
		clientOptions,
		repository,
		repositorySetPolicyPermission,
	)
}

// grantRepositoryRole adds a member to a role on a repository. It leaves the
// policy as it is when the member already holds the role.
func grantRepositoryRole(
	ctx context.Context,
	clientOptions []option.ClientOption,
	repository artifactRegistryRepository,
	role string,
	member string,
) error {
	service, err := registryapi.NewService(ctx, clientOptions...)
	if err != nil {
		return fmt.Errorf("failed to create artifact registry client: %w", err)
	}

	// read the repository's policy
	resource := repository.resourceName()
	policy, err := service.Projects.Locations.Repositories.GetIamPolicy(resource).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("failed to read the policy of repository %s: %w", resource, err)
	}

	// add the member to the role's unconditional binding
	granted := false
	for _, binding := range policy.Bindings {
		if binding.Role != role || binding.Condition != nil {
			continue
		}
		if slices.Contains(binding.Members, member) {
			return nil
		}
		binding.Members = append(binding.Members, member)
		granted = true
		break
	}
	if !granted {
		policy.Bindings = append(policy.Bindings, &registryapi.Binding{
			Role:    role,
			Members: []string{member},
		})
	}

	// write the policy back with the etag it was read under
	if _, err := service.Projects.Locations.Repositories.SetIamPolicy(
		resource,
		&registryapi.SetIamPolicyRequest{Policy: policy},
	).Context(ctx).Do(); err != nil {
		return fmt.Errorf("failed to grant %s on repository %s to %s: %w", role, resource, member, err)
	}

	return nil
}

// holdsRepositoryPermission reports whether the calling identity holds a
// permission on a repository.
func holdsRepositoryPermission(
	ctx context.Context,
	clientOptions []option.ClientOption,
	repository artifactRegistryRepository,
	permission string,
) (bool, error) {
	service, err := registryapi.NewService(ctx, clientOptions...)
	if err != nil {
		return false, fmt.Errorf("failed to create artifact registry client: %w", err)
	}

	resource := repository.resourceName()
	response, err := service.Projects.Locations.Repositories.TestIamPermissions(
		resource,
		&registryapi.TestIamPermissionsRequest{Permissions: []string{permission}},
	).Context(ctx).Do()
	if err != nil {
		return false, fmt.Errorf("failed to test permissions on repository %s: %w", resource, err)
	}

	return slices.Contains(response.Permissions, permission), nil
}
