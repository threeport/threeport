//go:generate ../../../bin/threeport-codegen api-model --filename $GOFILE --package $GOPACKAGE
//go:generate ../../../bin/threeport-codegen controller --filename $GOFILE
package v0

// +threeport-codegen:reconciler
// ControlPlaneDefinition is the configuration for a Control Plane.
type ControlPlaneDefinition struct {
	Common         `swaggerignore:"true" mapstructure:",squash"`
	Definition     `mapstructure:",squash"`
	Reconciliation `mapstructure:",squash"`

	// Used to indicate whether the control plane is deployed with auth settings
	AuthEnabled *bool `json:"AuthEnabled,omitempty" query:"authenabled" gorm:"default:true" validate:"optional"`

	// When instances of this control plane are deployed, Object representing control plane and its parent are
	// onboarded as part of deployment, using this we can disable that process and simply spin a new instance with
	// a clean DB.
	OnboardParent *bool `json:"OnboardParent,omitempty" query:"onboardparent" gorm:"default:true" validate:"optional"`

	// The associated workload instances that are deployed from this definition.
	ControlPlaneInstances []*ControlPlaneInstance `json:"ControlPlaneInstances,omitempty" validate:"optional,association"`
}

// +threeport-codegen:reconciler
// ControlPlaneInstance is the instance for a deployed Control Plane.
type ControlPlaneInstance struct {
	Common         `swaggerignore:"true" mapstructure:",squash"`
	Instance       `mapstructure:",squash"`
	Reconciliation `mapstructure:",squash"`

	// The namespace to deploy the control plane in
	Namespace *string `json:"Namespace,omitempty" query:"namespace" gorm:"not null" validate:"required"`

	// Used to indicate whether the control plane instance that the reconciler is deployed on
	IsSelf *bool `json:"IsSelf,omitempty" query:"isself" gorm:"default:false" validate:"optional"`

	// Passed in information for the different components of the control plane i.e. controller etc
	// When not provided, the default field will be used. If provided will override the provided field.
	// Despite being a reference to another database entry, we dont validate association.
	// This is allows a user to provide CustomComponentInfo at instance creation time so the reconciler has the info it needs
	CustomComponentInfo []*ControlPlaneComponent `json:"CustomComponentInfo,omitempty" query:"customcomponentinfo" validate:"optional"`

	// Indicates whether this is was the first control plane that was spun up with a control plane group
	Genesis *bool `json:"Genesis,omitempty" query:"genesis" gorm:"default:false" validate:"optional"`

	// Information for connecting to the rest api for the control plane
	ApiServerEndpoint *string `json:"ApiServerEndpoint,omitempty" query:"apiserverendpoint" validate:"optional"`

	// The CA Cert that is associated with the control plane
	CACert *string `json:"CACert,omitempty" query:"cacert" validate:"optional"`

	// The client cert that is associated with the control plane
	ClientCert *string `json:"ClientCert,omitempty" query:"clientcert" validate:"optional"`

	// The client Key that is associated with the control plane
	ClientKey *string `json:"ClientKey,omitempty" query:"clientkey" validate:"optional"`

	// Kubernetes runtime the control planei is running on
	KubernetesRuntimeInstanceID *uint `json:"KubernetesRuntimeInstanceID,omitempty" query:"kubernetesruntimeinstanceid" gorm:"not null" validate:"required"`

	// These are pointers to the parent and children of current control plane
	// This is useful to map out the topology between control planes being managed by one another
	ParentControlPlaneInstanceID *uint                   `json:"ParentControlPlaneInstanceID,omitempty" validate:"optional"`
	Parent                       *ControlPlaneInstance   `json:"Parent,omitempty" gorm:"foreignKey:ParentControlPlaneInstanceID" validate:"optional,association"`
	Children                     *[]ControlPlaneInstance `json:"Children,omitempty" gorm:"foreignKey:ParentControlPlaneInstanceID" validate:"optional,association"`

	// The definition used to configure the control plane instance.
	ControlPlaneDefinitionID *uint `json:"ControlPlaneDefinitionID,omitempty" query:"controlplanedefinitionid" gorm:"not null" validate:"required"`
}
