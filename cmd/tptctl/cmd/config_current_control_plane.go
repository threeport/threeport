/*
Copyright © 2023 Threeport admin@threeport.io
*/
package cmd

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/spf13/cobra"

	v0 "github.com/threeport/threeport/pkg/api/v0"
	cli "github.com/threeport/threeport/pkg/cli/v0"
	client_lib "github.com/threeport/threeport/pkg/client/lib/v0"
	client "github.com/threeport/threeport/pkg/client/v0"
)

var configCurrentControlPlaneName string

// errNotChildControlPlane is returned when a control plane exists but is not a
// child of the control plane that was asked about.
var errNotChildControlPlane = errors.New("control plane is not a child of the current control plane")

// ConfigCurrentControlPlaneCmd represents the current-instance command
var ConfigCurrentControlPlaneCmd = &cobra.Command{
	Use:     "current-control-plane",
	Example: "tptctl config current-control-plane --control-plane-name testport",
	Short:   "Set a threeport control plane as the current in-use control plane",
	Long: `Set a threeport control plane as the current in-use control plane.  Once set as
the current control plane all subsequent tptctl commands will apply to that Threeport
control plane.`,
	// this command is how the current control plane gets set, so it must run
	// when there isn't one
	PreRun:       ConfigOnlyPreRunFunc,
	SilenceUsage: true,
	Run: func(cmd *cobra.Command, args []string) {
		// get threeport config
		threeportConfig, _, err := cli.GetThreeportConfig(cliArgs.ControlPlaneName)
		if err != nil {
			cli.Error("failed to get threeport config", err)
			os.Exit(1)
		}

		// We try to find the requested control plane in the current config
		// If found we set and return
		resolution := resolveControlPlaneRequest(threeportConfig, configCurrentControlPlaneName)
		if resolution.inConfig {
			// refresh the stored address and certificates, then select the entry
			if err := refreshStoredControlPlane(threeportConfig, resolution, configCurrentControlPlaneName); err != nil {
				cli.Error("failed to refresh control plane in config", err)
				os.Exit(1)
			}
			threeportConfig.SetCurrentControlPlane(configCurrentControlPlaneName)
			cli.Info("Control plane found in config")
			cli.Complete(fmt.Sprintf("Threeport control plane %s set as the current control plane", configCurrentControlPlaneName))
			return
		}

		cli.Info("Control plane not found in config")

		if resolution.current != nil {
			cli.Info("Checking if child of current control plane")

			// get threeport API client
			apiClient, err := threeportConfig.GetHTTPClient(resolution.current.Name)
			if err != nil {
				cli.Error("failed to get threeport API client", err)
				os.Exit(1)
			}

			apiEndpoint, err := threeportConfig.GetThreeportAPIEndpoint(resolution.current.Name)
			if err != nil {
				cli.Error("failed to get threeport API endpoint from config", err)
				os.Exit(1)
			}

			// If the requested control plane is not found already in config,
			// check whether it is a child of the current control plane
			child, err := findChildControlPlane(apiClient, apiEndpoint, resolution.current.Name, configCurrentControlPlaneName)
			switch {
			case err == nil:
				cli.Info("found requested control plane in children of current control plane")
				if err := updateThreeportConfigWithControlPlaneInstance(apiClient, apiEndpoint, child, threeportConfig); err != nil {
					cli.Error("failed to set control plane from its instance", err)
					os.Exit(1)
				}
				cli.Complete(fmt.Sprintf("Threeport control plane %s set as the current control plane", configCurrentControlPlaneName))
				return
			case errors.Is(err, client_lib.ErrObjectNotFound), errors.Is(err, errNotChildControlPlane):
				cli.Warning(fmt.Sprintf("%s is not a child of the current control plane %s", configCurrentControlPlaneName, resolution.current.Name))
			default:
				cli.Error("failed to check children of the current control plane", err)
				os.Exit(1)
			}
		} else {
			cli.Warning("Current control plane not found, cannot search children for requested control plane instance.")
		}

		cli.Info("Checking to see genesis control plane info for info exists")
		// If the control plane in the instance is not already present in the config and not a child of the current control plane,
		// we look ensure the genesis control plane information exists and prompt the user to do a manual tree crawl
		if resolution.genesis != nil {
			cli.Warning(fmt.Sprintf("could not find requested control plane. Try setting current control to the genesis control plane for this instance: %s and traversing the topology of your control planes", resolution.genesis.Name))
			os.Exit(1)
		}

		if resolution.any == nil {
			cli.Warning("could not find any control plane in the requested config. Please add atleast one control plane from the group")
			os.Exit(1)
		}

		// reach the genesis control plane through a control plane in the
		// config, using that control plane's own credentials
		apiClient, err := threeportConfig.GetHTTPClient(resolution.any.Name)
		if err != nil {
			cli.Error("failed to get threeport API client", err)
			os.Exit(1)
		}
		apiEndpoint, err := threeportConfig.GetThreeportAPIEndpoint(resolution.any.Name)
		if err != nil {
			cli.Error("failed to get threeport API endpoint from config", err)
			os.Exit(1)
		}

		// get control plane instances
		genesisInstance, err := client.GetGenesisControlPlaneInstance(apiClient, apiEndpoint)
		if err != nil {
			cli.Error("failed to retrieve genesis control plane instance", err)
			os.Exit(1)
		}

		if err := updateThreeportConfigWithControlPlaneInstance(apiClient, apiEndpoint, genesisInstance, threeportConfig); err != nil {
			cli.Error("failed to set genesis control plane from its instance", err)
			os.Exit(1)
		}

		cli.Warning(fmt.Sprintf("could not find requested control plane info. Current control plane set to the genesis control plane for this instance: %s. Try traversing the topology of your control planes", valueOrEmpty(genesisInstance.Name)))
		os.Exit(1)
	},
}

