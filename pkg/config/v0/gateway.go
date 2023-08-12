package v0

// DomainNameDefinitionConfig contains the config for a domain name definition.
type DomainNameDefinitionConfig struct {
	DomainNameDefinition DomainNameDefinitionValues `yaml:"DomainNameDefinition"`
}

// DomainNameDefinitionValues contains the attributes needed to manage a domain
// name definition.
type DomainNameDefinitionValues struct {
	Name string `yaml:"name"`
	Path string `yaml:"path"`
}

// DomainNameInstanceConfig contains the config for a domain name instance.
type DomainNameInstanceConfig struct {
	DomainNameInstance DomainNameInstanceValues `yaml:"DomainNameInstance"`
}

// DomainNameInstanceValues contains the attributes needed to manage a domain
// name instance.
type DomainNameInstanceValues struct {
	DomainNameDefinition      DomainNameDefinitionValues      `yaml:"DomainNameDefinition"`
	KubernetesRuntimeInstance KubernetesRuntimeInstanceValues `yaml:"KubernetesRuntimeInstance"`
	WorkloadInstance          WorkloadInstanceValues          `yaml:"WorkloadInstance"`
}

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
