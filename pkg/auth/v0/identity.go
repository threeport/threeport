package v0

// OUControlPlane identifies threeport control-plane components in client
// certs.
const OUControlPlane = "controlplane"

// OUDatabase identifies certs issued for the threeport database.
const OUDatabase = "database"

// CNKubernetesRuntimeController is the common name of the kubernetes runtime
// controller's client certificate.  It is the only caller the API issues agent
// certificates to.
const CNKubernetesRuntimeController = "kubernetes-runtime-controller"

// AgentCommonNamePrefix starts the common name of every certificate the API
// issues to an agent on a managed cluster.
const AgentCommonNamePrefix = "threeport-agent"
