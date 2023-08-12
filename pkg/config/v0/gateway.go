package v0

// GatewayDefinitionConfig contains the config for a gateway definition.
type GatewayDefinitionConfig struct {
	GatewayDefinition GatewayDefinitionValues `yaml:"GatewayDefinition"`
}

// GatewayDefinitionValues contains the attributes needed to manage a gateway.
type GatewayDefinitionValues struct {
	TCPPort     int    `yaml:"TCPPort"`
	TLSEnabled  bool   `yaml:"TlSEnabled"`
	ServiceName string `yaml:"ServiceName"`
}

// GatewayInstanceConfig contains the config for a gateway instance.
type GatewayInstanceConfig struct {
	GatewayInstance GatewayInstanceValues `yaml:"GatewayInstance"`
}

// GatewayInstanceValues contains the attributes needed to manage a gateway
// instance.
type GatewayInstanceValues struct {
	Name                      string                          `yaml:"Name"`
	GatewayDefinition         GatewayDefinitionValues         `yaml:"GatewayDefinition"`
	KubernetesRuntimeInstance KubernetesRuntimeInstanceValues `yaml:"KubernetesRuntimeInstance"`
	WorkloadInstance          WorkloadInstanceValues          `yaml:"WorkloadInstance"`
}
