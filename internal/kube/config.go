package kube

import (
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/threeport/threeport/internal/util"
	"k8s.io/client-go/tools/clientcmd"
)

// KubeConnectionInfo contains the necessary info to connect to a Kubernetes
// API.
type KubeConnectionInfo struct {
	APIEndpoint   string `yaml:"APIEndpoint"`
	CACertificate string `yaml:"CACertificate"`
	Certificate   string `yaml:"Certificate"`
	Key           string `yaml:"Key"`
	EKSToken      string `yaml:"EKSToken"`
}

// DefaultKubeconfig returns the path to the user's default kubeconfig.
func DefaultKubeconfig() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to user's home directory: %w", err)
	}

	return filepath.Join(homeDir, ".kube", "config"), nil
}

// GetConnectionInfoFromKubeconfig extracts the Kubernetes API connection info
// from a kubeconfig.
func GetConnectionInfoFromKubeconfig(kubeconfig string) (*KubeConnectionInfo, error) {
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
	clusterFound := false
	for clusterName, cluster := range kubeConfig.Clusters {
		if clusterName == kubeConfig.CurrentContext {
			kubeConnInfo.CACertificate = util.Base64Encode(string(cluster.CertificateAuthorityData))
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
	userFound := false
	for userName, user := range kubeConfig.AuthInfos {
		if userName == kubeConfig.CurrentContext {
			kubeConnInfo.Certificate = util.Base64Encode(string(user.ClientCertificateData))
			kubeConnInfo.Key = util.Base64Encode(string(user.ClientKeyData))
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
