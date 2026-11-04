package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	gce "google.golang.org/api/compute/v1"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/option"
)

// gceClaimNamespaceKey is the key in the JSON description the GCE persistent
// disk driver gives a disk it provisions that names the namespace of the claim
// it was created for.
const gceClaimNamespaceKey = "kubernetes.io/created-for/pvc/namespace"

// gceUnspecifiedProject is the project some versions of the GCE persistent
// disk driver put in a volume handle, meaning the cluster's own project.
const gceUnspecifiedProject = "UNSPECIFIED"

// gceOperationPollInterval is how often a disk deletion is checked on while
// it runs.
var gceOperationPollInterval = 2 * time.Second

// gceDisk is a GCE persistent disk, zonal or regional, as a volume handle
// names it.
type gceDisk struct {
	project string
	zone    string
	region  string
	name    string
}

// String returns the disk as a volume handle names it.
func (d gceDisk) String() string {
	if d.region != "" {
		return fmt.Sprintf("projects/%s/regions/%s/disks/%s", d.project, d.region, d.name)
	}

	return fmt.Sprintf("projects/%s/zones/%s/disks/%s", d.project, d.zone, d.name)
}

// parseGceDiskHandle reads the disk a GCE persistent disk volume handle names:
// projects/<project>/zones/<zone>/disks/<name>, or regions/<region> for a
// regional disk. A handle with no project of its own is in projectID.
func parseGceDiskHandle(handle, projectID string) (gceDisk, error) {
	parts := strings.Split(handle, "/")
	if len(parts) != 6 || parts[0] != "projects" || parts[4] != "disks" ||
		parts[1] == "" || parts[3] == "" || parts[5] == "" {
		return gceDisk{}, fmt.Errorf("%q is not a GCE persistent disk", handle)
	}

	disk := gceDisk{project: parts[1], name: parts[5]}
	if disk.project == gceUnspecifiedProject {
		disk.project = projectID
	}
	switch parts[2] {
	case "zones":
		disk.zone = parts[3]
	case "regions":
		disk.region = parts[3]
	default:
		return gceDisk{}, fmt.Errorf("%q is not a GCE persistent disk", handle)
	}

	return disk, nil
}

// gceDiskApi is the part of the Compute Engine API the leftover disk cleanup
// uses.
type gceDiskApi interface {
	getDisk(ctx context.Context, disk gceDisk) (*gce.Disk, error)
	deleteDisk(ctx context.Context, disk gceDisk) error
}

// DeleteLeftoverGkeDisks deletes the GCE persistent disks named by handles -
// the volume handles of the control plane's persistent volumes that were to be
// deleted with their claims, read before the cluster was - that are still
// there once the cluster is gone, and returns the handles it deleted.
//
// GKE puts nothing on a disk that says which cluster it was provisioned for,
// and every control plane's claims are in the same namespace, so a disk is
// never looked for: only the ones this cluster named are deleted. Each is
// checked again first, and left alone unless its description names a claim in
// namespace and nothing is using it.
func DeleteLeftoverGkeDisks(
	ctx context.Context,
	projectID string,
	namespace string,
	handles []string,
	serviceAccountCredentials string,
) ([]string, error) {
	if len(handles) == 0 {
		return nil, nil
	}

	clientOpts := []option.ClientOption{option.WithScopes(gce.ComputeScope)}
	if serviceAccountCredentials != "" {
		clientOpts = append(clientOpts, option.WithCredentialsJSON([]byte(serviceAccountCredentials)))
	}
	service, err := gce.NewService(ctx, clientOpts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create Compute Engine client: %w", err)
	}

	return deleteLeftoverGkeDisks(ctx, computeDiskApi{service: service}, projectID, namespace, handles)
}

