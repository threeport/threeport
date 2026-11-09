package v0

import (
	"fmt"
	"strings"
)

// ParseImage splits a full image into the registry, name, and tag.
// The registry is every path segment except the last.
func ParseImage(image string) (string, string, string, error) {
	// split the image name from the registry
	imageSplit := strings.Split(image, "/")
	if len(imageSplit) < 2 {
		return "", "", "", fmt.Errorf("unable to parse image %s", image)
	}
	imageRegistry := strings.Join(imageSplit[:len(imageSplit)-1], "/")
	imageName := imageSplit[len(imageSplit)-1]

	// split the image tag from the image name
	tagSplit := strings.Split(imageName, ":")
	if len(tagSplit) < 2 || tagSplit[1] == "" {
		return "", "", "", fmt.Errorf("failed to parse image %s, image has no tag", image)
	}
	imageName = tagSplit[0]
	imageTag := tagSplit[1]

	return imageRegistry, imageName, imageTag, nil
}
