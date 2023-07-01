package cli

import (
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/nukleros/eks-cluster/pkg/resource"
	kubeerrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/client-go/dynamic"

	"github.com/threeport/threeport/internal/kube"
	"github.com/threeport/threeport/internal/kubernetesruntime/mapping"
	"github.com/threeport/threeport/internal/provider"
	"github.com/threeport/threeport/internal/threeport"
	"github.com/threeport/threeport/internal/tptdev"
	"github.com/threeport/threeport/internal/util"
	v0 "github.com/threeport/threeport/pkg/api/v0"
	"github.com/threeport/threeport/pkg/auth/v0"
	client "github.com/threeport/threeport/pkg/client/v0"
	config "github.com/threeport/threeport/pkg/config/v0"
)

var ThreeportConfigAlreadyExistsErr = errors.New("threeport control plane with provided name already exists in threeport config")

// ControlPlaneCLIArgs is the set of control plane arguments passed to one of
// the CLI tools.
type ControlPlaneCLIArgs struct {
	AuthEnabled             bool
	AwsConfigProfile        string
	AwsConfigEnv            bool
	AwsRegion               string
	CfgFile                 string
	ControlPlaneImageRepo   string
	ControlPlaneImageTag    string
	CreateRootDomain        string
	CreateProviderAccountID string
	CreateAdminEmail        string
	DevEnvironment          bool
	ForceOverwriteConfig    bool
	InstanceName            string
	InfraProvider           string
	KubeconfigPath          string
	NumWorkerNodes          int
	ProviderConfigDir       string
	ThreeportLocalAPIPort   int
	ThreeportPath           string
}
