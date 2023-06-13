package cli

// ControlPlaneCLIArgs is the set of control plane arguments passed to the CLI.
type ControlPlaneCLIArgs struct {
	InstanceName            string
	CreateRootDomain        string
	CreateProviderAccountID string
	CreateAdminEmail        string
	ForceOverwriteConfig    bool
	AuthEnabled             bool
	InfraProvider           string
	ControlPlaneImageRepo   string
	ControlPlaneImageTag    string
	ThreeportLocalAPIPort   int
	NumWorkerNodes          int
	AwsConfigProfile        string
	AwsConfigEnv            bool
	AwsRegion               string
	KubeconfigPath          string
	ThreeportPath           string
	CfgFile                 string
	ProviderConfigDir       string
	DevEnvironment          bool
}
