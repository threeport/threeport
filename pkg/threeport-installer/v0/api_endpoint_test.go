package v0

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	k8stesting "k8s.io/client-go/testing"

	v0 "github.com/threeport/threeport/pkg/api/v0"
)

const apiEndpointTestNamespace = "threeport-control-plane"

// apiServiceWithIngress returns the threeport API's Service, with the given
// load balancer ingress in its status.
func apiServiceWithIngress(ingress ...interface{}) *unstructured.Unstructured {
	service := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "Service",
		"metadata": map[string]interface{}{
			"name":      ThreeportAPIServiceResourceName,
			"namespace": apiEndpointTestNamespace,
		},
		"spec": map[string]interface{}{"type": "LoadBalancer"},
	}}
	if len(ingress) > 0 {
		service.Object["status"] = map[string]interface{}{
			"loadBalancer": map[string]interface{}{"ingress": ingress},
		}
	}

	return service
}

// apiEndpointTestInstaller returns an installer for a control plane on
// provider.
func apiEndpointTestInstaller(provider string) *ControlPlaneInstaller {
	cpi := &ControlPlaneInstaller{}
	cpi.Opts.Namespace = apiEndpointTestNamespace
	cpi.Opts.InfraProvider = provider
	cpi.Opts.RestApiInfo = &v0.ControlPlaneComponent{ServiceResourceName: ThreeportAPIServiceResourceName}

	return cpi
}

// apiServiceMapper maps the core v1 Service kind.
func apiServiceMapper() meta.RESTMapper {
	mapper := meta.NewDefaultRESTMapper([]schema.GroupVersion{{Version: "v1"}})
	mapper.Add(schema.GroupVersionKind{Version: "v1", Kind: "Service"}, meta.RESTScopeNamespace)

	return mapper
}

// fastApiLoadBalancerWait shortens the wait for the load balancer for the
// length of a test.
func fastApiLoadBalancerWait(t *testing.T, timeout time.Duration) {
	t.Helper()
	previousTimeout, previousInterval := apiLoadBalancerTimeout, apiLoadBalancerPollInterval
	apiLoadBalancerTimeout, apiLoadBalancerPollInterval = timeout, 10*time.Millisecond
	t.Cleanup(func() {
		apiLoadBalancerTimeout, apiLoadBalancerPollInterval = previousTimeout, previousInterval
	})
}

// TestTheWaitForTheApiLoadBalancerIsLongEnough covers #645: a GKE network load
// balancer took about 65 seconds to get its address, and a one-minute wait
// failed the install seconds before it.
func TestTheWaitForTheApiLoadBalancerIsLongEnough(t *testing.T) {
	assert.GreaterOrEqual(t, apiLoadBalancerTimeout, 5*time.Minute)
	assert.Less(t, apiLoadBalancerPollInterval, apiLoadBalancerTimeout)
}

// TestTheApiEndpointIsTheAddressTheLoadBalancerGets covers the field each
// provider gives the address in.
func TestTheApiEndpointIsTheAddressTheLoadBalancerGets(t *testing.T) {
	fastApiLoadBalancerWait(t, 100*time.Millisecond)
	for provider, ingress := range map[string]map[string]interface{}{
		v0.KubernetesRuntimeInfraProviderEKS: {"hostname": "lb.elb.amazonaws.com"},
		v0.KubernetesRuntimeInfraProviderGKE: {"ip": "34.1.2.3"},
		v0.KubernetesRuntimeInfraProviderOKE: {"ip": "129.1.2.3"},
	} {
		t.Run(provider, func(t *testing.T) {
			kubeClient := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme(), apiServiceWithIngress(ingress))

			endpoint, err := apiEndpointTestInstaller(provider).GetThreeportAPIEndpoint(kubeClient, apiServiceMapper())
			require.NoError(t, err)
			for _, address := range ingress {
				assert.Equal(t, address, endpoint)
			}
		})
	}
}

// TestTheWaitLastsUntilTheLoadBalancerHasAnAddress covers the address
// arriving after the first checks, as it does on every cloud.
func TestTheWaitLastsUntilTheLoadBalancerHasAnAddress(t *testing.T) {
	fastApiLoadBalancerWait(t, 5*time.Second)
	kubeClient := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme(), apiServiceWithIngress())

	// the cloud provider sets the address on the fourth check
	var gets atomic.Int32
	kubeClient.PrependReactor("get", "services", func(k8stesting.Action) (bool, runtime.Object, error) {
		if gets.Add(1) < 4 {
			return true, apiServiceWithIngress(), nil
		}

		return true, apiServiceWithIngress(map[string]interface{}{"ip": "34.1.2.3"}), nil
	})

	endpoint, err := apiEndpointTestInstaller(v0.KubernetesRuntimeInfraProviderGKE).GetThreeportAPIEndpoint(kubeClient, apiServiceMapper())
	require.NoError(t, err)
	assert.Equal(t, "34.1.2.3", endpoint)
	assert.Equal(t, int32(4), gets.Load())
}

func TestTheWaitForTheApiLoadBalancerEnds(t *testing.T) {
	fastApiLoadBalancerWait(t, 100*time.Millisecond)

	for name, ingress := range map[string][]interface{}{
		"no ingress yet":             nil,
		"an ingress with no address": {map[string]interface{}{"ports": []interface{}{}}},
	} {
		t.Run(name, func(t *testing.T) {
			kubeClient := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme(), apiServiceWithIngress(ingress...))

			_, err := apiEndpointTestInstaller(v0.KubernetesRuntimeInfraProviderGKE).GetThreeportAPIEndpoint(kubeClient, apiServiceMapper())
			require.Error(t, err)
			assert.Contains(t, err.Error(), "timed out after 100ms")
			assert.NotContains(t, err.Error(), "%!w", "the error says what is missing rather than wrapping a nil error")
		})
	}
}