// deleteLeftoverGkeDisks is DeleteLeftoverGkeDisks against any disk API.
func deleteLeftoverGkeDisks(
	ctx context.Context,
	diskApi gceDiskApi,
	projectID string,
	namespace string,
	handles []string,
) ([]string, error) {
	var deleted []string
	var deleteErrors []error
	for _, handle := range handles {
		disk, err := parseGceDiskHandle(handle, projectID)
		if err != nil {
			deleteErrors = append(deleteErrors, err)
			continue
		}

		found, err := diskApi.getDisk(ctx, disk)
		if isGoogleApiNotFound(err) {
			continue
		}
		if err != nil {
			deleteErrors = append(deleteErrors, fmt.Errorf("failed to get disk %s: %w", disk, err))
			continue
		}
		if reason := keepGceDisk(found, namespace); reason != "" {
			deleteErrors = append(deleteErrors, fmt.Errorf("left disk %s alone: %s", disk, reason))
			continue
		}

		if err := diskApi.deleteDisk(ctx, disk); err != nil && !isGoogleApiNotFound(err) {
			deleteErrors = append(deleteErrors, fmt.Errorf("failed to delete disk %s: %w", disk, err))
			continue
		}
		deleted = append(deleted, disk.String())
	}

	return deleted, errors.Join(deleteErrors...)
}

// keepGceDisk returns why a disk is not one the cleanup deletes, or nothing
// when it is: its description has to name a claim in namespace, the way the
// GCE persistent disk driver describes a disk it provisions, and nothing can
// be using it.
func keepGceDisk(disk *gce.Disk, namespace string) string {
	var description map[string]string
	if err := json.Unmarshal([]byte(disk.Description), &description); err != nil ||
		description[gceClaimNamespaceKey] != namespace {
		return fmt.Sprintf("it was not provisioned for a claim in namespace %s", namespace)
	}
	if len(disk.Users) > 0 {
		return fmt.Sprintf("it is in use by %s", strings.Join(disk.Users, ", "))
	}

	return ""
}

// isGoogleApiNotFound reports whether err is a Google API's not found.
func isGoogleApiNotFound(err error) bool {
	var apiErr *googleapi.Error

	return errors.As(err, &apiErr) && apiErr.Code == http.StatusNotFound
}

// computeDiskApi is gceDiskApi on the Compute Engine API.
type computeDiskApi struct {
	service *gce.Service
}

// getDisk gets a disk.
func (c computeDiskApi) getDisk(ctx context.Context, disk gceDisk) (*gce.Disk, error) {
	if disk.region != "" {
		return c.service.RegionDisks.Get(disk.project, disk.region, disk.name).Context(ctx).Do()
	}

	return c.service.Disks.Get(disk.project, disk.zone, disk.name).Context(ctx).Do()
}

// deleteDisk deletes a disk and waits for the deletion to finish, so a disk
// reported deleted is gone.
func (c computeDiskApi) deleteDisk(ctx context.Context, disk gceDisk) error {
	var operation *gce.Operation
	var err error
	if disk.region != "" {
		operation, err = c.service.RegionDisks.Delete(disk.project, disk.region, disk.name).Context(ctx).Do()
	} else {
		operation, err = c.service.Disks.Delete(disk.project, disk.zone, disk.name).Context(ctx).Do()
	}
	if err != nil {
		return err
	}

	for operation.Status != "DONE" {
		select {
		case <-ctx.Done():
			return fmt.Errorf("deletion did not finish: %w", ctx.Err())
		case <-time.After(gceOperationPollInterval):
		}
		if disk.region != "" {
			operation, err = c.service.RegionOperations.Get(disk.project, disk.region, operation.Name).Context(ctx).Do()
		} else {
			operation, err = c.service.ZoneOperations.Get(disk.project, disk.zone, operation.Name).Context(ctx).Do()
		}
		if err != nil {
			return fmt.Errorf("failed to check on the deletion: %w", err)
		}
	}
	if operation.Error != nil && len(operation.Error.Errors) > 0 {
		return fmt.Errorf("deletion failed: %s", operation.Error.Errors[0].Message)
	}

	return nil
}
