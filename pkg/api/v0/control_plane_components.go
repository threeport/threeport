// +threeport-codegen route-exclude
package v0

type ControlPlaneComponents struct {
	Common `swaggerignore:"true" mapstructure:",squash"`

	ControlPlaneInstanceID *uint  `json:"ControlPlaneInstanceID,omitempty" gorm:"not null" validate:"required"`
	Enabled                *bool  `json:"Enabled,omitempty" yaml:"Enabled" validate:"optional"`
	Name                   string `json:"Name,omitempty" yaml:"Name" query:"name" gorm:"not null" validate:"required"`
	ImageName              string `json:"ImageName,omitempty" yaml:"ImageName" query:"imagename" validate:"optional"`
	ImageRepo              string `json:"ImageRepo,omitempty" yaml:"ImageRepo" query:"imagerepo" validate:"optional"`
	ImageTag               string `json:"ImageTag,omitempty" yaml:"ImageTag" query:"imagetag" validate:"optional"`
	ServiceAccountName     string `json:"ServiceAccountName,omitempty" yaml:"ServiceAccountName" query:"serviceaccountname" validate:"optional"`
	ServiceResourceName    string `json:"ServiceResourceName,omitempty" yaml:"ServiceResourceName" query:"serviceresourcename" validate:"optional"`
}
