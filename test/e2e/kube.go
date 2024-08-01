package e2e_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
)

func getKubeClient() (*kubernetes.Clientset, error) {
	// get kubeconfig file path
	kubeconfigPath, ok := os.LookupEnv("KUBECONFIG")
	if !ok {
		home := homedir.HomeDir()
		if home == "" {
			return nil, errors.New("home directory not found")
		}
		kubeconfigPath = filepath.Join(home, ".kube", "config")
	}

	// load kubeconfig
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load kubeconfig: %w", err)
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

	labelSelector := fmt.Sprintf("control-plane.threeport.io/workload-instance=%d", workloadInstId)

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
