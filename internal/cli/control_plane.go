package cli

// ControlPlaneCLIArgs is the set of control plane arguments passed to the CLI.
type ControlPlaneCLIArgs struct {
	AuthEnabled             bool
	AwsConfigProfile        string
	AwsConfigEnv            bool
	AwsRegion               string
	CfgFile                 string
	ControlPlaneImageRepo   string
	ControlPlaneImageTag    string
	CreateRootDomain        string
	CreateProviderAccountID string
	CreateAdminEmail        string
	DevEnvironment          bool
	ForceOverwriteConfig    bool
	InstanceName            string
	InfraProvider           string
	KubeconfigPath          string
	NumWorkerNodes          int
	ProviderConfigDir       string
	ThreeportLocalAPIPort   int
	ThreeportPath           string
}
