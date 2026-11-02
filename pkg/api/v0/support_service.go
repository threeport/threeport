package v0

// SupportServiceIngress is the ingress component a caller records.
type SupportServiceIngress struct {
	// The workload instance that runs the ingress component for this instance.
	IngressComponentInstanceID *uint `validate:"optional" relationship:"requires;type:KubernetesWorkloadInstance"`
}

// SupportServiceCertificates is the certificates component a caller records.
type SupportServiceCertificates struct {
	// The workload instance that runs the certificates component for this instance.
	CertificatesComponentInstanceID *uint `validate:"optional" relationship:"requires;type:KubernetesWorkloadInstance"`
}

// SupportServiceSecrets is the secrets component a caller records.
type SupportServiceSecrets struct {
	// The workload instance that runs the secrets component for this instance.
	SecretsComponentInstanceID *uint `validate:"optional" relationship:"requires;type:KubernetesWorkloadInstance"`
}

// SupportServiceDatabase is the database component a caller records.
type SupportServiceDatabase struct {
	// The workload instance that provides a database for this instance.
	DatabaseComponentInstanceID *uint `validate:"optional" relationship:"requires;type:KubernetesWorkloadInstance"`
}
