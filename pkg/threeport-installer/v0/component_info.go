package v0

import (
	"fmt"

	"gorm.io/datatypes"

	v0 "github.com/threeport/threeport/pkg/api/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// DefaultComponentNames returns the name of every component a control plane
// installs by default: the controllers, the rest api, the agent and the
// database migrator.
func DefaultComponentNames() []string {
	names := make([]string, 0, len(ThreeportControllerList)+3)
	for _, controller := range ThreeportControllerList {
		names = append(names, controller.Name)
	}

	return append(
		names,
		ThreeportRestApi.Name,
		ThreeportAgent.Name,
		DatabaseMigrator.Name,
	)
}

// IsDefaultComponent returns true if name is one of the components a control
// plane installs by default.
func IsDefaultComponent(name string) bool {
	for _, defaultName := range DefaultComponentNames() {
		if name == defaultName {
			return true
		}
	}

	return false
}

// copyComponent returns a copy of a component that shares no memory with the
// original, so a write to one is never seen through the other.
//
// The copy describes what to install, not a database row: the identity of the
// original (its ID, timestamps and owning control plane instance) is dropped.
// A component read from the API carries them, and a copy that kept them would
// be written back under the wrong owner when the install is recorded.
func copyComponent(component *v0.ControlPlaneComponent) *v0.ControlPlaneComponent {
	if component == nil {
		return nil
	}

	copied := *component
	copied.Common = v0.Common{}
	copied.ControlPlaneInstanceID = nil
	copied.Enabled = copyPtr(component.Enabled)
	copied.Additional = copyPtr(component.Additional)
	copied.AdditionalVolumes = copyJSON(component.AdditionalVolumes)
	copied.AdditionalVolumeMounts = copyJSON(component.AdditionalVolumeMounts)
	copied.AdditionalEnvRef = copyJSON(component.AdditionalEnvRef)

	return &copied
}

// copyComponents returns a copy of each component in a new slice.
func copyComponents(components []*v0.ControlPlaneComponent) []*v0.ControlPlaneComponent {
	copied := make([]*v0.ControlPlaneComponent, len(components))
	for i, component := range components {
		copied[i] = copyComponent(component)
	}

	return copied
}

// copyPtr returns a pointer to a copy of the value p points to.
func copyPtr[T any](p *T) *T {
	if p == nil {
		return nil
	}
	value := *p

	return &value
}

// copyJSON returns a copy of a JSON value that has its own backing bytes.
func copyJSON(j *datatypes.JSON) *datatypes.JSON {
	if j == nil {
		return nil
	}
	copied := make(datatypes.JSON, len(*j))
	copy(copied, *j)

	return &copied
}

// ValidateComponentOverrides checks the component info a control plane
// instance supplies to customize its install.
//
// A name that matches a default component overrides that component. A name
// that matches none is a component of the instance's own, which is how custom
// controllers are added, so it has to say so with Additional and has to say
// what to run. Anything else is rejected: an override with a mistyped name
// installs nothing it was meant to and, supplying only a tag, would add a
// controller with an image of "/:tag" that never starts.
func ValidateComponentOverrides(overrides []*v0.ControlPlaneComponent) error {
	multiErr := util.MultiError{}
	seen := make(map[string]bool, len(overrides))

	for i, override := range overrides {
		if override == nil {
			multiErr.AppendError(fmt.Errorf("component info at index %d is empty", i))
			continue
		}
		if override.Name == "" {
			multiErr.AppendError(fmt.Errorf("component info at index %d has no name", i))
			continue
		}
		if seen[override.Name] {
			multiErr.AppendError(fmt.Errorf("component %q is given more than once", override.Name))
			continue
		}
		seen[override.Name] = true

		additional := util.Deref(override.Additional)
		switch {
		case IsDefaultComponent(override.Name) && additional:
			multiErr.AppendError(fmt.Errorf(
				"component %q is a default component of the control plane, so it cannot be marked Additional",
				override.Name,
			))
		case IsDefaultComponent(override.Name):
			// an override of a default component
		case !additional:
			multiErr.AppendError(fmt.Errorf(
				"component %q is not a default component of the control plane (%v): "+
					"fix the name, or set Additional to add a component of your own",
				override.Name, DefaultComponentNames(),
			))
		default:
			if override.ImageName == "" || override.ImageNamespace == "" || override.ImageTag == "" {
				multiErr.AppendError(fmt.Errorf(
					"additional component %q must set ImageName, ImageNamespace and ImageTag",
					override.Name,
				))
			}
		}
	}

	return multiErr.Error()
}

// ConfigureChildComponents sets the components a child control plane installs.
//
// Each source overrides the one before it:
//
//  1. the compiled-in defaults the installer starts with;
//  2. the images the parent control plane runs, as the parent recorded them
//     for itself, and the parent's own instance-wide image;
//  3. the instance-wide image of the child;
//  4. the component info the child supplies.
//
// Without 2 a child pulls ghcr.io images at the version this controller was
// built from, whatever its parent runs: a development image, a private mirror
// or an air-gapped registry the child cannot reach.
//
// Only what the parent runs is inherited. Its image pull secrets live in its
// own namespace, and a component it disabled or added for itself is not a
// decision about the child.
func (cpi *ControlPlaneInstaller) ConfigureChildComponents(
	parent *v0.ControlPlaneInstance,
	child *v0.ControlPlaneInstance,
) error {
	if err := ValidateComponentOverrides(child.CustomComponentInfo); err != nil {
		return fmt.Errorf("invalid component info for control plane instance: %w", err)
	}

	components := cpi.defaultComponents()

	if parent != nil {
		for _, recorded := range parent.CustomComponentInfo {
			if recorded == nil {
				continue
			}
			if component, ok := components[recorded.Name]; ok {
				setImage(component, recorded.ImageName, recorded.ImageNamespace, recorded.ImageTag)
			}
		}
		setInstanceImage(components, parent)
	}
	setInstanceImage(components, child)

	for _, override := range child.CustomComponentInfo {
		component, exists := components[override.Name]
		if !exists {
			custom := copyComponent(override)
			if custom.Enabled == nil {
				custom.Enabled = util.Ptr(true)
			}
			cpi.Opts.ControllerList = append(cpi.Opts.ControllerList, custom)
			continue
		}

		setImage(component, override.ImageName, override.ImageNamespace, override.ImageTag)

		if override.ServiceAccountName != "" {
			component.ServiceAccountName = override.ServiceAccountName
		}
		if override.ServiceResourceName != "" {
			component.ServiceResourceName = override.ServiceResourceName
		}
		if override.ImagePullSecretName != "" {
			component.ImagePullSecretName = override.ImagePullSecretName
		}
		if override.Enabled != nil {
			component.Enabled = copyPtr(override.Enabled)
		}
	}

	return nil
}

// RecordedComponents returns the components this installer installs as the
// control plane records them for itself: each controller, the rest api, the
// agent and the database migrator, as copies.
//
// A control plane's children read this record to learn what images their
// parent runs. A controller that is not a default component is marked
// Additional, since that is what the record of it has to say for it to be
// accepted back as component info.
func (cpi *ControlPlaneInstaller) RecordedComponents() []*v0.ControlPlaneComponent {
	recorded := make([]*v0.ControlPlaneComponent, 0, len(cpi.Opts.ControllerList)+3)
	for _, controller := range copyComponents(cpi.Opts.ControllerList) {
		if !IsDefaultComponent(controller.Name) {
			controller.Additional = util.Ptr(true)
		}
		recorded = append(recorded, controller)
	}
	for _, component := range []*v0.ControlPlaneComponent{
		cpi.Opts.RestApiInfo,
		cpi.Opts.AgentInfo,
		cpi.Opts.DatabaseMigratorInfo,
	} {
		if component != nil {
			recorded = append(recorded, copyComponent(component))
		}
	}

	return recorded
}

// defaultComponents returns the default components of this installer by name.
func (cpi *ControlPlaneInstaller) defaultComponents() map[string]*v0.ControlPlaneComponent {
	components := make(map[string]*v0.ControlPlaneComponent, len(cpi.Opts.ControllerList)+3)
	for _, controller := range cpi.Opts.ControllerList {
		components[controller.Name] = controller
	}
	for _, component := range []*v0.ControlPlaneComponent{
		cpi.Opts.RestApiInfo,
		cpi.Opts.AgentInfo,
		cpi.Opts.DatabaseMigratorInfo,
	} {
		if component != nil {
			components[component.Name] = component
		}
	}

	return components
}

// setImage sets each part of a component's image that is not empty.
func setImage(component *v0.ControlPlaneComponent, imageName, imageNamespace, imageTag string) {
	if imageName != "" {
		component.ImageName = imageName
	}
	if imageNamespace != "" {
		component.ImageNamespace = imageNamespace
	}
	if imageTag != "" {
		component.ImageTag = imageTag
	}
}

// setInstanceImage sets the instance-wide image of a control plane instance on
// each of the components.
func setInstanceImage(components map[string]*v0.ControlPlaneComponent, instance *v0.ControlPlaneInstance) {
	for _, component := range components {
		setImage(component, "", util.DerefString(instance.ImageNamespace), util.DerefString(instance.ImageTag))
	}
}
