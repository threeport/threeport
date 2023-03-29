package kube

import (
	"errors"
	"fmt"
	"io/ioutil"

	"k8s.io/client-go/tools/clientcmd"
)

// KubeConnectionInfo contains the necessary info to connect to a Kubernetes
// API.
type KubeConnectionInfo struct {
	APIEndpoint   string
	CACertificate string
	Certificate   string
	Key           string
}

// GetConnectionInfoFromKubeconfig extracts the Kubernetes API connection info
// from a kubeconfig.
func GetConnectionInfoFromKubeconfig(kubeconfig string) (*KubeConnectionInfo, error) {
	//// get kubeconfig
	//defaultLoadRules := clientcmd.NewDefaultClientConfigLoadingRules()

	//clientConfigLoadRules, err := defaultLoadRules.Load()
	//if err != nil {
	//	return fmt.Errorf("failed to load default kubeconfig rules: %w", err)
	//}

	//clientConfig := clientcmd.NewDefaultClientConfig(*clientConfigLoadRules, &clientcmd.ConfigOverrides{})
	//kubeConfig, err := clientConfig.RawConfig()
	//if err != nil {
	//	return fmt.Errorf("failed to load kubeconfig: %w", err)
	//}

	var kubeConnInfo KubeConnectionInfo

	// read kubeconfig
	kubeconfigContent, err := ioutil.ReadFile(kubeconfig)
	if err != nil {
		return &kubeConnInfo, fmt.Errorf("failed to read kubeconfig file: %w", err)
	}

	// get kube client config
	clientConfig, err := clientcmd.NewClientConfigFromBytes(kubeconfigContent)
	if err != nil {
		return &kubeConnInfo, fmt.Errorf("failed to get client config from kubeconfig file: %w", err)
	}
	kubeConfig, err := clientConfig.RawConfig()

	// get cluster CA and server endpoint
	//var caCert string
	clusterFound := false
	for clusterName, cluster := range kubeConfig.Clusters {
		if clusterName == kubeConfig.CurrentContext {
			//caCert = string(cluster.CertificateAuthorityData)
			kubeConnInfo.CACertificate = string(cluster.CertificateAuthorityData)
			kubeConnInfo.APIEndpoint = string(cluster.Server)
			clusterFound = true
		}
	}
	if !clusterFound {
		return &kubeConnInfo, fmt.Errorf(
			"failed to get Kubernetes cluster CA and endpoint: %w",
			errors.New("cluster config not found in kubeconfig"),
		)
	}

	// get client certificate and key
	//var cert string
	//var key string
	userFound := false
	for userName, user := range kubeConfig.AuthInfos {
		if userName == kubeConfig.CurrentContext {
			//cert = string(user.ClientCertificateData)
			//key = string(user.ClientKeyData)
			kubeConnInfo.Certificate = string(user.ClientCertificateData)
			kubeConnInfo.Key = string(user.ClientKeyData)
			userFound = true
		}
	}
	if !userFound {
		return &kubeConnInfo, fmt.Errorf(
			"failed to get user credentials to Kubernetes cluster: %w",
			errors.New("kubeconfig user for threeport cluster not found"),
		)
	}

	return &kubeConnInfo, nil
}
