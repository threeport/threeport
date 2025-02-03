package tptdev

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/kind/pkg/cluster"
)

const (
	registryImage = "registry:2"
	registryName  = "local-registry"
	registryPort  = "5001"
)

// CreateLocalRegistry starts a Docker container to serve as a local container
// registry.  If a local registry already exists with the <registryName> name,
// it will return without error
func CreateLocalRegistry() error {
	ctx := context.Background()
	cli, err := client.NewClientWithOpts(client.FromEnv)
	if err != nil {
		return fmt.Errorf("failed to create Docker client: %w", err)
	}
	defer cli.Close()

	_, err = cli.ContainerInspect(ctx, registryName)
	if err == nil {
		// registry already exists
		return nil
	}

	// pull image if it doesn't exist locally
	_, _, err = cli.ImageInspectWithRaw(ctx, registryImage)
	if err != nil {
		// image does not exist, pull it
		reader, err := cli.ImagePull(ctx, registryImage, image.PullOptions{})
		if err != nil {
			return fmt.Errorf("failed to pull registry image: %w", err)
		}
		defer reader.Close()
		// read the output of the pull process
		io.Copy(os.Stdout, reader)
	}

	config := &container.Config{
		Image: registryImage,
		ExposedPorts: nat.PortSet{
			"5000/tcp": struct{}{},
		},
	}
	hostConfig := &container.HostConfig{
		RestartPolicy: container.RestartPolicy{
			Name: "always",
		},
		PortBindings: nat.PortMap{
			"5000/tcp": []nat.PortBinding{
				{
					HostIP:   "127.0.0.1",
					HostPort: registryPort,
				},
			},
		},
	}
	networkingConfig := &network.NetworkingConfig{}
	resp, err := cli.ContainerCreate(ctx, config, hostConfig, networkingConfig, nil, registryName)
	if err != nil {
		return fmt.Errorf("failed to create registry container: %w", err)
	}

	if err := cli.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		return fmt.Errorf("failed to start registry container: %w", err)
	}

	return nil
}

// ConnectLocalRegistry connects a local Docker container registry to a kind cluster.
func ConnectLocalRegistry(clusterName string) error {
	ctx := context.Background()
	cli, err := client.NewClientWithOpts(client.FromEnv)
	if err != nil {
		return fmt.Errorf("failed to create Docker client: %w", err)
	}
	defer cli.Close()

	provider := cluster.NewProvider()
	nodes, err := provider.ListNodes(clusterName)
	if err != nil {
		return fmt.Errorf("failed to list nodes for kind cluster: %w", err)
	}

	registryDir := fmt.Sprintf("/etc/containerd/certs.d/localhost:%s", registryPort)
	for _, node := range nodes {
		cmd := exec.Command("docker", "exec", node.String(), "mkdir", "-p", registryDir)
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("failed to make directory in kind node container: %w", err)
		}

		hostsToml := "[host.\"http://local-registry:5000\"]\n"
		cmd = exec.Command("docker", "exec", "-i", node.String(), "sh", "-c", fmt.Sprintf("echo '%s' > %s/hosts.toml", hostsToml, registryDir))
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("failed to configure local registry networking in kind cluster node: %w", err)
		}
	}

	inspect, err := cli.ContainerInspect(ctx, registryName)
	if err != nil {
		return fmt.Errorf("failed to inspect kind node container to configure docker network: %w", err)
	}

	if _, ok := inspect.NetworkSettings.Networks["kind"]; !ok {
		if err := cli.NetworkConnect(ctx, "kind", registryName, nil); err != nil {
			return fmt.Errorf("failed to configure docker network to connect registry to kind cluster: %w", err)
		}
	}

	// https://github.com/kubernetes/enhancements/tree/master/keps/sig-cluster-lifecycle/generic/1755-communicating-a-local-registry
	config := `apiVersion: v1
kind: ConfigMap
metadata:
  name: local-registry-hosting
  namespace: kube-public
data:
  localRegistryHosting.v1: |
    host: "localhost:%s"
    help: "https://kind.sigs.k8s.io/docs/user/local-registry/"`

	config = fmt.Sprintf(config, registryPort)

	return applyK8sConfig(config)
}

// DeleteLocalRegistry stops and removes the Docker container running the local
// container registry.
func DeleteLocalRegistry() error {
	ctx := context.Background()
	cli, err := client.NewClientWithOpts(client.FromEnv)
	if err != nil {
		return fmt.Errorf("failed to create Docker client: %w", err)
	}
	defer cli.Close()

	if err := cli.ContainerStop(ctx, registryName, container.StopOptions{}); err != nil {
		return fmt.Errorf("failed to stop registry docker container: %w", err)
	}

	if err := cli.ContainerRemove(ctx, registryName, container.RemoveOptions{}); err != nil {
		return fmt.Errorf("failed to remove registry docker container: %w", err)
	}

	return nil
}

// applyK8sConfig creates a configmap to in the Kubernetes cluster.
func applyK8sConfig(config string) error {
	kubeconfig := os.Getenv("KUBECONFIG")
	if kubeconfig == "" {
		kubeconfig = clientcmd.RecommendedHomeFile
	}
	restConfig, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		return fmt.Errorf("failed to generate Kubernetes REST config from kubeconfig: %w", err)
	}

	clientset, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return fmt.Errorf("failed to create new clientset for Kubernetes: %w", err)
	}

	configMap := &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "local-registry-hosting",
			Namespace: "kube-public",
		},
		Data: map[string]string{
			"localRegistryHosting.v1": fmt.Sprintf("host: \"localhost:%s\"\nhelp: \"https://kind.sigs.k8s.io/docs/user/local-registry/\"", registryPort),
		},
	}

	if _, err = clientset.CoreV1().ConfigMaps("kube-public").Create(context.TODO(), configMap, metav1.CreateOptions{}); err != nil {
		return fmt.Errorf("failed to create configmap for local registry: %w", err)
	}

	return nil
}