// controlPlaneResolution is what the threeport config says about a requested
// control plane before any API is called.
type controlPlaneResolution struct {
	// inConfig is true when the name passed to the command is already in the config.
	inConfig bool

	// The config entry for the name passed to the command
	requested *cli.ControlPlane

	// current is the config entry for the current control plane, nil when
	// there is no current control plane or it is not in the config.
	current *cli.ControlPlane

	// genesis is a genesis control plane in the config, if there is one.
	genesis *cli.ControlPlane

	// any is a control plane in the config, if there is one.
	any *cli.ControlPlane
}

// resolveControlPlaneRequest looks for the requested control plane, and for the
// control planes that can lead to it, in the threeport config.
func resolveControlPlaneRequest(threeportConfig *cli.ThreeportConfig, requested string) controlPlaneResolution {
	var resolution controlPlaneResolution
	for i := range threeportConfig.ControlPlanes {
		controlPlane := &threeportConfig.ControlPlanes[i]
		if controlPlane.Name == requested {
			resolution.inConfig = true
			resolution.requested = controlPlane
		}

		// keep a named genesis when a later entry has no name
		if controlPlane.Genesis && (resolution.genesis == nil || controlPlane.Name != "") {
			resolution.genesis = controlPlane
		}

		// the current control plane is empty after tptctl down
		if threeportConfig.CurrentControlPlane != "" && controlPlane.Name == threeportConfig.CurrentControlPlane {
			resolution.current = controlPlane
		}

		// In case we dont find the control plane or its genesis instance,
		resolution.any = controlPlane
	}

	return resolution
}

// findChildControlPlane returns the requested control plane instance when it is
// a child of the current control plane.  The by-name lookup does not load a
// control plane's children, so the requested instance is checked for the
// current control plane as its parent instead.
func findChildControlPlane(
	apiClient *http.Client,
	apiEndpoint string,
	currentName string,
	requestedName string,
) (*v0.ControlPlaneInstance, error) {
	current, err := client.GetControlPlaneInstanceByName(apiClient, apiEndpoint, currentName)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve current control plane instance: %w", err)
	}
	if current.ID == nil {
		return nil, fmt.Errorf("current control plane instance %s has no ID", currentName)
	}

	requested, err := client.GetControlPlaneInstanceByName(apiClient, apiEndpoint, requestedName)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve control plane instance %s: %w", requestedName, err)
	}
	if requested.ParentControlPlaneInstanceID == nil || *requested.ParentControlPlaneInstanceID != *current.ID {
		return nil, errNotChildControlPlane
	}

	return requested, nil
}

