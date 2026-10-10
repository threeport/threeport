package e2e_test

import (
	"flag"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var (
	provider         string
	imageRepo        string
	threeportPath    string
	clean            bool
	awsRegion        string
	awsConfigProfile string
	apiPort          int
)

func init() {
	flag.StringVar(&provider, "provider", "kind", "Infrastructure provider for the control plane (kind or eks)")
	flag.StringVar(&imageRepo, "image-repo", "", "Container image repo to use for test images.  Empty uses the released control plane images, which is the only option that works with a remote provider unless you have a repo its nodes can pull from.")
	flag.StringVar(&threeportPath, "threeport-path", "", "Path to root of Threeport repo")
	flag.BoolVar(&clean, "clean", true, "Remove Threeport control plane and image repo where applicable after e2e tests")
	flag.StringVar(&awsRegion, "aws-region", "us-east-1", "AWS region to install into when using the eks provider")
	flag.StringVar(&awsConfigProfile, "aws-config-profile", "default", "AWS config profile to use when using the eks provider")
	flag.IntVar(&apiPort, "api-port", 0, "Host port to serve the Threeport API on with the kind provider.  Set this when another control plane already holds the default, which otherwise fails the cluster create with a port conflict.")
}

func TestE2e(t *testing.T) {
	RegisterFailHandler(Fail)
	flag.Parse()
	RunSpecs(t, "E2e Suite")
}
