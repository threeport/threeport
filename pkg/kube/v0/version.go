package v0

import (
	"fmt"
	"regexp"
	"strings"

	"k8s.io/client-go/discovery"
)

// gitVersionMinor reads the major and minor version from a Kubernetes git
// version such as "v1.36.2-eks-1a2b3c4".
var gitVersionMinor = regexp.MustCompile(`^v(\d+)\.(\d+)\.`)

// ServerMinorVersion returns the Kubernetes minor version a cluster's API
// server runs, such as "1.36".  Providers may mark the minor version, as EKS
// does with "36+", so the marks are dropped, and the git version is read when
// the minor version is missing.
func ServerMinorVersion(client discovery.ServerVersionInterface) (string, error) {
	info, err := client.ServerVersion()
	if err != nil {
		return "", fmt.Errorf("failed to get the Kubernetes API server's version: %w", err)
	}

	major := strings.TrimRight(info.Major, "+")
	minor := strings.TrimRight(info.Minor, "+")
	if major == "" || minor == "" {
		match := gitVersionMinor.FindStringSubmatch(info.GitVersion)
		if match == nil {
			return "", fmt.Errorf("the Kubernetes API server reports no version it can be read from: %+v", *info)
		}
		major, minor = match[1], match[2]
	}

	return major + "." + minor, nil
}