// updateThreeportConfigWithControlPlaneInstance adds a control plane to the
// threeport config from its instance and sets it as the current control plane.
func updateThreeportConfigWithControlPlaneInstance(
	apiClient *http.Client,
	apiEndpoint string,
	controlPlaneInstanceToSet *v0.ControlPlaneInstance,
	threeportConfig *cli.ThreeportConfig,
) error {
	if controlPlaneInstanceToSet.ControlPlaneDefinitionID == nil {
		return errors.New("control plane instance has no control plane definition")
	}
	if controlPlaneInstanceToSet.KubernetesRuntimeInstanceID == nil {
		return errors.New("control plane instance has no kubernetes runtime instance")
	}

	// Get the corresponding control plane definition
	controlPlaneDefinition, err := client.GetControlPlaneDefinitionByID(apiClient, apiEndpoint, *controlPlaneInstanceToSet.ControlPlaneDefinitionID)
	if err != nil {
		return fmt.Errorf("failed to get control plane definition associated with the control plane being set: %w", err)
	}

	kubernetesRuntimeInstance, err := client.GetKubernetesRuntimeInstanceByID(apiClient, apiEndpoint, *controlPlaneInstanceToSet.KubernetesRuntimeInstanceID)
	if err != nil {
		return fmt.Errorf("failed to get kubernetes runtime instance associated with the control plane being set: %w", err)
	}
	if kubernetesRuntimeInstance.KubernetesRuntimeDefinitionID == nil {
		return errors.New("kubernetes runtime instance has no kubernetes runtime definition")
	}

	kubernetesRuntimeDefinition, err := client.GetKubernetesRuntimeDefinitionByID(apiClient, apiEndpoint, *kubernetesRuntimeInstance.KubernetesRuntimeDefinitionID)
	if err != nil {
		return fmt.Errorf("failed to get kubernetes runtime definition associated with the control plane being set: %w", err)
	}

	threeportControlPlaneConfig, err := controlPlaneConfigFromInstance(controlPlaneInstanceToSet, controlPlaneDefinition, kubernetesRuntimeDefinition)
	if err != nil {
		return err
	}

	// keep the kubeconfig and the provider config on an entry that already exists
	for i := range threeportConfig.ControlPlanes {
		if threeportConfig.ControlPlanes[i].Name != threeportControlPlaneConfig.Name {
			continue
		}
		refreshControlPlaneClient(&threeportConfig.ControlPlanes[i], threeportControlPlaneConfig)
		threeportControlPlaneConfig = &threeportConfig.ControlPlanes[i]
		break
	}

	if err := cli.UpdateThreeportConfig(threeportConfig, threeportControlPlaneConfig); err != nil {
		return fmt.Errorf("failed to update threeport config: %w", err)
	}

	return nil
}

// refreshStoredControlPlane copies the API address and client certificates from the control plane that manages the saved entry.
// A manager that is missing or down, or a refresh that fails, leaves the saved entry in place.
func refreshStoredControlPlane(threeportConfig *cli.ThreeportConfig, resolution controlPlaneResolution, name string) error {
	// a genesis control plane has no parent to read
	if resolution.requested == nil || resolution.requested.Genesis {
		return nil
	}

	// ask the current control plane, then genesis, then the other configured control planes
	sources := refreshSources(threeportConfig, resolution)
	if len(sources) == 0 {
		return nil
	}

	// use the first source that manages this saved control plane
	apiClient, apiEndpoint, instance, found := refreshInstanceFrom(sources, func(source *cli.ControlPlane) (*http.Client, string, *v0.ControlPlaneInstance, error) {
		sourceClient, err := threeportConfig.GetHTTPClient(source.Name)
		if err != nil {
			return nil, "", nil, err
		}
		sourceEndpoint, err := threeportConfig.GetThreeportAPIEndpoint(source.Name)
		if err != nil {
			return nil, "", nil, err
		}
		sourceInstance, err := findChildControlPlane(sourceClient, sourceEndpoint, source.Name, name)
		if err != nil {
			return nil, "", nil, err
		}

		// reject a same-named child of another control plane
		if !sameSavedControlPlane(resolution.requested, sourceInstance) {
			return nil, "", nil, errNotChildControlPlane
		}
		return sourceClient, sourceEndpoint, sourceInstance, nil
	})

	// copy the address when the manager answers
	if found {
		if err := updateThreeportConfigWithControlPlaneInstance(
			apiClient,
			apiEndpoint,
			instance,
			threeportConfig,
		); err == nil {
			return nil
		}
	}

	// a missing manager or a failed refresh leaves the saved entry in place
	cli.Warning(fmt.Sprintf("Could not refresh control plane %s. Using the saved entry.", name))
	return nil
}

// refreshSources lists the configured control planes that can hold the saved entry.
// The current control plane is first, then genesis, then the rest.
func refreshSources(threeportConfig *cli.ThreeportConfig, resolution controlPlaneResolution) []*cli.ControlPlane {
	if resolution.requested == nil {
		return nil
	}

	// skip the saved entry. Its registration lives on the control plane that created it.
	var sources []*cli.ControlPlane
	seen := map[string]bool{resolution.requested.Name: true}
	add := func(controlPlane *cli.ControlPlane) {
		if controlPlane == nil || controlPlane.Name == "" || controlPlane.APIServer == "" || seen[controlPlane.Name] {
			return
		}
		seen[controlPlane.Name] = true
		sources = append(sources, controlPlane)
	}
	add(resolution.current)
	add(resolution.genesis)
	for i := range threeportConfig.ControlPlanes {
		add(&threeportConfig.ControlPlanes[i])
	}
	return sources
}

