package cmd

import (
	"os"

	"github.com/threeport/threeport/internal/cli"
	"github.com/threeport/threeport/internal/kube"
	config "github.com/threeport/threeport/pkg/config/v0"
)

func InitArgs(args *config.ControlPlaneCLIArgs) {
	if args.ProviderConfigDir == "" {
		providerConf, err := config.DefaultProviderConfigDir()
		if err != nil {
			cli.Error("failed to set infra provider config directory", err)
			os.Exit(1)
		}
		args.ProviderConfigDir = providerConf
	}

	dk, err := kube.DefaultKubeconfig()
	if err != nil {
		cli.Error("failed to get path to default kubeconfig", err)
		os.Exit(1)
	}
	args.KubeconfigPath = dk
}
