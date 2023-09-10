package v0

import (
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/client-go/dynamic"
)

type InstallerOption func(o *Options)

type CustomInstallFunction func(dynamic.Interface, *meta.RESTMapper, *ControlPlaneInstaller) error

type Options struct {
	Name                string
	Namespace           string
	PreInstallFunction  CustomInstallFunction
	PostInstallFunction CustomInstallFunction
	ControllerList      []InstallInfo
	RestApiInfo         InstallInfo
	AgentInfo           InstallInfo
}

type InstallInfo struct {
	Name                string
	ImageName           string
	ImageRepo           string
	ImageTag            string
	ServiceAccountName  string
	ServiceResourceName string
}

type ControlPlaneInstaller struct {
	Opts Options
}

func (cpi *ControlPlaneInstaller) SetAllImageRepo(imageRepo string) {
	for _, c := range cpi.Opts.ControllerList {
		c.ImageRepo = imageRepo
	}
}

func (cpi *ControlPlaneInstaller) SetAllImageTags(imageTag string) {
	for _, c := range cpi.Opts.ControllerList {
		c.ImageTag = imageTag
	}
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

func RestApi(r InstallInfo) InstallerOption {
	return func(o *Options) {
		o.RestApiInfo = r
	}
}

func CustomController(c InstallInfo) InstallerOption {
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
	RestApiInfo:         ThreeportRestApi,
	AgentInfo:           ThreeportAgent,
	ControllerList:      ThreeportControllerList,
	PreInstallFunction:  defaultInstallFunction,
	PostInstallFunction: defaultInstallFunction,
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
