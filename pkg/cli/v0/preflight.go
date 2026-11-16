package v0

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	api "github.com/threeport/threeport/pkg/api/v0"
	client "github.com/threeport/threeport/pkg/client/v0"
	installer "github.com/threeport/threeport/pkg/threeport-installer/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// BlueGreenPreflight names what a blue-green upgrade is about to do, so the
// preflight can check it before any writer stops.
type BlueGreenPreflight struct {
	// The control plane being replaced. Empty means the connected control plane.
	SourceName string
	// The name of the new control plane.
	NewName string
	// The image namespace and tag of the new control plane. An empty namespace
	// keeps the one the source records, and an empty tag is not checked.
	ImageNamespace string
	ImageTag       string
	// The module a module upgrade installs, with the image coordinates the
	// command was given. Empty for a control plane upgrade.
	ModuleName           string
	ModuleImageNamespace string
	ModuleImageTag       string
}

// imageExistsCheck looks an image up in its registry. Tests replace it.
var imageExistsCheck = registryImageExists

// preflightRun collects the results of the checks a preflight runs.
type preflightRun struct {
	failures []error
}

// check runs one check, prints its result, and records a failure. A warning
// reports something the check could not confirm and does not fail it.
func (run *preflightRun) check(name string, fn func() (string, error)) bool {
	warning, err := fn()
	switch {
	case err != nil:
		fmt.Printf("Error: preflight %s: %v\n", name, err)
		run.failures = append(run.failures, fmt.Errorf("%s: %w", name, err))
		return false
	case warning != "":
		fmt.Printf("Warning: preflight %s: %s\n", name, warning)
	default:
		fmt.Printf("Info: preflight %s: ok\n", name)
	}

	return true
}

// PreflightBlueGreen checks what a blue-green upgrade needs before it stops any
// writer. It runs every check and returns one error naming each that failed.
func PreflightBlueGreen(
	currentClient *http.Client,
	currentEndpoint string,
	plan BlueGreenPreflight,
) error {
	run := &preflightRun{}

	// the connected control plane has to be one that can run an upgrade
	run.check("connected control plane is not a child", func() (string, error) {
		self, err := client.GetSelfControlPlaneInstance(currentClient, currentEndpoint)
		if err != nil {
			return "", fmt.Errorf("failed to get control plane instance: %w", err)
		}

		return "", api.ChildControlPlaneRequest(self)
	})

	// the source has to exist, and the new name has to be free
	var source *ControlPlaneTarget
	sourceFound := run.check("source control plane exists", func() (string, error) {
		target, err := resolveControlPlane(currentClient, currentEndpoint, plan.SourceName, false)
		source = target
		return "", err
	})
	run.check("new control plane name is free", func() (string, error) {
		found, lookupErr := client.GetControlPlaneInstanceByName(currentClient, currentEndpoint, plan.NewName)
		sourceID := uint(0)
		if source != nil && source.Instance != nil && source.Instance.ID != nil {
			sourceID = *source.Instance.ID
		}

		return "", ReplacementNameTaken(sourceID, plan.NewName, found, lookupErr)
	})

	// checks that read the source row
	if sourceFound {
		if plan.SourceName != "" {
			run.check("new name differs from the child's current name", func() (string, error) {
				return "", refuseSameChildName(source.Instance, plan.NewName)
			})
		}
		run.check("no upgrade is pending on the source", func() (string, error) {
			return "", pendingUpgrade(source.Instance)
		})
		run.check("new namespace differs from the source", func() (string, error) {
			if plan.NewName == util.DerefString(source.Instance.Namespace) {
				return "", fmt.Errorf("new name %s is the namespace the source runs in", plan.NewName)
			}

			return "", nil
		})
	}

	// the images the new control plane and the module pull have to exist
	var images []string
	if sourceFound {
		images = append(images, controlPlaneImages(source.Instance, plan.ImageNamespace, plan.ImageTag)...)
	}
	if plan.ModuleName != "" {
		moduleImages, moduleOK := moduleUpgradeImages(run, source, sourceFound, plan)
		if moduleOK {
			images = append(images, moduleImages...)
		}
	}
	run.check("images exist in the registry", func() (string, error) {
		return missingImages(context.Background(), images, imageExistsCheck)
	})

	return errors.Join(run.failures...)
}

