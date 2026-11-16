package e2e_test

import (
	"context"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	client_v0 "github.com/threeport/threeport/pkg/client/v0"
	kube "github.com/threeport/threeport/pkg/kube/v0"
)

// getKubeClient returns a client for the control plane's cluster, built from
// its kubernetes runtime instance in the Threeport API the same way tptctl
// builds its own.  A kubeconfig would not do: on EKS tptctl writes no context
// for the cluster, and the identity running the suite is not granted access
// to it - only the role Threeport assumes is.
func getKubeClient() (*kubernetes.Clientset, error) {
	suiteConfig, apiClient, apiEndpoint, err := suiteThreeportAPI()
	if err != nil {
		return nil, err
	}

	encryptionKey, err := suiteConfig.GetThreeportEncryptionKey(threeportName)
	if err != nil {
		return nil, fmt.Errorf("failed to get the threeport encryption key: %w", err)
	}

	runtimeInstance, err := client_v0.GetThreeportControlPlaneKubernetesRuntimeInstance(apiClient, apiEndpoint)
	if err != nil {
		return nil, fmt.Errorf("failed to get the control plane's kubernetes runtime instance: %w", err)
	}

	config, err := kube.GetRestConfig(runtimeInstance, false, apiClient, apiEndpoint, encryptionKey)
	if err != nil {
		return nil, fmt.Errorf("failed to get a REST config for the control plane's cluster: %w", err)
	}

	// create K8s clientset
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create new Kubernetes clientset: %w", err)
	}

	return clientset, nil
}

func getNamespaceByWorkloadInstanceId(workloadInstId int64) (string, error) {
	clientset, err := getKubeClient()
	if err != nil {
		return "", fmt.Errorf("failed to get Kubernetes client: %w", err)
	}

	labelSelector := fmt.Sprintf("control-plane.threeport.io/kubernetes-workload-instance=%d", workloadInstId)

	// get namespace by label
	namespaceList, err := clientset.CoreV1().Namespaces().List(context.TODO(), metav1.ListOptions{
		LabelSelector: labelSelector,
	})
	if err != nil {
		return "", fmt.Errorf("failed to list namespaces by label selector: %w", err)
	}

	if len(namespaceList.Items) == 0 {
		return "", fmt.Errorf("no namespace found using label selector %s", labelSelector)
	} else if len(namespaceList.Items) > 1 {
		return "", fmt.Errorf("more than one namespace found using label selector %s", labelSelector)
	}

	return namespaceList.Items[0].Name, nil
}

func getDeploymentByName(deploymentName, namespaceName string) (*appsv1.Deployment, error) {
	clientset, err := getKubeClient()
	if err != nil {
		return nil, fmt.Errorf("failed to get Kubernetes client: %w", err)
	}

	// get deployment by name in the namespace provided
	deployment, err := clientset.AppsV1().Deployments(namespaceName).Get(context.TODO(), deploymentName, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf(
			"failed to find deployment %s in namespace %s: %w",
			deploymentName,
			namespaceName,
			err,
		)
	}

	return deployment, nil
}
