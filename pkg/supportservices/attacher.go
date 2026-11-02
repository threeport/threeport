package supportservices

// SupportServiceAttacher is the object that uses one shared workload.
type SupportServiceAttacher interface {

	// IsComponentReconciled reports whether that kubernetes workload instance has reconciled.
	// A missing row returns reconciled false and missing true.
	IsComponentReconciled(id uint) (reconciled bool, missing bool, err error)

	// SetCallerKubernetesWorkloadInstanceID stores the component kubernetes workload instance id on the caller.
	SetCallerKubernetesWorkloadInstanceID(id *uint) error

	// ClaimRuntimeKubernetesWorkloadInstanceID writes the runtime component column when the compare accepts it.
	ClaimRuntimeKubernetesWorkloadInstanceID(runtimeID uint, workloadID uint) error

	// ClearRuntimeKubernetesWorkloadInstanceID nulls the runtime component column when this id is the stored one.
	// The cleared result is true when this caller nulled the column.
	ClearRuntimeKubernetesWorkloadInstanceID(runtimeID uint, workloadID uint) (cleared bool, err error)

	// DeleteComponentKubernetesWorkloadInstance removes that component kubernetes workload instance.
	DeleteComponentKubernetesWorkloadInstance(id uint) error
}

// ManifestEditor returns the component document, including any object the caller adds.
// A caller that leaves the document unchanged does not implement it.
type ManifestEditor interface {
	EditManifest(base func() (string, error)) (string, error)
}