// pendingUpgrade returns an error when the source already carries a request. A
// child renamed to its namespace that still records a source is waiting on its
// module install, and the way forward is a reinstall, not a second blue-green.
func pendingUpgrade(instance *api.ControlPlaneInstance) error {
	// a recorded source namespace means an upgrade has not finished
	from := util.DerefString(instance.FromNamespace)
	namespace := util.DerefString(instance.Namespace)
	if from != "" && from != namespace {
		if util.DerefString(instance.Name) == namespace {
			return fmt.Errorf(
				"control plane %s is waiting on its module install and keeps the old control plane %s: "+
					"reinstall the module on it with the corrected image, which removes %s when it succeeds",
				namespace, from, from,
			)
		}
		return fmt.Errorf("a blue-green request replacing %s is already pending", from)
	}
	if util.DerefString(instance.ModuleInstall) != "" {
		return errors.New("a module install request is already pending")
	}

	return nil
}

// controlPlaneImages returns the images every control plane pulls at the new
// tag, plus the control plane controller on a control plane with no parent. A
// tag left empty resolves at run time and is not checked.
func controlPlaneImages(source *api.ControlPlaneInstance, imageNamespace, imageTag string) []string {
	// default to the image namespace the source records
	if imageNamespace == "" {
		imageNamespace = util.DerefString(source.ImageNamespace)
	}
	if imageNamespace == "" || imageTag == "" {
		return nil
	}

	names := []string{
		installer.ThreeportAPIImage,
		installer.ThreeportDatabaseMigratorImage,
		installer.ThreeportAgentImage,
	}
	if source.ParentControlPlaneInstanceID == nil {
		names = append(names, "threeport-"+installer.ThreeportControlPlaneControllerName)
	}

	var images []string
	for _, name := range names {
		images = append(images, fmt.Sprintf("%s/%s:%s", imageNamespace, name, imageTag))
	}

	return images
}

// moduleUpgradeImages checks the module record a module upgrade will use and
// returns the module images to look up. The record has to give an image
// namespace, from the command or from the record itself, and a tag.
func moduleUpgradeImages(
	run *preflightRun,
	source *ControlPlaneTarget,
	sourceFound bool,
	plan BlueGreenPreflight,
) ([]string, bool) {
	var images []string
	ok := run.check("module record has an image to install", func() (string, error) {
		if !sourceFound {
			return "skipped: the source was not found", nil
		}

		// read the namespace the module records, when the command gives none
		namespace := plan.ModuleImageNamespace
		if namespace == "" {
			module, err := client.GetModuleApiByName(source.APIClient, source.APIEndpoint, plan.ModuleName)
			if err != nil {
				return "", fmt.Errorf("failed to get module %s: %w", plan.ModuleName, err)
			}
			namespace = util.DerefString(module.ImageNamespace)
		}
		if namespace == "" {
			return "", fmt.Errorf("module %s records no image namespace: pass --module-image-namespace", plan.ModuleName)
		}
		if plan.ModuleImageTag == "" {
			return "", fmt.Errorf("module %s has no image tag: pass --module-image-tag", plan.ModuleName)
		}
		// the plugin image runs only in the install job a child upgrade records
		images = installer.ModuleInstallImages(namespace, plan.ModuleName, plan.ModuleImageTag, plan.SourceName != "")

		return "", nil
	})

	return images, ok
}

// missingImages looks each image up and fails when any is missing from its
// registry. An image the lookup could not reach is a warning, not a failure,
// because it may still exist.
func missingImages(
	ctx context.Context,
	images []string,
	exists func(context.Context, string) (bool, error),
) (string, error) {
	var missing, unchecked []string
	for _, image := range images {
		found, err := exists(ctx, image)
		switch {
		case err != nil:
			unchecked = append(unchecked, image)
		case !found:
			missing = append(missing, image)
		}
	}
	if len(missing) > 0 {
		return "", fmt.Errorf("not found in the registry: %s", strings.Join(missing, ", "))
	}
	if len(unchecked) > 0 {
		return fmt.Sprintf("could not check %d image(s): %s", len(unchecked), strings.Join(unchecked, ", ")), nil
	}

	return "", nil
}
