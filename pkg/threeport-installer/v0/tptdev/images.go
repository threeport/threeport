package tptdev

import (
	"bufio"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/registry"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/archive"
	"sigs.k8s.io/kind/pkg/cluster"
	"sigs.k8s.io/kind/pkg/cluster/nodes"
	"sigs.k8s.io/kind/pkg/cluster/nodeutils"
	"sigs.k8s.io/kind/pkg/cmd"
	"sigs.k8s.io/kind/pkg/errors"
	k8sexec "sigs.k8s.io/kind/pkg/exec"
	"sigs.k8s.io/kind/pkg/fs"

	v0 "github.com/threeport/threeport/pkg/api/v0"
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

// PrepareDevImage builds and loads the threeport control plane images for
// development use.
func PrepareDevImage(threeportPath, kindKubernetesRuntimeName string, cpi *threeport.ControlPlaneInstaller) error {

	if err := BuildDevImage(threeportPath); err != nil {
		return fmt.Errorf("failed to build dev images: %w", err)
	}

	if err := LoadDevImage(kindKubernetesRuntimeName, "threeport-air"); err != nil {
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
		Dockerfile: filepath.Join("cmd", "tptdev", "image", "Dockerfile"),
		Tags:       []string{imageName},
		Remove:     true,
		Target:     "live-reload",
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

// BuildGoBinary builds the go binary for a threeport control plane component.
func BuildGoBinary(threeportPath, arch string, component *v0.ControlPlaneComponent, noCache bool) error {
	// set name of main.go file
	main := "main_gen.go"
	if strings.Contains(component.Name, "agent") || strings.Contains(component.Name, "database-migrator") {
		main = "main.go"
	}

	// construct build arguments
	buildArgs := []string{"build"}

	// append build flags
	buildArgs = append(buildArgs, "-gcflags=\\\"all=-N -l\\\"") // escape quotes and escape char for shell

	// append no cache flag if specified
	if noCache {
		buildArgs = append(buildArgs, "-a")
	}

	// append output flag
	buildArgs = append(buildArgs, "-o")

	// append binary name
	buildArgs = append(buildArgs, "bin/"+component.BinaryName)

	// append main.go filepath
	buildArgs = append(buildArgs, "cmd/"+component.Name+"/"+main)

	fmt.Printf("go %s \n", strings.Join(buildArgs, " "))

	// construct build command
	cmd := exec.Command("go", buildArgs...)
	cmd.Env = os.Environ()
	goEnv := []string{
		"CGO_ENABLED=0",
		"GOOS=linux",
		"GOARCH=" + arch,
	}
	cmd.Env = append(cmd.Env, goEnv...)
	cmd.Dir = threeportPath

	// start build command
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to build component %s: %v\noutput:\n%s", component.Name, err, string(output))
	}

	return nil
}

// DockerBuildxImage builds a specified docker image
// with the 'docker buildx' command.
func DockerBuildxImage(threeportPath, dockerFilePath, tag, arch string, component *v0.ControlPlaneComponent) error {
	// set target for terraform controller
	buildTarget := "dev"
	if component.Name == threeport.ThreeportTerraformControllerName {
		buildTarget = "dev-terraform"
	}

	// construct build arguments
	buildArgs := []string{
		"docker",
		"buildx",
		"build",
		"--build-arg",
		fmt.Sprintf("BINARY=%s", component.BinaryName),
		"--target",
		buildTarget,
		"--load",
		"--platform=linux/" + arch,
		"-t " + tag,
		"-f " + dockerFilePath,
		threeportPath,
	}
	fmt.Println(strings.Join(buildArgs, " "))

	cmdStr := strings.Join(buildArgs, (" "))
	cmd := exec.Command("/bin/sh", "-c", cmdStr)
	cmd.Dir = threeportPath

	// start build command
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to docker build component %s: %v\noutput:\n%s", component.Name, err, string(output))
	}

	return nil
}

// PushDockerImage pushes a specified docker image to the docker registry.
func PushDockerImage(tag string) error {
	// initialize docker client
	dockerClient, err := client.NewClientWithOpts(
		client.FromEnv,
		client.WithAPIVersionNegotiation(),
	)
	if err != nil {
		return fmt.Errorf("failed to create docker client for building images: %w", err)
	}

	// initialize context
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*300)
	defer cancel()

	// get path to docker config file
	configFilePath := os.ExpandEnv("$HOME/.docker/config.json")

	// Read the Docker configuration file
	isDockerConfigPresent := true
	configFile, err := os.ReadFile(configFilePath)
	if err != nil {
		isDockerConfigPresent = false
	}

	imagePushOptions := types.ImagePushOptions{All: true}

	dockerUsername := os.Getenv("DOCKER_USERNAME")
	dockerPassword := os.Getenv("DOCKER_PASSWORD")

	// configure docker auth if credentials are present
	switch {
	case isDockerConfigPresent &&
		dockerUsername == "" &&
		dockerPassword == "":
		// Parse the JSON content of the configuration file
		var dockerConfig map[string]interface{}
		if err := json.Unmarshal(configFile, &dockerConfig); err != nil {
			fmt.Println("Error parsing Docker config JSON:", err)
			return err
		}

		// unmarshal auth map
		ok := false
		var targetMap map[string]interface{}
		if targetMap, ok = dockerConfig["auths"].(map[string]interface{}); !ok {
			return fmt.Errorf("failed to parse docker config auths")
		}
		if targetMap, ok = targetMap["https://index.docker.io/v1/"].(map[string]interface{}); !ok {
			return fmt.Errorf("failed to parse docker config auth endpoint")
		}
		var authString string
		if authString, ok = targetMap["auth"].(string); !ok {
			return fmt.Errorf("failed to parse docker config auth credentials")
		}

		// Decode the base64 auth string
		decodedBytes, err := base64.StdEncoding.DecodeString(authString)
		if err != nil {
			fmt.Println("Error decoding Base64:", err)
			return err
		}

		// parse credentials
		credentials := strings.Split(string(decodedBytes), ":")

		// configure auth config for docker client
		authConfig := registry.AuthConfig{
			Username:      credentials[0],
			Password:      credentials[1],
			ServerAddress: "https://index.docker.io/v1/",
		}
		authConfigBytes, _ := json.Marshal(authConfig)
		authConfigEncoded := base64.URLEncoding.EncodeToString(authConfigBytes)
		imagePushOptions.RegistryAuth = authConfigEncoded
	case dockerUsername != "" &&
		dockerPassword != "":
		authConfig := registry.AuthConfig{
			Username:      dockerUsername,
			Password:      dockerPassword,
			ServerAddress: "https://index.docker.io/v1/",
		}
		authConfigBytes, _ := json.Marshal(authConfig)
		authConfigEncoded := base64.URLEncoding.EncodeToString(authConfigBytes)
		imagePushOptions.RegistryAuth = authConfigEncoded
	}

	// authenticate and push image
	out, err := dockerClient.ImagePush(ctx, tag, imagePushOptions)
	if err != nil {
		return fmt.Errorf("failed to push docker image %s: %w", tag, err)
	}
	defer out.Close()

	// Copy the push output to the console
	_, err = io.Copy(os.Stdout, out)
	if err != nil {
		return fmt.Errorf("failed to copy push output to console: %w", err)
	}

	return nil
}

// LoadDevImage loads the threeport control plane development container images
// onto the kind cluster nodes.
func LoadDevImage(kindKubernetesRuntimeName, imageName string) error {
	logger := cmd.NewLogger()
	provider := cluster.NewProvider(
		cluster.ProviderWithLogger(logger),
	)

	// check that the image exists locally and gets its ID, if not return error
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
	cmd := k8sexec.Command("docker", "image", "inspect",
		"-f", "{{ .Id }}",
		containerNameOrID, // ... against the container
	)
	lines, err := k8sexec.OutputLines(cmd)
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
