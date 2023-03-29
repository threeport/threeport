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
)

type BuildErrorLine struct {
	Error       string      `json:"error"`
	ErrorDetail ErrorDetail `json:"errorDetail"`
}

type ErrorDetail struct {
	Message string `json:"message"`
}

type imageTagFetcher func(nodes.Node, string) (map[string]bool, error)

// devImages returns a map of main package dirs to image names
func DevImages() map[string]string {
	return map[string]string{
		"rest-api":            "threeport-rest-api-dev:latest",
		"workload-controller": "threeport-workload-controller-dev:latest",
	}
}

// PrepareDevImages builds and loads the threeport control plane images for
// development use.
func PrepareDevImages(threeportPath, kindClusterName string) error {
	devImages := DevImages()

	if err := BuildDevImages(threeportPath, devImages); err != nil {
		return fmt.Errorf("failed to build dev images: %w", err)
	}

	if err := LoadDevImages(kindClusterName, devImages); err != nil {
		return fmt.Errorf("failed to load dev images to kind cluster: %w", err)
	}

	return nil
}

// BuildDevImages builds all the threeport control plane container images using
// the dev dockerfile to provide live reload of code in the container.
func BuildDevImages(threeportPath string, devImages map[string]string) error {
	dockerClient, err := client.NewClientWithOpts(
		client.FromEnv,
		client.WithAPIVersionNegotiation(),
	)
	if err != nil {
		return fmt.Errorf("failed to create docker client for building images: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*300)
	defer cancel()

	for buildDir, imageName := range devImages {
		tar, err := archive.TarWithOptions(threeportPath, &archive.TarOptions{})
		if err != nil {
			return fmt.Errorf("failed to build tarball of threeport repo: %w", err)
		}

		buildOpts := types.ImageBuildOptions{
			Dockerfile: filepath.Join("cmd", buildDir, "image", "Dockerfile-dev"),
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
	}

	return nil
}

// LoadDevImages loads the threeport control plane development container images
// onto the kind cluster nodes.
func LoadDevImages(kindClusterName string, devImages map[string]string) error {
	logger := cmd.NewLogger()
	provider := cluster.NewProvider(
		cluster.ProviderWithLogger(logger),
	)

	// check that the image exists locally and gets its ID, if not return error
	var imageIDs []string
	var imageNames []string
	for _, imageName := range devImages {
		imgID, err := imageID(imageName)
		if err != nil {
			return fmt.Errorf("image: %q not present locally", imageName)
		}
		imageIDs = append(imageIDs, imgID)
		imageNames = append(imageNames, imageName)
	}

	// check that the cluster nodes exist
	nodeList, err := provider.ListInternalNodes(kindClusterName)
	if err != nil {
		return err
	}
	if len(nodeList) == 0 {
		return fmt.Errorf("no nodes found for cluster %q", kindClusterName)
	}

	// map cluster nodes by their name
	nodesByName := map[string]nodes.Node{}
	for _, node := range nodeList {
		nodesByName[node.String()] = node
	}

	// we want to load container images to all nodes - no need to select
	// specific nodes
	candidateNodes := nodeList
	//if len(flags.Nodes) > 0 {
	//	candidateNodes = []nodes.Node{}
	//	for _, name := range flags.Nodes {
	//		node, ok := nodesByName[name]
	//		if !ok {
	//			return fmt.Errorf("unknown node: %q", name)
	//		}
	//		candidateNodes = append(candidateNodes, node)
	//	}
	//}

	// pick only the nodes that don't have the image
	selectedNodes := map[string]nodes.Node{}
	fns := []func() error{}
	for i, imageName := range imageNames {
		imageID := imageIDs[i]
		processed := false
		for _, node := range candidateNodes {
			//exists, reTagRequired, sanitizedImageName := checkIfImageReTagRequired(node, imageID, imageName, nodeutils.ImageTags)
			exists := checkIfImageExists(node, imageID, imageName, nodeutils.ImageTags)
			//if exists && !reTagRequired {
			if exists {
				continue
			}

			//if reTagRequired {
			//	// We will try to re-tag the image. If the re-tag fails, we will fall back to the default behavior of loading
			//	// the images into the nodes again
			//	logger.V(0).Infof("Image with ID: %s already present on the node %s but is missing the tag %s. re-tagging...", imageID, node.String(), sanitizedImageName)
			//	if err := nodeutils.ReTagImage(node, imageID, sanitizedImageName); err != nil {
			//		logger.Errorf("failed to re-tag image on the node %s due to an error %s. Will load it instead...", node.String(), err)
			//		selectedNodes[node.String()] = node
			//	} else {
			//		processed = true
			//	}
			//	continue
			//}
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
	}

	// return early if no node needs the image
	if len(selectedNodes) == 0 {
		return nil
	}

	// Setup the tar path where the images will be saved
	dir, err := fs.TempDir("", "images-tar")
	if err != nil {
		return errors.Wrap(err, "failed to create tempdir")
	}
	defer os.RemoveAll(dir)
	imagesTarPath := filepath.Join(dir, "images.tar")
	// Save the images into a tar
	err = save(imageNames, imagesTarPath)
	if err != nil {
		return err
	}

	// Load the images on the selected nodes
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
	//sanitizedImage = sanitizeImage(imageName)
	//if ok := tags[sanitizedImage]; ok {
	//	reTagRequired = false
	//	return
	//}
	//reTagRequired = true
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

//// sanitizeImage is a helper to return human readable image name
//func sanitizeImage(image string) (sanitizedName string) {
//	const (
//		defaultDomain    = "docker.io/"
//		officialRepoName = "library"
//	)
//	sanitizedName = image
//
//	if !strings.ContainsRune(image, '/') {
//		sanitizedName = officialRepoName + "/" + image
//	}
//
//	i := strings.IndexRune(sanitizedName, '/')
//	if i == -1 || (!strings.ContainsAny(sanitizedName[:i], ".:") && sanitizedName[:i] != "localhost") {
//		sanitizedName = defaultDomain + sanitizedName
//	}
//
//	i = strings.IndexRune(sanitizedName, ':')
//	if i == -1 {
//		sanitizedName += ":latest"
//	}
//
//	return
//}
