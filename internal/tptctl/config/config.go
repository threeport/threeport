package config

// ThreeportConfig is the client's configuration for connecting to Threeport instances
type ThreeportConfig struct {
	Instances       []Instance `yaml:"Instances"`
	CurrentInstance string     `yaml:"CurrentInstance"`
}

// ThreeportInstance is an instance of Threeport the client can use
type Instance struct {
	Name      string `yaml:"Name"`
	Provider  string `yaml:"Provider"`
	APIServer string `yaml:"APIServer"`
}
