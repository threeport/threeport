package v0

const (
	// PathAgentCertificates is the path to request a client certificate for the
	// agent on a managed runtime.
	PathAgentCertificates = "/v0/agent-certificates"

	// ObjectTypeAgentCertificate is the type of the response to a request for an
	// agent certificate.  It is not a stored object.
	ObjectTypeAgentCertificate string = "AgentCertificate"
)

// AgentCertificateRequest asks the API to issue a client certificate to the
// agent on a managed runtime.  Only the kubernetes runtime controller may make
// the request.
type AgentCertificateRequest struct {
	// The name of the kubernetes runtime instance the agent runs on.  The
	// certificate's common name is derived from it.
	RuntimeInstanceName string `validate:"required"`

	// A PEM-encoded certificate signing request for the agent's key.  Only the
	// public key in it is used: the API decides the certificate's subject, so
	// whatever the request says about identity is ignored.
	CertificateSigningRequest string `validate:"required"`
}

// AgentCertificate is a client certificate issued to an agent.  The API never
// sees the agent's private key.
type AgentCertificate struct {
	// The PEM-encoded client certificate.
	Certificate string

	// The PEM-encoded certificate of the CA that signed it, for the agent to
	// verify the API with.
	CACertificate string
}

// GetFullyQualifiedType returns the fully qualified type of an agent
// certificate response.
func (ac *AgentCertificate) GetFullyQualifiedType() string {
	return "threeport.io/v0.AgentCertificate"
}
