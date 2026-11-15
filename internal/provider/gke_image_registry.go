package provider

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/pulumi/pulumi-gcp/sdk/v8/go/gcp/artifactregistry"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// artifactRegistryReaderRole lets an account pull images from one repository.
const artifactRegistryReaderRole = "roles/artifactregistry.reader"

// artifactRegistryHost matches a regional Artifact Registry Docker host and
// captures its location.
var artifactRegistryHost = regexp.MustCompile(`^([a-z0-9-]+)-docker\.pkg\.dev$`)

// artifactRegistryRepository names one Artifact Registry repository.
type artifactRegistryRepository struct {
	location string
	project  string
	name     string
}

// parseArtifactRegistryNamespace reads the repository from an image namespace
// of the form <location>-docker.pkg.dev/<project>/<repository>. It reports
// false for any other registry.
func parseArtifactRegistryNamespace(imageNamespace string) (artifactRegistryRepository, bool) {
	// split the host from the path
	parts := strings.Split(strings.Trim(imageNamespace, "/"), "/")
	if len(parts) < 3 {
		return artifactRegistryRepository{}, false
	}

	// require an Artifact Registry host
	match := artifactRegistryHost.FindStringSubmatch(parts[0])
	if match == nil {
		return artifactRegistryRepository{}, false
	}

	return artifactRegistryRepository{
		location: match[1],
		project:  parts[1],
		name:     parts[2],
	}, true
}

// imageReaderGrant lets the nodes' service account pull the control plane
// images from the Artifact Registry repository the image namespace names.
// It returns nil when the namespace is not an Artifact Registry repository.
func (i *KubernetesRuntimeInfraGKE) imageReaderGrant(
	ctx *pulumi.Context,
	gcpProvider pulumi.ProviderResource,
	nodeAccountEmail pulumi.StringOutput,
) (pulumi.Resource, error) {
	// skip a registry the cluster's account has nothing to grant on
	repository, ok := parseArtifactRegistryNamespace(i.ImageNamespace)
	if !ok {
		return nil, nil
	}

	grant, err := artifactregistry.NewRepositoryIamMember(
		ctx,
		fmt.Sprintf("%s-node-sa-image-reader", i.RuntimeInstanceName),
		&artifactregistry.RepositoryIamMemberArgs{
			Project:    pulumi.String(repository.project),
			Location:   pulumi.String(repository.location),
			Repository: pulumi.String(repository.name),
			Role:       pulumi.String(artifactRegistryReaderRole),
			Member:     pulumi.Sprintf("serviceAccount:%s", nodeAccountEmail),
		},
		pulumi.Provider(gcpProvider),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to grant the nodes read access to the image repository: %w", err)
	}

	return grant, nil
}
