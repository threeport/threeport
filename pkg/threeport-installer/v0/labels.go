package v0

// CreateOrUpdateKubeResource stamps managed-by on each object it writes.
// Reinstall deletes those objects unless persistent is set. The control
// plane namespace stores its tier on a separate label.

const (
	// Kubernetes recommended-label key for the manager of an installer-written object
	LabelManagedBy = "app.kubernetes.io/managed-by"
	// Manager value written on those objects and matched by selectors
	LabelManagedByValue = "threeport-installer"
	// Label key for an object reinstall neither deletes nor replaces
	LabelPersistent = "threeport.io/persistent"
	// Value of LabelPersistent that marks that object
	LabelPersistentValue = "true"
	// Label key for the control plane availability and data retention level
	LabelTier = "threeport.io/tier"
)
