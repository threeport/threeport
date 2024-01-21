package helmworkload

import (
	"fmt"
	"os"

	"github.com/go-logr/logr"
	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/cli"
	"helm.sh/helm/v3/pkg/cli/values"
	"helm.sh/helm/v3/pkg/getter"
	"helm.sh/helm/v3/pkg/registry"
	"helm.sh/helm/v3/pkg/repo"

	v0 "github.com/threeport/threeport/pkg/api/v0"
	client "github.com/threeport/threeport/pkg/client/v0"
	controller "github.com/threeport/threeport/pkg/controller/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// helmWorkloadInstanceCreated reconciles state for a new helm workload
// instance.
func helmWorkloadInstanceCreated(
	r *controller.Reconciler,
	helmWorkloadInstance *v0.HelmWorkloadInstance,
	log *logr.Logger,
) (int64, error) {
	// get helm workload definition
	helmWorkloadDefinition, err := client.GetHelmWorkloadDefinitionByID(
		r.APIClient,
		r.APIServer,
		*helmWorkloadInstance.HelmWorkloadDefinitionID,
	)
	if err != nil {
		return 0, fmt.Errorf("failed to get helm workload definition: %w", err)
	}

	// get helm action config and env settings
	actionConf, settings, err := getHelmActionConfig(r, helmWorkloadInstance)
	if err != nil {
		return 0, fmt.Errorf("failed to get a helm action config: %w", err)
	}

	// add the helm repo
	repoFile := settings.RepositoryConfig
	repoFileEntries, err := repo.LoadFile(repoFile)
	if err != nil || repoFileEntries == nil {
		return 0, fmt.Errorf("failed to load repo files: %w", err)
	}
	newEntry := &repo.Entry{
		Name: fmt.Sprintf("%d-repo", *helmWorkloadInstance.ID),
		URL:  *helmWorkloadDefinition.HelmRepo,
	}
	repoFileEntries.Add(newEntry)
	if err := repoFileEntries.WriteFile(repoFile, 0644); err != nil {
		return 0, fmt.Errorf("failed to write repo files: %w", err)
	}

	// download the index file for https-based helm repositories
	if !registry.IsOCI(*helmWorkloadDefinition.HelmRepo) {
		repository, err := repo.NewChartRepository(newEntry, getter.All(settings))
		if err != nil {
			return 0, fmt.Errorf("failed to create chart repository: %w", err)
		}
		_, err = repository.DownloadIndexFile()
		if err != nil {
			return 0, fmt.Errorf("failed to download index file: %w", err)
		}
	}

	// install the chart
	install := action.NewInstall(actionConf)

	// configure version if it is supplied by the workload definition
	if helmWorkloadDefinition.HelmChartVersion != nil && *helmWorkloadDefinition.HelmChartVersion != "" {
		install.Version = *helmWorkloadDefinition.HelmChartVersion
	}

	install.ReleaseName = fmt.Sprintf("%s-release", *helmWorkloadInstance.Name)
	install.Namespace = fmt.Sprintf("%s-%s", *helmWorkloadInstance.Name, util.RandomAlphaString(10))
	install.CreateNamespace = true
	install.PostRenderer = &ThreeportPostRenderer{
		HelmWorkloadDefinition: helmWorkloadDefinition,
		HelmWorkloadInstance:   helmWorkloadInstance,
	}
	install.RepoURL = *helmWorkloadDefinition.HelmRepo
	install.DependencyUpdate = true
	chartPath, err := install.LocateChart(*helmWorkloadDefinition.HelmChart, settings)
	if err != nil {
		return 0, fmt.Errorf("failed to set helm chart path: %w", err)
	}

	// load the chart
	chart, err := loader.Load(chartPath)
	if err != nil {
		return 0, fmt.Errorf("failed to load helm chart: %w", err)
	}

	// write the value files to merge as needed
	var valueFiles []string
	if helmWorkloadDefinition.HelmValuesDocument != nil {
		filePath := fmt.Sprintf("/tmp/%d-definition-vals.yaml", (*helmWorkloadInstance.ID))
		if err := os.WriteFile(
			filePath,
			[]byte(*helmWorkloadDefinition.HelmValuesDocument),
			0644,
		); err != nil {
			return 0, fmt.Errorf("failed to values file for helm workload definition values: %w", err)
		}
		valueFiles = append(valueFiles, filePath)
	}
	if helmWorkloadInstance.HelmValuesDocument != nil {
		filePath := fmt.Sprintf("/tmp/%d-instance-vals.yaml", (*helmWorkloadInstance.ID))
		if err := os.WriteFile(
			filePath,
			[]byte(*helmWorkloadInstance.HelmValuesDocument),
			0644,
		); err != nil {
			return 0, fmt.Errorf("failed to values file for helm workload instance values: %w", err)
		}
		valueFiles = append(valueFiles, filePath)
	}

	// merge the helm values
	var helmValues map[string]interface{}
	if len(valueFiles) > 0 {
		vals := values.Options{ValueFiles: valueFiles}
		helmVals, err := vals.MergeValues(nil)
		if err != nil {
			return 0, fmt.Errorf("failed to merge helm values: %s", err)
		}
		helmValues = helmVals
	}

	// deploy the helm workload
	_, err = install.Run(chart, helmValues)
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
	releaseName := fmt.Sprintf("%s-release", *helmWorkloadInstance.Name)
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

	// create registry client
	client, err := registry.NewClient()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create helm registry client: %w", err)
	}

	// create helm action config
	actionConfig := new(action.Configuration)
	if err := actionConfig.Init(
		customGetter,
		settings.Namespace(),
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
