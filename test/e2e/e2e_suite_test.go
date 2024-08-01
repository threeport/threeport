package e2e_test

import (
	"flag"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var (
	provider      string
	imageRepo     string
	threeportPath string
	clean         bool
)

func init() {
	flag.StringVar(&provider, "provider", "kind", "Infrastructure provider for the control plane (kind or eks)")
	flag.StringVar(&imageRepo, "image-repo", "", "Conatiner image repo to use for test images")
	flag.StringVar(&threeportPath, "threeport-path", "", "Path to root of Threeport repo")
	flag.BoolVar(&clean, "clean", true, "Remove Threeport control plane and image repo where applicable after e2e tests")
}

func TestE2e(t *testing.T) {
	RegisterFailHandler(Fail)
	flag.Parse()
	RunSpecs(t, "E2e Suite")
}
