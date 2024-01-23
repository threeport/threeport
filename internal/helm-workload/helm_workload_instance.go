package helmworkload

import (
	"context"
	"errors"
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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"

	v0 "github.com/threeport/threeport/pkg/api/v0"
	client "github.com/threeport/threeport/pkg/client/v0"
	controller "github.com/threeport/threeport/pkg/controller/v0"
	kube "github.com/threeport/threeport/pkg/kube/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

const (
	HelmRepoConfigFile = "/root/repository.yaml"
	HelmValuesDir      = "/tmp/helm"
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

	// get helm action config, env settings and kube client
	actionConf, settings, kubeClient, err := getHelmActionConfig(r, helmWorkloadInstance)
	if err != nil {
		return 0, fmt.Errorf("failed to get a helm action config: %w", err)
	}

	// add the helm repo
	repoFile := settings.RepositoryConfig
	repoFileEntries, err := repo.LoadFile(repoFile)
	if err != nil || repoFileEntries == nil {
		return 0, fmt.Errorf("failed to load repo files: %w", err)
	}
	repoName := fmt.Sprintf("%d-repo", *helmWorkloadInstance.ID)
	newEntry := &repo.Entry{
		Name: repoName,
		URL:  *helmWorkloadDefinition.Repo,
	}
	repoFileEntries.Add(newEntry)
	if err := repoFileEntries.WriteFile(repoFile, 0644); err != nil {
		return 0, fmt.Errorf("failed to write repo files: %w", err)
	}

	// download the index file for https-based helm repositories
	if !registry.IsOCI(*helmWorkloadDefinition.Repo) {
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
	if helmWorkloadDefinition.ChartVersion != nil && *helmWorkloadDefinition.ChartVersion != "" {
		install.Version = *helmWorkloadDefinition.ChartVersion
	}

	install.ReleaseName = helmReleaseName(helmWorkloadInstance)
	install.Namespace = fmt.Sprintf("%s-%s", *helmWorkloadInstance.Name, util.RandomAlphaString(10))
	install.CreateNamespace = true
	install.DependencyUpdate = true
	install.PostRenderer = &ThreeportPostRenderer{
		HelmWorkloadDefinition: helmWorkloadDefinition,
		HelmWorkloadInstance:   helmWorkloadInstance,
	}
	var chartPath string
	if registry.IsOCI(*helmWorkloadDefinition.Repo) {
		ociChart := fmt.Sprintf("%s/%s", *helmWorkloadDefinition.Repo, *helmWorkloadDefinition.Chart)
		chart, err := install.LocateChart(ociChart, settings)
		if err != nil {
			return 0, fmt.Errorf("failed to set oci helm chart path: %w", err)
		}
		chartPath = chart
	} else {
		httpsChart := fmt.Sprintf("%s/%s", repoName, *helmWorkloadDefinition.Chart)
		chart, err := install.LocateChart(httpsChart, settings)
		if err != nil {
			return 0, fmt.Errorf("failed to set https helm chart path: %w", err)
		}
		chartPath = chart
	}

	// load the chart
	chart, err := loader.Load(chartPath)
	if err != nil {
		return 0, fmt.Errorf("failed to load helm chart: %w", err)
	}

	// write the value files to merge as needed
	var valueFiles []string
	if helmWorkloadDefinition.ValuesDocument != nil {
		filePath := fmt.Sprintf("%s/%d-definition-vals.yaml", HelmValuesDir, (*helmWorkloadInstance.ID))
		if err := os.WriteFile(
			filePath,
			[]byte(*helmWorkloadDefinition.ValuesDocument),
			0644,
		); err != nil {
			return 0, fmt.Errorf("failed to write values file for helm workload definition values: %w", err)
		}
		valueFiles = append(valueFiles, filePath)
	}
	if helmWorkloadInstance.ValuesDocument != nil {
		filePath := fmt.Sprintf("%s/%d-instance-vals.yaml", HelmValuesDir, (*helmWorkloadInstance.ID))
		if err := os.WriteFile(
			filePath,
			[]byte(*helmWorkloadInstance.ValuesDocument),
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
	release, err := install.Run(chart, helmValues)
	if err != nil {
		if uninstallErr := uninstallHelmRelease(
			release.Name,
			actionConf,
			kubeClient,
		); err != nil {
			return 0, fmt.Errorf("failed to uninstall helm release: %w after failed to install helm chart: %w", uninstallErr, err)
		}
		return 0, fmt.Errorf("failed to install helm chart: %w", err)
	}

	// clean up files written to disk
	if err := cleanLocalFiles(); err != nil {
		// logging err but not returning it as it is non-critical and we do not
		// want to re-queue reconciliation
		log.Error(err, "failed to remove files written to disk")
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
	// get helm action config and kube client
	actionConf, _, kubeClient, err := getHelmActionConfig(r, helmWorkloadInstance)
	if err != nil {
		return 0, fmt.Errorf("failed to get a helm action config: %w", err)
	}

	// uninstall helm release
	if err := uninstallHelmRelease(
		helmReleaseName(helmWorkloadInstance),
		actionConf,
		kubeClient,
	); err != nil {
		return 0, fmt.Errorf("failed to uninstall helm release: %w", err)
	}

	// clean up files written to disk
	if err := cleanLocalFiles(); err != nil {
		// logging err but not returning it as it is non-critical and we do not
		// want to re-queue reconciliation
		log.Error(err, "failed to remove files written to disk")
	}

	return 0, nil
}

// uninstallHelmRelease uninstalls a named helm release.
func uninstallHelmRelease(
	releaseName string,
	actionConf *action.Configuration,
	kubeClient dynamic.Interface,
) error {
	// set up uninstall action
	uninstall := action.NewUninstall(actionConf)

	// ignore error if release not found
	uninstall.IgnoreNotFound = true

	// run uninstall action
	release, err := uninstall.Run(releaseName)
	if err != nil {
		return fmt.Errorf("failed to uninstall helm chart: %w", err)
	}

	// remove namespace
	gvr := schema.GroupVersionResource{Group: "", Version: "v1", Resource: "namespaces"}
	namespaceResource := kubeClient.Resource(gvr)
	namespace := release.Release.Namespace
	deletePolicy := metav1.DeletePropagationForeground
	deleteOptions := metav1.DeleteOptions{
		PropagationPolicy: &deletePolicy,
	}
	if err = namespaceResource.Delete(context.TODO(), namespace, deleteOptions); err != nil {
		return fmt.Errorf("failed to delete helm release namespace: %w", err)
	}

	return nil
}

// getHelmActionConfig returns a helm action config and cli env settings to use
// for managing a workload with helm.
func getHelmActionConfig(
	r *controller.Reconciler,
	helmWorkloadInstance *v0.HelmWorkloadInstance,
) (*action.Configuration, *cli.EnvSettings, dynamic.Interface, error) {
	// get kubernetes runtime instance
	kubernetesRuntimeInstance, err := client.GetKubernetesRuntimeInstanceByID(
		r.APIClient,
		r.APIServer,
		*helmWorkloadInstance.KubernetesRuntimeInstanceID,
	)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to get workload kubernetesRuntime instance by ID: %w", err)
	}

	// create env settings and set repo config
	settings := cli.New()
	settings.RepositoryConfig = HelmRepoConfigFile

	// ensure helm repo config exists
	if _, err := os.Stat(settings.RepositoryConfig); os.IsNotExist(err) {
		_, err := os.Create(settings.RepositoryConfig)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("failed to initialize helm repo config: %w", err)
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
		return nil, nil, nil, fmt.Errorf("failed to create helm registry client: %w", err)
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
		return nil, nil, nil, fmt.Errorf("failed to initialize action config: %w", err)
	}

	// set the registry client in the action config
	actionConfig.RegistryClient = client

	// create a directory for helm values files
	if _, err := os.Stat(HelmValuesDir); errors.Is(err, os.ErrNotExist) {
		if err := os.Mkdir(HelmValuesDir, os.ModePerm); err != nil {
			return nil, nil, nil, fmt.Errorf("failed to create helm values directory: %w", err)
		}
	}

	// get a dynamic kubernetes client
	kubeClient, _, err := kube.GetClient(
		kubernetesRuntimeInstance,
		true,
		r.APIClient,
		r.APIServer,
		r.EncryptionKey,
	)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to get dynamic kubernetes client: %w", err)
	}

	return actionConfig, settings, kubeClient, nil
}

// helmReleaseName returns a standardized helm release name based on a helm
// workload instance name.
func helmReleaseName(helmWorkloadInstance *v0.HelmWorkloadInstance) string {
	return fmt.Sprintf("%s-release", *helmWorkloadInstance.Name)
}

// cleanLocalFiles removes all local files written by the helm workload instance
// reconciler and helm itself so as to not incrementally increase disk usage
// over time.
func cleanLocalFiles() error {
	// remove helm repo config file
	if err := os.Remove(HelmRepoConfigFile); err != nil {
		return fmt.Errorf("failed to remove helm repo config file: %w", err)
	}

	// remove values files
	if err := os.RemoveAll(HelmValuesDir); err != nil {
		return fmt.Errorf("failed to remove helm values files: %w", err)
	}

	// remove helm cache files
	if err := os.RemoveAll("/root/.cache/helm"); err != nil {
		return fmt.Errorf("failed to remove helm cache files: %w", err)
	}

	return nil
}
