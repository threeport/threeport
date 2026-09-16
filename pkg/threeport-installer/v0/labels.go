package v0

const (
	// The kubernetes recommended-label manager of installer-created objects
	LabelManagedBy      = "app.kubernetes.io/managed-by"
	LabelManagedByValue = "threeport-installer"

	// The label that keeps a namespaced object across a reinstall
	LabelPersistent      = "threeport.io/persistent"
	LabelPersistentValue = "true"

	// The control plane availability and data retention level
	LabelTier = "threeport.io/tier"
)
