package v0

import (
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/client-go/dynamic"

	v0 "github.com/threeport/threeport/pkg/api/v0"
)

type InstallerOption func(o *Options)

type CustomInstallFunction func(dynamic.Interface, *meta.RESTMapper, *ControlPlaneInstaller) error

type Options struct {
	Name                        string
	Namespace                   string
	PreInstallFunction          CustomInstallFunction
	PostInstallFunction         CustomInstallFunction
	ControllerList              []*v0.ControlPlaneComponent
	RestApiInfo                 *v0.ControlPlaneComponent
	AgentInfo                   *v0.ControlPlaneComponent
	InThreeport                 bool
	CreateOrUpdateKubeResources bool
	AuthEnabled                 bool
	AwsConfigProfile            string
	AwsConfigEnv                bool
	AwsRegion                   string
	CfgFile                     string
	CreateRootDomain            string
	CreateAdminEmail            string
	DevEnvironment              bool
	EncryptionKey               string
	ForceOverwriteConfig        bool
	ControlPlaneName            string
	InfraProvider               string
	KubeconfigPath              string
	NumWorkerNodes              int
	ProviderConfigDir           string
	ThreeportPath               string
}

type ControlPlaneInstaller struct {
	Opts Options
}

func (cpi *ControlPlaneInstaller) SetAllImageRepo(imageRepo string) {
	for _, c := range cpi.Opts.ControllerList {
		c.ImageRepo = imageRepo
	}
	cpi.Opts.RestApiInfo.ImageRepo = imageRepo
	cpi.Opts.AgentInfo.ImageRepo = imageRepo
}

func (cpi *ControlPlaneInstaller) SetAllImageTags(imageTag string) {
	for _, c := range cpi.Opts.ControllerList {
		c.ImageTag = imageTag
	}
	cpi.Opts.RestApiInfo.ImageTag = imageTag
	cpi.Opts.AgentInfo.ImageTag = imageTag
}

func Name(n string) InstallerOption {
	return func(o *Options) {
		o.Name = n
	}
}

func Namespace(n string) InstallerOption {
	return func(o *Options) {
		o.Namespace = n
	}
}

func RestApi(r *v0.ControlPlaneComponent) InstallerOption {
	return func(o *Options) {
		o.RestApiInfo = r
	}
}

func CustomController(c *v0.ControlPlaneComponent) InstallerOption {
	return func(o *Options) {
		o.ControllerList = append(o.ControllerList, c)
	}
}

func PreInstallFunction(f CustomInstallFunction) InstallerOption {
	return func(o *Options) {
		o.PreInstallFunction = f
	}
}

func PostInstallFunction(f CustomInstallFunction) InstallerOption {
	return func(o *Options) {
		o.PostInstallFunction = f
	}
}

func defaultInstallFunction(c dynamic.Interface, m *meta.RESTMapper, cpi *ControlPlaneInstaller) error {
	return nil
}

var defaultInstallerOptions = Options{
	Name:                ControlPlaneName,
	Namespace:           ControlPlaneNamespace,
	ControllerList:      ThreeportControllerList,
	RestApiInfo:         ThreeportRestApi,
	AgentInfo:           ThreeportAgent,
	PreInstallFunction:  defaultInstallFunction,
	PostInstallFunction: defaultInstallFunction,
	InThreeport:         false,
}

func NewInstaller(os ...InstallerOption) *ControlPlaneInstaller {
	opts := &defaultInstallerOptions
	for _, o := range os {
		o(opts)
	}

	return &ControlPlaneInstaller{
		Opts: *opts,
	}
}
