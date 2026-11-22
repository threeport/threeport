package handlers

import (
	"errors"
	"fmt"
	"net/http"
	"sort"

	api_v0 "github.com/threeport/threeport/pkg/api/v0"
	client_lib "github.com/threeport/threeport/pkg/client/lib/v0"
	client "github.com/threeport/threeport/pkg/client/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// init registers the reader the delete check uses for a child's runtimes.
func init() {
	api_v0.ChildRuntimeNames = childRuntimeNames
}

// childRuntimeNames lists the kubernetes runtimes a child manages, other than the one it runs on.
func childRuntimeNames(controlPlane *api_v0.ControlPlaneInstance) ([]string, error) {
	// a child with no endpoint has no database of its own yet
	if controlPlane.ApiServerEndpoint == nil || *controlPlane.ApiServerEndpoint == "" {
		return nil, nil
	}

	// open a client for the child's api
	httpClient, err := controlPlaneHTTPClient(controlPlane)
	if err != nil {
		return nil, err
	}

	// read the child's runtimes, and its host id when a self row exists
	self, err := client.GetSelfControlPlaneInstance(httpClient, *controlPlane.ApiServerEndpoint)
	var hostID *uint
	if err != nil && !errors.Is(err, client.ErrNoLocalControlPlane) {
		return nil, fmt.Errorf("failed to read the child control plane: %w", err)
	}
	if err == nil {
		hostID = self.KubernetesRuntimeInstanceID
	}
	runtimes, err := client.GetKubernetesRuntimeInstances(httpClient, *controlPlane.ApiServerEndpoint)
	if err != nil {
		return nil, fmt.Errorf("failed to read kubernetes runtime instances: %w", err)
	}

	// keep every runtime except the one the child runs on
	var names []string
	for _, runtime := range *runtimes {
		if hostID != nil && runtime.ID != nil && *runtime.ID == *hostID {
			continue
		}
		if runtime.Name != nil {
			names = append(names, *runtime.Name)
		}
	}
	sort.Strings(names)
	return names, nil
}

// controlPlaneHTTPClient builds a client for the control plane this row manages.
func controlPlaneHTTPClient(controlPlane *api_v0.ControlPlaneInstance) (*http.Client, error) {
	// use a plain client when the child has no client certificate
	if controlPlane.ClientCert == nil || *controlPlane.ClientCert == "" {
		return client_lib.GetHTTPClient(false, "", "", "", "")
	}

	// decode the certificates the create path stored as base64
	if controlPlane.CACert == nil || controlPlane.ClientKey == nil {
		return nil, fmt.Errorf("control plane %s is missing a client certificate", util.DerefString(controlPlane.Name))
	}
	ca, err := util.Base64Decode(*controlPlane.CACert)
	if err != nil {
		return nil, fmt.Errorf("failed to decode ca cert: %w", err)
	}
	cert, err := util.Base64Decode(*controlPlane.ClientCert)
	if err != nil {
		return nil, fmt.Errorf("failed to decode client cert: %w", err)
	}
	key, err := util.Base64Decode(*controlPlane.ClientKey)
	if err != nil {
		return nil, fmt.Errorf("failed to decode client key: %w", err)
	}
	return client_lib.GetHTTPClient(true, ca, cert, key, "")
}