// refreshInstanceFrom returns the instance from the first source that has it.
// Skip a source that is down or does not have the instance.
func refreshInstanceFrom(
	sources []*cli.ControlPlane,
	lookup func(*cli.ControlPlane) (*http.Client, string, *v0.ControlPlaneInstance, error),
) (*http.Client, string, *v0.ControlPlaneInstance, bool) {
	// take the first source that returns the instance
	for _, source := range sources {
		apiClient, apiEndpoint, instance, err := lookup(source)
		if err != nil {
			continue
		}
		return apiClient, apiEndpoint, instance, true
	}
	return nil, "", nil, false
}

// refreshControlPlaneClient copies the API address and client certificates onto an existing entry.
func refreshControlPlaneClient(existing, from *cli.ControlPlane) {
	// leave the kubeconfig and the provider config on the existing entry
	existing.APIServer = from.APIServer
	existing.AuthEnabled = from.AuthEnabled
	existing.CACert = from.CACert
	existing.Credentials = from.Credentials
}

// controlPlaneConfigFromInstance builds the threeport config entry for a control
// plane instance.  It returns an error, instead of dereferencing, when the API
// has left out a field the entry needs.
func controlPlaneConfigFromInstance(
	instance *v0.ControlPlaneInstance,
	definition *v0.ControlPlaneDefinition,
	runtimeDefinition *v0.KubernetesRuntimeDefinition,
) (*cli.ControlPlane, error) {
	var missing []string
	if instance.Name == nil {
		missing = append(missing, "control plane instance name")
	}
	if instance.ApiServerEndpoint == nil {
		missing = append(missing, "control plane instance API server endpoint")
	}
	if definition.AuthEnabled == nil {
		missing = append(missing, "control plane definition auth enabled")
	}
	if runtimeDefinition.InfraProvider == nil {
		missing = append(missing, "kubernetes runtime definition infra provider")
	}
	if definition.AuthEnabled != nil && *definition.AuthEnabled {
		if instance.CACert == nil {
			missing = append(missing, "control plane instance CA cert")
		}
		if instance.ClientCert == nil {
			missing = append(missing, "control plane instance client cert")
		}
		if instance.ClientKey == nil {
			missing = append(missing, "control plane instance client key")
		}
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("control plane cannot be added to the config, the API did not return: %s", strings.Join(missing, ", "))
	}

	controlPlaneConfig := &cli.ControlPlane{
		Name:        *instance.Name,
		APIServer:   *instance.ApiServerEndpoint,
		AuthEnabled: *definition.AuthEnabled,
		Provider:    *runtimeDefinition.InfraProvider,
	}
	if instance.Genesis != nil {
		controlPlaneConfig.Genesis = *instance.Genesis
	}

	// we construct the instance info for the threeport config
	if *definition.AuthEnabled {
		controlPlaneConfig.CACert = *instance.CACert
		controlPlaneConfig.Credentials = []cli.Credential{
			{
				Name:       *instance.Name,
				ClientCert: *instance.ClientCert,
				ClientKey:  *instance.ClientKey,
			},
		}
	}

	return controlPlaneConfig, nil
}

// sameSavedControlPlane reports whether the instance is the saved control plane.
// The CA stays when the API address changes, so a same name with a different CA is another control plane.
func sameSavedControlPlane(saved *cli.ControlPlane, instance *v0.ControlPlaneInstance) bool {
	if saved == nil || instance == nil {
		return false
	}

	// match the CA
	if saved.CACert != "" && saved.CACert == valueOrEmpty(instance.CACert) {
		return true
	}

	// match the client certificate when the CA is absent
	if cert := savedClientCert(saved); cert != "" && cert == valueOrEmpty(instance.ClientCert) {
		return true
	}

	// match the API address when the certificates are absent
	return saved.APIServer != "" && saved.APIServer == valueOrEmpty(instance.ApiServerEndpoint)
}

// savedClientCert returns the client certificate stored for this control plane.
func savedClientCert(saved *cli.ControlPlane) string {
	// use the credential that holds a client certificate
	for _, credential := range saved.Credentials {
		if credential.ClientCert != "" {
			return credential.ClientCert
		}
	}
	return ""
}

// valueOrEmpty returns the value a string pointer points at, or an empty
// string.
func valueOrEmpty(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func init() {
	ConfigCmd.AddCommand(ConfigCurrentControlPlaneCmd)

	ConfigCurrentControlPlaneCmd.Flags().StringVarP(
		&configCurrentControlPlaneName,
		"control-plane-name", "n", "", "The name of the Control plane to set as current.",
	)
	ConfigCurrentControlPlaneCmd.MarkFlagRequired("control-plane-name")
}
