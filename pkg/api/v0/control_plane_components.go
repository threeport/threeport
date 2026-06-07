// +threeport-sdk route-exclude
package v0

import "gorm.io/datatypes"

type ControlPlaneComponent struct {
	Common `swaggerignore:"true" mapstructure:",squash"`

	// The control plane instance ID that this component belongs to.
	ControlPlaneInstanceID *uint `json:",omitempty" validate:"required" gorm:"not null" relationship:"requires"`

	// Indicate whether the component is enabled to be deployed. Currently only respected by controllers.
	Enabled *bool `json:",omitempty" validate:"optional"`

	// The name of the component.
	Name string `json:",omitempty" validate:"required" gorm:"not null"`

	// The binary name of the component.
	BinaryName string `json:",omitempty" validate:"optional"`

	// The image registry and namespace of the control planecomponent.  If the complete image
	// reference is 'ghcr.io/threeport/threeport-rest-api:v0.6.1', the ImageNamespace
	// component would be 'ghcr.io/threeport'.
	ImageNamespace string `json:",omitempty" validate:"optional"`

	// The image name of the control plane component.  If the complete image
	// reference is 'ghcr.io/threeport/threeport-rest-api:v0.6.1', the ImageName
	// component would be 'threeport-rest-api'.
	ImageName string `json:",omitempty" validate:"optional"`

	// The image tag of the control plane component.  If the complete image
	// reference is 'ghcr.io/threeport/threeport-rest-api:v0.6.1', the ImageTag
	// component would be 'v0.6.1'.
	ImageTag string `json:",omitempty" validate:"optional"`

	// The service account name to use when deploying.
	ServiceAccountName string `json:",omitempty" validate:"optional"`

	// The service resource name to use when deploying.
	ServiceResourceName string `json:",omitempty" validate:"optional"`

	// The name of the secret with credentials to pull a private container image.
	ImagePullSecretName string `json:",omitempty" validate:"optional"`

	// The additional volumes to be added to the deployment spec of the component.
	AdditionalVolumes *datatypes.JSON `json:",omitempty" validate:"optional"`

	// The additional volume mounts to be added to the deployment spec of the component.
	AdditionalVolumeMounts *datatypes.JSON `json:",omitempty" validate:"optional"`

	// The additional env reference to be added to the environment variables of the component.
	AdditionalEnvRef *datatypes.JSON `json:",omitempty" validate:"optional"`
}
