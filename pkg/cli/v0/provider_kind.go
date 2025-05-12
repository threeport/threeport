package v0

import (
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"

	"github.com/threeport/threeport/internal/provider"
	config "github.com/threeport/threeport/pkg/config/v0"
	kube "github.com/threeport/threeport/pkg/kube/v0"
	threeport "github.com/threeport/threeport/pkg/threeport-installer/v0"
	"github.com/threeport/threeport/pkg/threeport-installer/v0/tptdev"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// DeployKindInfra deploys kind infrastructure for the control plane.
func DeployKindInfra(
	cpi *threeport.ControlPlaneInstaller,
	threeportAPIEndpoint *string,
	threeportControlPlaneConfig *config.ControlPlane,
	threeportConfig *config.ThreeportConfig,
	kubernetesRuntimeInfra *provider.KubernetesRuntimeInfra,
	kubeConnectionInfo *kube.KubeConnectionInfo,
	uninstaller *Uninstaller,
) error {

	portForwards := make(map[int32]int32)
	for _, mapping := range cpi.Opts.KindInfraPortForward {
		split := strings.Split(mapping, ":")
		if len(split) != 2 {
			return fmt.Errorf("failed to parse kind port forward %s", mapping)
		}

		containerPort, err := strconv.ParseInt(split[0], 10, 32)
		if err != nil {
			return fmt.Errorf("failed to parse container port: %s as int32", split[0])
		}

		hostPort, err := strconv.ParseInt(split[1], 10, 32)
		if err != nil {
			return fmt.Errorf("failed to parse host port: %s as int32", split[0])
		}

		portForwards[int32(containerPort)] = int32(hostPort)
	}

	// construct kind infra provider object
	kubernetesRuntimeInfraKind := provider.KubernetesRuntimeInfraKind{
		RuntimeInstanceName: provider.ThreeportRuntimeName(cpi.Opts.ControlPlaneName),
		KubeconfigPath:      cpi.Opts.KubeconfigPath,
		DevEnvironment:      cpi.Opts.DevEnvironment,
		ThreeportPath:       cpi.Opts.ThreeportPath,
		NumWorkerNodes:      cpi.Opts.NumWorkerNodes,
		AuthEnabled:         cpi.Opts.AuthEnabled,
		PortForwards:        portForwards,
	}

	// update threeport config with api endpoint
	var err error
	threeportAPIEndpoint = util.Ptr(threeport.GetLocalThreeportAPIEndpoint(cpi.Opts.AuthEnabled))
	if threeportConfig, err = threeportControlPlaneConfig.UpdateThreeportConfigInstance(func(c *config.ControlPlane) {
		c.APIServer = *threeportAPIEndpoint
	}); err != nil {
		return fmt.Errorf("failed to update threeport config: %w", err)
	}

	// delete kind kubernetes runtime if interrupted
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigs
		Warning("received Ctrl+C, removing kind kubernetes runtime...")
		// first update the threeport config so the Delete method has
		// something to reference
		threeportControlPlaneConfig.UpdateThreeportConfigInstance(func(c *config.ControlPlane) {})
		if err := DeleteGenesisControlPlane(cpi); err != nil {
			Error("failed to delete kind kubernetes runtime", err)
		}
		os.Exit(1)
	}()

	*kubernetesRuntimeInfra = &kubernetesRuntimeInfraKind
	uninstaller.kubernetesRuntimeInfra = *kubernetesRuntimeInfra
	if cpi.Opts.ControlPlaneOnly {
		kubeConnectionInfo, err = kube.GetConnectionInfoFromKubeconfig(kubernetesRuntimeInfraKind.KubeconfigPath)
		if err != nil {
			return fmt.Errorf("failed to get connection info for kind kubernetes runtime: %w", err)
		}
	} else {
		kubeConnectionInfo, err = (*kubernetesRuntimeInfra).Create()
		if err != nil {
			return uninstaller.cleanOnCreateError("failed to create control plane infra for threeport", err)
		}
	}

	// connect local registry if requested
	if cpi.Opts.LocalRegistry {
		if err := tptdev.ConnectLocalRegistry(
			provider.ThreeportRuntimeName(cpi.Opts.ControlPlaneName),
		); err != nil {
			return uninstaller.cleanOnCreateError("failed to connect local container registry to Threeport control plane cluster", err)
		}
	}
	return nil
}
