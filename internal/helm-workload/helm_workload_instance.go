package helmworkload

import (
	"fmt"
	"os"

	"github.com/go-logr/logr"
	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/cli"
	"helm.sh/helm/v3/pkg/registry"
	"helm.sh/helm/v3/pkg/repo"

	v0 "github.com/threeport/threeport/pkg/api/v0"
	client "github.com/threeport/threeport/pkg/client/v0"
	controller "github.com/threeport/threeport/pkg/controller/v0"
)

// helmWorkloadInstanceCreated reconciles state for a new helm workload
// instance.
func helmWorkloadInstanceCreated(
	r *controller.Reconciler,
	helmWorkloadInstance *v0.HelmWorkloadInstance,
	log *logr.Logger,
) (int64, error) {
	// get helm action config and env settings
	actionConf, settings, err := getHelmActionConfig(r, helmWorkloadInstance)
	if err != nil {
		return 0, fmt.Errorf("failed to get a helm action config: %w", err)
	}

	// add the OCI repo
	repoFile := settings.RepositoryConfig
	repoFileEntries, err := repo.LoadFile(repoFile)
	if err != nil || repoFileEntries == nil {
		return 0, fmt.Errorf("failed to load repo files: %w", err)
	}
	newEntry := &repo.Entry{
		Name: "bitnami",
		URL:  "oci://registry-1.docker.io/bitnamicharts",
	}
	repoFileEntries.Add(newEntry)
	if err := repoFileEntries.WriteFile(repoFile, 0644); err != nil {
		return 0, fmt.Errorf("failed to write repo files: %w", err)
	}

	// install the chart
	install := action.NewInstall(actionConf)
	install.ReleaseName = "wordpress-release"
	install.Namespace = "default"
	chartPath, err := install.LocateChart("oci://registry-1.docker.io/bitnamicharts/wordpress", settings)
	if err != nil {
		return 0, fmt.Errorf("failed to set helm chart path: %w", err)
	}

	// load the chart
	chart, err := loader.Load(chartPath)
	if err != nil {
		return 0, fmt.Errorf("failed to load helm chart: %w", err)
	}

	// define custom values
	customValues := map[string]interface{}{
		// Add more custom values as needed
	}

	// deploy the helm workload
	_, err = install.Run(chart, customValues)
	if err != nil {
		return 0, fmt.Errorf("failed to install helm chart: %w", err)
	}

	return 0, nil
}

// helmWorkloadInstanceCreated reconciles state for a helm workload
// instance when it is changed.
func helmWorkloadInstanceUpdated(
	r *controller.Reconciler,
	helmWorkloadInstance *v0.HelmWorkloadInstance,
	log *logr.Logger,
) (int64, error) {
	return 0, nil
}

// helmWorkloadInstanceCreated reconciles state for a helm workload
// instance when it is removed.
func helmWorkloadInstanceDeleted(
	r *controller.Reconciler,
	helmWorkloadInstance *v0.HelmWorkloadInstance,
	log *logr.Logger,
) (int64, error) {
	// get helm action config
	actionConf, _, err := getHelmActionConfig(r, helmWorkloadInstance)
	if err != nil {
		return 0, fmt.Errorf("failed to get a helm action config: %w", err)
	}

	// Setting up uninstall action
	uninstall := action.NewUninstall(actionConf)

	// Running uninstall action
	releaseName := "wordpress-release"
	_, err = uninstall.Run(releaseName)
	if err != nil {
		return 0, fmt.Errorf("failed to uninstall helm chart: %w", err)
	}

	return 0, nil
}

// getHelmActionConfig returns a helm action config and cli env settings to use
// for managing a workload with helm.
func getHelmActionConfig(
	r *controller.Reconciler,
	helmWorkloadInstance *v0.HelmWorkloadInstance,
) (*action.Configuration, *cli.EnvSettings, error) {
	// get kubernetes runtime instance
	kubernetesRuntimeInstance, err := client.GetKubernetesRuntimeInstanceByID(
		r.APIClient,
		r.APIServer,
		*helmWorkloadInstance.KubernetesRuntimeInstanceID,
	)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get workload kubernetesRuntime instance by ID: %w", err)
	}

	// create env settings and set repo config
	settings := cli.New()
	settings.RepositoryConfig = "/root/repository.yaml"

	// ensure helm repo config exists
	if _, err := os.Stat(settings.RepositoryConfig); os.IsNotExist(err) {
		_, err := os.Create(settings.RepositoryConfig)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to initialize helm repo config: %w", err)
		}
	}

	// create a new custom REST client getter
	customGetter := &CustomRESTClientGetter{
		kubernetesRuntimeInstance,
		r.APIClient,
		r.APIServer,
		r.EncryptionKey,
	}

	// create OCI registry client
	client, err := registry.NewClient()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create helm registry client: %w", err)
	}

	// create helm action config
	actionConfig := new(action.Configuration)
	if err := actionConfig.Init(
		customGetter,
		settings.Namespace(), // TODO: set namespace properly
		os.Getenv("HELM_DRIVER"),
		func(format string, v ...interface{}) {
			fmt.Sprintf(format, v)
		}); err != nil {
		return nil, nil, fmt.Errorf("failed to initialize action config: %w", err)
	}

	// set the registry client in the action config
	actionConfig.RegistryClient = client

	return actionConfig, settings, nil
}
