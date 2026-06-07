package v0

// ControlPlaneDefinition is the configuration for a Threeport Control Plane.
type ControlPlaneDefinition struct {
	Common         `swaggerignore:"true" mapstructure:",squash"`
	Definition     `mapstructure:",squash"`
	Reconciliation `mapstructure:",squash"`

	// Used to indicate whether the control plane is deployed with auth settings
	AuthEnabled *bool `json:",omitempty" validate:"optional" gorm:"default:true"`

	// When instances of this control plane are deployed, Object representing control plane and its parent are
	// onboarded as part of deployment, using this we can disable that process and simply spin a new instance with
	// a clean DB.
	OnboardParent *bool `json:",omitempty" validate:"optional" gorm:"default:true"`

	// The associated control plane instances that are deployed from this definition.
	ControlPlaneInstances []*ControlPlaneInstance `json:",omitempty" validate:"optional,association"`
}

// ControlPlaneInstance is the instance for a deployed Threeport Control Plane.
type ControlPlaneInstance struct {
	Common         `swaggerignore:"true" mapstructure:",squash"`
	Instance       `mapstructure:",squash"`
	Reconciliation `mapstructure:",squash"`

	// The namespace to deploy the control plane in
	Namespace *string `json:",omitempty" validate:"required" gorm:"not null"`

	// When true, indicates the control plane instance represents the control plane in which it's stored
	IsSelf *bool `json:",omitempty" validate:"optional" gorm:"default:false"`

	// Passed in information for the different components of the control plane i.e. controller etc
	// When not provided, the default values will be used. If provided, they will override the default values.
	// Despite being a reference to another database entry, we dont validate association.
	// This allows a user to provide CustomComponentInfo at instance creation time so the reconciler has the info it needs
	CustomComponentInfo []*ControlPlaneComponent `json:",omitempty" validate:"optional"`

	// Indicates whether this is was the first control plane that was spun up in a control plane group
	Genesis *bool `json:",omitempty" validate:"optional" gorm:"default:false"`

	// Information for connecting to the rest api for the control plane
	ApiServerEndpoint *string `json:",omitempty" validate:"optional"`

	// The CA Cert that is associated with the control plane
	CACert *string `json:",omitempty" validate:"optional"`

	// The client cert that is associated with the control plane
	ClientCert *string `json:",omitempty" validate:"optional"`

	// The client Key that is associated with the control plane
	ClientKey *string `json:",omitempty" validate:"optional"`

	// the kubernetes runtime instance the control plane is running on
	KubernetesRuntimeInstanceID *uint `json:",omitempty" validate:"required" gorm:"not null" relationship:"requires"`

	// These are pointers to the parent and children of the current control plane
	// This is useful to map out the topology between control planes being managed by one another
	ParentControlPlaneInstanceID *uint                   `json:",omitempty" validate:"optional" relationship:"requires;type:ControlPlaneInstance"`
	Parent                       *ControlPlaneInstance   `json:",omitempty" validate:"optional,association" gorm:"foreignKey:ParentControlPlaneInstanceID"`
	Children                     *[]ControlPlaneInstance `json:",omitempty" validate:"optional,association" gorm:"foreignKey:ParentControlPlaneInstanceID"`

	// The definition used to configure the control plane instance.
	ControlPlaneDefinitionID *uint `json:",omitempty" validate:"required" gorm:"not null" relationship:"requires"`
}
