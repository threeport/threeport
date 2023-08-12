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
