package tptdev

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/archive"
	"sigs.k8s.io/kind/pkg/cluster"
	"sigs.k8s.io/kind/pkg/cluster/nodes"
	"sigs.k8s.io/kind/pkg/cluster/nodeutils"
	"sigs.k8s.io/kind/pkg/cmd"
	"sigs.k8s.io/kind/pkg/errors"
	"sigs.k8s.io/kind/pkg/exec"
	"sigs.k8s.io/kind/pkg/fs"

	threeport "github.com/threeport/threeport/pkg/threeport-installer/v0"
)

type BuildErrorLine struct {
	Error       string      `json:"error"`
	ErrorDetail ErrorDetail `json:"errorDetail"`
}

type ErrorDetail struct {
	Message string `json:"message"`
}

type imageTagFetcher func(nodes.Node, string) (map[string]bool, error)

// PrepareDevImages builds and loads the threeport control plane images for
// development use.
func PrepareDevImages(threeportPath, kindKubernetesRuntimeName string, cpi *threeport.ControlPlaneInstaller) error {
	// devImages := cpi.ThreeportDevImages()

	if err := BuildDevImage(threeportPath); err != nil {
		return fmt.Errorf("failed to build dev images: %w", err)
	}

	if err := LoadDevImage(kindKubernetesRuntimeName); err != nil {
		return fmt.Errorf("failed to load dev images to kind cluster: %w", err)
	}

	return nil
}

// BuildDevImage builds all the threeport control plane container images using
// the dev dockerfile to provide live reload of code in the container.
func BuildDevImage(threeportPath string) error {
	dockerClient, err := client.NewClientWithOpts(
		client.FromEnv,
		client.WithAPIVersionNegotiation(),
	)
	if err != nil {
		return fmt.Errorf("failed to create docker client for building images: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*300)
	defer cancel()

	tar, err := archive.TarWithOptions(threeportPath, &archive.TarOptions{})
	if err != nil {
		return fmt.Errorf("failed to build tarball of threeport repo: %w", err)
	}

	imageName := "threeport-air"
	buildOpts := types.ImageBuildOptions{
		Dockerfile: filepath.Join("cmd", "dev", "Dockerfile-dev"),
		Tags:       []string{imageName},
		Remove:     true,
	}

	result, err := dockerClient.ImageBuild(ctx, tar, buildOpts)
	if err != nil {
		return fmt.Errorf("failed to build docker image %s: %w", imageName, err)
	}
	defer result.Body.Close()

	if err := buildOutput(result.Body); err != nil {
		return fmt.Errorf("failed to write output from docker build for %s: %w", imageName, err)
	}

	return nil
}

// LoadDevImage loads the threeport control plane development container images
// onto the kind cluster nodes.
func LoadDevImage(kindKubernetesRuntimeName string) error {
	logger := cmd.NewLogger()
	provider := cluster.NewProvider(
		cluster.ProviderWithLogger(logger),
	)

	// check that the image exists locally and gets its ID, if not return error
	imageName := "threeport-air"
	imageID, err := imageID(imageName)
	if err != nil {
		return fmt.Errorf("image: %q not present locally", imageName)
	}

	// check that the cluster nodes exist
	nodeList, err := provider.ListInternalNodes(kindKubernetesRuntimeName)
	if err != nil {
		return err
	}
	if len(nodeList) == 0 {
		return fmt.Errorf("no nodes found for cluster %q", kindKubernetesRuntimeName)
	}

	// map cluster nodes by their name
	nodesByName := map[string]nodes.Node{}
	for _, node := range nodeList {
		nodesByName[node.String()] = node
	}

	// we want to load container images to all nodes - no need to select
	// specific nodes
	candidateNodes := nodeList

	// pick only the nodes that don't have the image
	selectedNodes := map[string]nodes.Node{}
	fns := []func() error{}
	processed := false
	for _, node := range candidateNodes {
		exists := checkIfImageExists(node, imageID, imageName, nodeutils.ImageTags)
		if exists {
			continue
		}

		id, err := nodeutils.ImageID(node, imageName)
		if err != nil || id != imageID {
			selectedNodes[node.String()] = node
			logger.V(0).Infof("Image: %q with ID %q not yet present on node %q, loading...", imageName, imageID, node.String())
		}
		continue
	}
	if len(selectedNodes) == 0 && !processed {
		logger.V(0).Infof("Image: %q with ID %q found to be already present on all nodes.", imageName, imageID)
	}

	// return early if no node needs the image
	if len(selectedNodes) == 0 {
		return nil
	}

	// setup the tar path where the images will be saved
	dir, err := fs.TempDir("", "images-tar")
	if err != nil {
		return errors.Wrap(err, "failed to create tempdir")
	}
	defer os.RemoveAll(dir)
	imagesTarPath := filepath.Join(dir, "images.tar")
	// save the images into a tar
	imageNames := []string{imageName}
	err = save(imageNames, imagesTarPath)
	if err != nil {
		return err
	}

	// load the images on the selected nodes
	for _, selectedNode := range selectedNodes {
		selectedNode := selectedNode // capture loop variable
		fns = append(fns, func() error {
			return loadImage(imagesTarPath, selectedNode)
		})
	}
	return errors.UntilErrorConcurrent(fns)
}

func buildOutput(reader io.Reader) error {
	var lastLine string

	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		lastLine = scanner.Text()
		fmt.Println(scanner.Text())
	}

	errLine := &BuildErrorLine{}
	json.Unmarshal([]byte(lastLine), errLine)
	if errLine.Error != "" {
		return errors.New(errLine.Error)
	}

	if err := scanner.Err(); err != nil {
		return err
	}

	return nil
}

// imageID return the ID of the container image
func imageID(containerNameOrID string) (string, error) {
	cmd := exec.Command("docker", "image", "inspect",
		"-f", "{{ .Id }}",
		containerNameOrID, // ... against the container
	)
	lines, err := exec.OutputLines(cmd)
	if err != nil {
		return "", err
	}
	if len(lines) != 1 {
		return "", errors.Errorf("Docker image ID should only be one line, got %d lines", len(lines))
	}
	return lines[0], nil
}

// checkIfImageExists makes sure we only perform the reverse lookup of the ImageID to tag map
func checkIfImageExists(
	node nodes.Node,
	imageID string,
	imageName string,
	tagFetcher imageTagFetcher,
	// ) (exists, reTagRequired bool, sanitizedImage string) {
) (exists bool) {
	tags, err := tagFetcher(node, imageID)
	if len(tags) == 0 || err != nil {
		exists = false
		return
	}
	exists = true

	return
}

// save saves images to dest, as in `docker save`
func save(images []string, dest string) error {
	commandArgs := append([]string{"save", "-o", dest}, images...)
	return exec.Command("docker", commandArgs...).Run()
}

// loads an image tarball onto a node
func loadImage(imageTarName string, node nodes.Node) error {
	f, err := os.Open(imageTarName)
	if err != nil {
		return errors.Wrap(err, "failed to open image")
	}
	defer f.Close()
	return nodeutils.LoadImageArchive(node, f)
}
