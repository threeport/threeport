package v0

import (
	"errors"
	"fmt"
	"strings"
)

// ParseImage takes a full image and returns the registry, name and tag values.
func ParseImage(image string) (string, string, string, error) {
	// extract tag
	tagSplit := strings.Split(image, ":")
	imageTag := tagSplit[1]

	var imageRegistry string
	var imageName string
	imageSplit := strings.Split(tagSplit[0], "/")
	switch len(imageSplit) {
	case 2:
		imageRegistry = imageSplit[0]
		imageName = imageSplit[1]
	case 3:
		imageRegistry = fmt.Sprintf("%s/%s", imageSplit[0], imageSplit[1])
		imageName = imageSplit[2]
	default:
		return "", "", "", errors.New(fmt.Sprintf("unable to parse image %s", image))
	}

	return imageRegistry, imageName, imageTag, nil
}
