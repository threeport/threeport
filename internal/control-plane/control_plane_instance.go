package controlplane

import (
	"bytes"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/go-logr/logr"
	builder_config "github.com/nukleros/aws-builder/pkg/config"
	"github.com/nukleros/aws-builder/pkg/iam"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/threeport/threeport/internal/provider"
	v0 "github.com/threeport/threeport/pkg/api/v0"
	auth "github.com/threeport/threeport/pkg/auth/v0"
	client "github.com/threeport/threeport/pkg/client/v0"
	config "github.com/threeport/threeport/pkg/config/v0"
	controller "github.com/threeport/threeport/pkg/controller/v0"
	"github.com/threeport/threeport/pkg/encryption/v0"
	kube "github.com/threeport/threeport/pkg/kube/v0"
	threeport "github.com/threeport/threeport/pkg/threeport-installer/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// controlPlaneInstanceCreated reconciles state for a new control plane
// Instance.
func controlPlaneInstanceCreated(
	r *controller.Reconciler,
	controlPlaneRuntimeInstance *v0.ControlPlaneInstance,
	log *logr.Logger,
) (int64, error) {

	var notFirstRun bool
	if controlPlaneRuntimeInstance.CreationAcknowledged == nil {
		notFirstRun = false
		// acknowledge the control plane instance is being created
		createdReconciliation := true
		creationTimestamp := time.Now().UTC()
		createdControlPlaneInstance := v0.ControlPlaneInstance{
			Common: v0.Common{
				ID: controlPlaneRuntimeInstance.ID,
			},
			Reconciliation: v0.Reconciliation{
				Reconciled:           &createdReconciliation,
				CreationAcknowledged: &creationTimestamp,
			},
		}
		_, err := client.UpdateControlPlaneInstance(
			r.APIClient,
			r.APIServer,
			&createdControlPlaneInstance,
		)
		if err != nil {
			return 0, fmt.Errorf("failed to confirm creation of control plane instance in threeport API: %w", err)
		}
	} else {
		notFirstRun = true
	}

	// ensure control plane definition is reconciled before working on an instance
	// for it
	controlPlaneDefinition, err := client.GetControlPlaneDefinitionByID(
		r.APIClient,
		r.APIServer,
		*controlPlaneRuntimeInstance.ControlPlaneDefinitionID,
	)
	if err != nil {
		return 0, fmt.Errorf("failed to get workload definition by workload definition ID: %w", err)
	}
	if controlPlaneDefinition.Reconciled != nil && !*controlPlaneDefinition.Reconciled {
		return 0, errors.New("controlplane definition not reconciled")
	}

	// get kubernetes runtime instance info
	kubernetesRuntimeInstance, err := client.GetKubernetesRuntimeInstanceByID(
		r.APIClient,
		r.APIServer,
		*controlPlaneRuntimeInstance.KubernetesRuntimeInstanceID,
	)
	if err != nil {
		return 0, fmt.Errorf("failed to get control plane kubernetesRuntime instance by ID: %w", err)
	}

	// ensure kubernetes runtime instance is reconciled before working with it
	if kubernetesRuntimeInstance.Reconciled != nil && !*kubernetesRuntimeInstance.Reconciled {
		return 0, errors.New("kubernetes runtime instance not reconciled")
	}

	// get kubernetes runtime definition info
	kubernetesRuntimeDefinition, err := client.GetKubernetesRuntimeDefinitionByID(
		r.APIClient,
		r.APIServer,
		*kubernetesRuntimeInstance.KubernetesRuntimeDefinitionID,
	)
	if err != nil {
		return 0, fmt.Errorf("failed to get control plane kubernetesRuntime definition by ID: %w", err)
	}

	// create a dynamic client to connect to kube API
	dynamicKubeClient, mapper, err := kube.GetClient(
		kubernetesRuntimeInstance,
		true,
		r.APIClient,
		r.APIServer,
		r.EncryptionKey,
	)
	if err != nil {
		return 0, fmt.Errorf("failed to create dynamic kube API client: %w", err)
	}

	selfInstance, err := client.GetSelfControlPlaneInstance(r.APIClient, r.APIServer)
	if err != nil {
		return 0, fmt.Errorf("failed to get self control plane instance: %w", err)
	}

	// Configure installer for new control plane instance
	cpi := threeport.NewInstaller(threeport.Namespace(*controlPlaneRuntimeInstance.Namespace))
	cpi.Opts.ControlPlaneName = *controlPlaneRuntimeInstance.Name
	cpi.Opts.InThreeport = true
	cpi.Opts.RestApiEksLoadBalancer = false
	// If this is not the first run of the creation reconciler, we want to use createOrUpdate mode
	cpi.Opts.CreateOrUpdateKubeResources = notFirstRun

	componentMap := make(map[string]*v0.ControlPlaneComponent, 0)
	componentMap["rest-api"] = cpi.Opts.RestApiInfo
	componentMap["agent"] = cpi.Opts.AgentInfo
	for _, info := range cpi.Opts.ControllerList {
		componentMap[info.Name] = info
	}

	for _, customInfo := range controlPlaneRuntimeInstance.CustomComponentInfo {
		installInfo, exists := componentMap[customInfo.Name]

		if !exists {
			cpi.Opts.ControllerList = append(cpi.Opts.ControllerList, customInfo)
			continue
		}

		if customInfo.ImageName != "" {
			installInfo.ImageName = customInfo.ImageName
		}

		if customInfo.ImageRepo != "" {
			installInfo.ImageRepo = customInfo.ImageRepo
		}

		if customInfo.ImageTag != "" {
			installInfo.ImageTag = customInfo.ImageTag
		}

		if customInfo.ServiceAccountName != "" {
			installInfo.ServiceAccountName = customInfo.ServiceAccountName
		}

		if customInfo.ServiceResourceName != "" {
			installInfo.ServiceResourceName = customInfo.ServiceResourceName
		}

		if customInfo.ImagePullSecretName != "" {
			installInfo.ImagePullSecretName = customInfo.ImagePullSecretName
		}

		if customInfo.Enabled != nil {
			installInfo.Enabled = customInfo.Enabled
		}
	}

	cpi.Opts.InfraProvider = *kubernetesRuntimeDefinition.InfraProvider

	// perform provider specific configuration
	var callerIdentity *sts.GetCallerIdentityOutput
	switch *kubernetesRuntimeDefinition.InfraProvider {
	case v0.KubernetesRuntimeInfraProviderEKS:
		// create AWS config
		awsConf, err := builder_config.LoadAWSConfig(false, "", "", "", "", "")
		if err != nil {
			return 0, fmt.Errorf("failed to load AWS configuration with local config: %w", err)
		}
		awsConfigResourceManager := *awsConf

		// get account ID
		if callerIdentity, err = provider.GetCallerIdentity(awsConf); err != nil {
			return 0, fmt.Errorf("failed to get caller identity: %w", err)
		}

		// create resource manager role
		resourceManagerRoleName := provider.GetResourceManagerRoleName(cpi.Opts.ControlPlaneName)
		_, err = provider.CreateResourceManagerRole(
			iam.CreateIamTags(
				cpi.Opts.ControlPlaneName,
				map[string]string{},
			),
			resourceManagerRoleName,
			*callerIdentity.Account,
			"",
			"",
			"",
			true,
			false, // don't attach internal resource manager policy
			awsConfigResourceManager,
		)
		if err != nil {
			return 0, fmt.Errorf("failed to create runtime manager role: %w", err)
		}

	}

	// Determine auth enabled and create config if needed
	var authConfig *auth.AuthConfig
	var newApiClient *http.Client
	if *controlPlaneDefinition.AuthEnabled {
		cpi.Opts.AuthEnabled = true
		// get auth config
		authConfig, err = auth.GetAuthConfig()
		if err != nil {
			return 0, fmt.Errorf("failed to get auth config: %w", err)
		}

		// generate client certificate
		clientCertificate, clientPrivateKey, err := auth.GenerateCertificate(
			authConfig.CAConfig,
			&authConfig.CAPrivateKey,
		)
		if err != nil {
			return 0, fmt.Errorf("failed to generate client certificate and private key: %w", err)
		}

		clientCredentials := &config.Credential{
			Name:       cpi.Opts.ControlPlaneName,
			ClientCert: util.Base64Encode(clientCertificate),
			ClientKey:  util.Base64Encode(clientPrivateKey),
		}

		// configure http client for calls to threeport API
		newApiClient, err = client.GetHTTPClient(true, authConfig.CAPemEncoded, clientCertificate, clientPrivateKey, "")
		if err != nil {
			return 0, fmt.Errorf("failed to create http client for new control plane: %w", err)
		}

		controlPlaneRuntimeInstance.CACert = &authConfig.CABase64Encoded
		controlPlaneRuntimeInstance.ClientCert = &clientCredentials.ClientCert
		controlPlaneRuntimeInstance.ClientKey = &clientCredentials.ClientKey
	} else {
		cpi.Opts.AuthEnabled = false
		newApiClient, err = client.GetHTTPClient(false, "", "", "", "")
		if err != nil {
			return 0, fmt.Errorf("failed to create http client for new control plane: %w", err)
		}
	}

	_, port := cpi.GetAPIServicePort()
	threeportAPIEndpoint := fmt.Sprintf("%s.%s:%d", cpi.Opts.RestApiInfo.ServiceResourceName, cpi.Opts.Namespace, port)
	controlPlaneRuntimeInstance.ApiServerEndpoint = &threeportAPIEndpoint

	// generate encryption key
	encryptionKey, err := encryption.GenerateKey()
	if err != nil {
		return 0, fmt.Errorf("failed to generate encryption key: %w", err)
	}

	// install the threeport control plane dependencies
	if err := cpi.InstallThreeportControlPlaneDependencies(
		dynamicKubeClient,
		mapper,
		*kubernetesRuntimeDefinition.InfraProvider,
		encryptionKey,
	); err != nil {
		return 0, fmt.Errorf("failed to install threeport control plane dependencies")
	}

	// install the API
	if err := cpi.UpdateThreeportAPIDeployment(
		dynamicKubeClient,
		mapper,
	); err != nil {
		return 0, fmt.Errorf("failed to install threeport API server: %w", err)
	}

	// for a cloud provider installed control plane:
	// * determine the threeport API's remote endpoint to add to the threeport
	//   config and to add to the server certificate's alt names when TLS
	//   assets are installed
	// * install provider-specific kubernetes resources
	switch *kubernetesRuntimeDefinition.InfraProvider {
	case v0.KubernetesRuntimeInfraProviderEKS:
		// create and configure service accounts for workload and aws controllers,
		// which will be used to authenticate to AWS via IRSA

		// configure IRSA controllers to use appropriate service account names
		provider.UpdateIrsaControllerList(cpi.Opts.ControllerList)

		// create IRSA service accounts
		for _, serviceAccount := range provider.GetIrsaServiceAccounts(
			cpi.Opts.Namespace,
			*callerIdentity.Account,
			provider.GetResourceManagerRoleName(cpi.Opts.ControlPlaneName),
		) {
			if err := cpi.CreateOrUpdateKubeResource(serviceAccount, dynamicKubeClient, mapper); err != nil {
				return 0, fmt.Errorf("failed to create threeport api service account: %w", err)
			}
		}
	}

	// if auth enabled install the threeport API TLS assets that include the alt
	// name for the remote load balancer if applicable
	if cpi.Opts.AuthEnabled {
		// install the threeport API TLS assets
		if err := cpi.InstallThreeportAPITLS(
			dynamicKubeClient,
			mapper,
			authConfig,
			threeportAPIEndpoint,
		); err != nil {
			return 0, fmt.Errorf("failed to install threeport API TLS assets: %w", err)
		}
	}

	// install the controllers
	if err := cpi.InstallThreeportControllers(
		dynamicKubeClient,
		mapper,
		authConfig,
	); err != nil {
		return 0, fmt.Errorf("failed to install threeport controllers: %w", err)
	}

	// install support services CRDs
	err = threeport.InstallThreeportCRDs(dynamicKubeClient, mapper)
	if err != nil {
		return 0, fmt.Errorf("failed to install threeport support services CRDs: %w", err)
	}

	// wait for kube API to persist the change and refresh the client and mapper
	// this is necessary to have the updated REST mapping for the CRDs as the
	// support services operator install includes one of those custom resources
	time.Sleep(time.Second * 10)
	dynamicKubeClient, mapper, err = kube.GetClient(
		kubernetesRuntimeInstance,
		true,
		r.APIClient,
		r.APIServer,
		r.EncryptionKey,
	)
	if err != nil {
		return 0, fmt.Errorf("failed to refresh dynamic kube API client: %w", err)
	}

	// install the support services operator
	err = threeport.InstallThreeportSupportServicesOperator(dynamicKubeClient, mapper)
	if err != nil {
		return 0, fmt.Errorf("failed to install threeport support services operator: %w", err)
	}

	fmt.Println("Waiting for threeport API to start running...")
	if err = util.Retry(30, 10, func() error {
		_, err := client.GetResponse(
			newApiClient,
			fmt.Sprintf("%s/version", threeportAPIEndpoint),
			http.MethodGet,
			new(bytes.Buffer),
			map[string]string{},
			http.StatusOK,
		)
		if err != nil {
			return fmt.Errorf("failed to get threeport API version: %w", err)
		}
		return nil
	}); err != nil {
		return 0, fmt.Errorf("threeport API did not come up: %w", err)
	}
	fmt.Println("Threeport API is running")

	// update the newly created instance with parent
	controlPlaneRuntimeInstance.ParentControlPlaneInstanceID = selfInstance.ID

	_, err = client.UpdateControlPlaneInstance(
		r.APIClient,
		r.APIServer,
		controlPlaneRuntimeInstance,
	)
	if err != nil {
		return 0, fmt.Errorf("failed to update control plane runtime instance with parent: %w", err)
	}

	if !*controlPlaneDefinition.OnboardParent {
		return 0, nil
	}

	cloneKubernetesRuntimeDefinition := *kubernetesRuntimeDefinition
	cloneKubernetesRuntimeDefinition.Common = v0.Common{}
	createdKubeDef, err := client.CreateKubernetesRuntimeDefinition(
		newApiClient,
		threeportAPIEndpoint,
		&cloneKubernetesRuntimeDefinition,
	)

	if err != nil {
		return 0, fmt.Errorf("failed to create kubernetes runtime definition on new threeport api: %w", err)
	}

	clonedKubernetesRuntimeInstance := *kubernetesRuntimeInstance
	clonedKubernetesRuntimeInstance.Common = v0.Common{}
	if clonedKubernetesRuntimeInstance.CertificateKey != nil && *clonedKubernetesRuntimeInstance.CertificateKey != "" {
		decryptedKey, err := encryption.Decrypt(r.EncryptionKey, *clonedKubernetesRuntimeInstance.CertificateKey)
		if err != nil {
			return 0, fmt.Errorf("failed to decrypt kubernetes runtime instance certificate key: %w", err)
		}

		clonedKubernetesRuntimeInstance.CertificateKey = &decryptedKey
	}

	if clonedKubernetesRuntimeInstance.ConnectionToken != nil && *clonedKubernetesRuntimeInstance.ConnectionToken != "" {
		decryptedToken, err := encryption.Decrypt(r.EncryptionKey, *clonedKubernetesRuntimeInstance.ConnectionToken)
		if err != nil {
			return 0, fmt.Errorf("failed to decrypt kubernetes runtime instance connection token: %w", err)
		}

		clonedKubernetesRuntimeInstance.ConnectionToken = &decryptedToken
	}

	clonedKubernetesRuntimeInstance.KubernetesRuntimeDefinitionID = createdKubeDef.ID
	createdKubeInstance, err := client.CreateKubernetesRuntimeInstance(
		newApiClient,
		threeportAPIEndpoint,
		&clonedKubernetesRuntimeInstance,
	)

	if err != nil {
		return 0, fmt.Errorf("failed to update threeport api with kubernetes runtime instance: %w", err)
	}

	switch *kubernetesRuntimeDefinition.InfraProvider {
	case v0.KubernetesRuntimeInfraProviderEKS:
		// Get associated AWS object and ensure they are in db
		awsRuntimeDef, err := client.GetAwsEksKubernetesRuntimeDefinitionByK8sRuntimeDef(r.APIClient, r.APIServer, *kubernetesRuntimeDefinition.ID)
		if err != nil {
			return 0, fmt.Errorf("failed to get AwsEksKubernetesRuntimeDefinition: %w", err)
		}

		awsRuntimeInstance, err := client.GetAwsEksKubernetesRuntimeInstanceByK8sRuntimeInst(r.APIClient, r.APIServer, *kubernetesRuntimeInstance.ID)
		if err != nil {
			return 0, fmt.Errorf("failed to get AwsEksKubernetesRuntimeInstance: %w", err)
		}

		awsAccount, err := client.GetAwsAccountByAccountID(r.APIClient, r.APIServer, fmt.Sprint(awsRuntimeDef.AwsAccountID))
		if err != nil {
			return 0, fmt.Errorf("failed to get AwsAccount: %w", err)
		}

		awsAccount.Common = v0.Common{}
		if awsAccount.AccessKeyID != nil && *awsAccount.AccessKeyID != "" {
			decryptedKey, err := encryption.Decrypt(r.EncryptionKey, *awsAccount.AccessKeyID)
			if err != nil {
				return 0, fmt.Errorf("failed to decrypt access key id on aws account: %w", err)
			}

			awsAccount.AccessKeyID = &decryptedKey
		}

		if awsAccount.SecretAccessKey != nil && *awsAccount.SecretAccessKey != "" {
			decryptedKey, err := encryption.Decrypt(r.EncryptionKey, *awsAccount.SecretAccessKey)
			if err != nil {
				return 0, fmt.Errorf("failed to decrypt secret access key on aws account: %w", err)
			}

			awsAccount.SecretAccessKey = &decryptedKey
		}

		createdAwsAccount, err := client.CreateAwsAccount(newApiClient, threeportAPIEndpoint, awsAccount)
		if err != nil {
			return 0, fmt.Errorf("failed to create AwsAccount: %w", err)
		}

		awsRuntimeDef.Common = v0.Common{}
		awsRuntimeDef.AwsAccountID = createdAwsAccount.ID
		createdAwsRuntimDef, err := client.CreateAwsEksKubernetesRuntimeDefinition(newApiClient, threeportAPIEndpoint, awsRuntimeDef)
		if err != nil {
			return 0, fmt.Errorf("failed to create AwsEksKubernetesRuntimeDefinition: %w", err)
		}

		awsRuntimeInstance.Common = v0.Common{}
		awsRuntimeInstance.AwsEksKubernetesRuntimeDefinitionID = createdAwsRuntimDef.ID
		_, err = client.CreateAwsEksKubernetesRuntimeInstance(newApiClient, threeportAPIEndpoint, awsRuntimeInstance)
		if err != nil {
			return 0, fmt.Errorf("failed to create AwsEksKubernetesRuntimeInstance: %w", err)
		}

	}

	// onboard control plane definition and instance to new control plane
	controlPlaneDefinition.Common = v0.Common{}

	createdControlPlaneDef, err := client.CreateControlPlaneDefinition(
		newApiClient,
		threeportAPIEndpoint,
		controlPlaneDefinition,
	)
	if err != nil {
		return 0, fmt.Errorf("failed to create control plane definition on new threeport api: %w", err)
	}

	reconciled := true
	self := true
	clonedRuntimeInstance := *controlPlaneRuntimeInstance
	clonedRuntimeInstance.Common = v0.Common{}
	clonedRuntimeInstance.ParentControlPlaneInstanceID = nil
	clonedRuntimeInstance.ControlPlaneDefinitionID = createdControlPlaneDef.ID
	clonedRuntimeInstance.IsSelf = &self
	clonedRuntimeInstance.KubernetesRuntimeInstanceID = createdKubeInstance.ID
	clonedRuntimeInstance.Reconciled = &reconciled

	_, err = client.CreateControlPlaneInstance(
		newApiClient,
		threeportAPIEndpoint,
		&clonedRuntimeInstance,
	)
	if err != nil {
		return 0, fmt.Errorf("failed to create new control plane instance on new threeport api: %w", err)
	}

	// Ensure the new control plane has genesis control plane registered
	genesisInstance, err := client.GetGenesisControlPlaneInstance(r.APIClient, r.APIServer)
	if err != nil {
		return 0, fmt.Errorf("failed to get genesis control plane instance: %w", err)
	}

	genesisDef, err := client.GetControlPlaneDefinitionByID(r.APIClient, r.APIServer, *genesisInstance.ControlPlaneDefinitionID)
	if err != nil {
		return 0, fmt.Errorf("failed to get genesis control plane definition: %w", err)
	}

	var genDefID uint
	if *genesisDef.ID != *controlPlaneRuntimeInstance.ControlPlaneDefinitionID {
		genesisDef.Common = v0.Common{}
		createdDef, err := client.CreateControlPlaneDefinition(newApiClient, threeportAPIEndpoint, genesisDef)
		if err != nil {
			return 0, fmt.Errorf("failed to create genesis control plane definition in new threeport api: %w", err)
		}
		genDefID = *createdDef.ID
	} else {
		genDefID = *createdControlPlaneDef.ID
	}

	var genKubeRuntimeInstanceID uint
	if *genesisInstance.KubernetesRuntimeInstanceID != *controlPlaneRuntimeInstance.KubernetesRuntimeInstanceID {
		genesisK8Instance, err := client.GetKubernetesRuntimeInstanceByID(r.APIClient, r.APIServer, *genesisInstance.KubernetesRuntimeInstanceID)
		if err != nil {
			return 0, fmt.Errorf("failed to get genesis control plane kubernetes runtime instance: %w", err)
		}

		var genesisK8DefID uint
		if *genesisK8Instance.KubernetesRuntimeDefinitionID != *kubernetesRuntimeInstance.KubernetesRuntimeDefinitionID {
			genesisK8Def, err := client.GetKubernetesRuntimeDefinitionByID(r.APIClient, r.APIServer, *genesisK8Instance.KubernetesRuntimeDefinitionID)
			if err != nil {
				return 0, fmt.Errorf("failed to get genesis control plane kubernetes runtime definition: %w", err)
			}

			genesisK8Def.Common = v0.Common{}
			newK8Def, err := client.CreateKubernetesRuntimeDefinition(newApiClient, threeportAPIEndpoint, genesisK8Def)
			if err != nil {
				return 0, fmt.Errorf("failed to create genesis control plane kubernetes runtime def: %w", err)
			}

			genesisK8DefID = *newK8Def.ID
		} else {
			genesisK8DefID = *createdKubeDef.ID
		}

		genesisK8Instance.Common = v0.Common{}
		genesisK8Instance.KubernetesRuntimeDefinitionID = &genesisK8DefID

		newK8Instance, err := client.CreateKubernetesRuntimeInstance(newApiClient, threeportAPIEndpoint, genesisK8Instance)
		if err != nil {
			return 0, fmt.Errorf("failed to create genesis kubernetes runtime instance: %w", err)
		}

		genKubeRuntimeInstanceID = *newK8Instance.ID
	} else {
		genKubeRuntimeInstanceID = *createdKubeInstance.ID
	}

	self = false
	genesisInstance.Common = v0.Common{}
	genesisInstance.IsSelf = &self
	genesisInstance.ControlPlaneDefinitionID = &genDefID
	genesisInstance.KubernetesRuntimeInstanceID = &genKubeRuntimeInstanceID
	_, err = client.CreateControlPlaneInstance(newApiClient, threeportAPIEndpoint, genesisInstance)
	if err != nil {
		return 0, fmt.Errorf("failed to create genesis control plane instance in new threeport api: %w", err)
	}

	return 0, nil
}

// controlPlaneInstanceUpdated reconciles state for a updated control plane
// Instance.
func controlPlaneInstanceUpdated(
	r *controller.Reconciler,
	controlPlaneRuntimeInstance *v0.ControlPlaneInstance,
	log *logr.Logger,
) (int64, error) {
	return 0, nil
}

// controlPlaneInstanceDeleted reconciles state for a deleted control plane
// Instance.
func controlPlaneInstanceDeleted(
	r *controller.Reconciler,
	controlPlaneRuntimeInstance *v0.ControlPlaneInstance,
	log *logr.Logger,
) (int64, error) {

	// get kubernetes runtime instance info
	kubernetesRuntimeInstance, err := client.GetKubernetesRuntimeInstanceByID(
		r.APIClient,
		r.APIServer,
		*controlPlaneRuntimeInstance.KubernetesRuntimeInstanceID,
	)
	if err != nil {
		return 0, fmt.Errorf("failed to get control plane kubernetesRuntime instance by ID: %w", err)
	}

	// get kubernetes runtime definition info
	kubernetesRuntimeDefinition, err := client.GetKubernetesRuntimeDefinitionByID(
		r.APIClient,
		r.APIServer,
		*kubernetesRuntimeInstance.KubernetesRuntimeDefinitionID,
	)
	if err != nil {
		return 0, fmt.Errorf("failed to get control plane kubernetesRuntime definition by ID: %w", err)
	}

	// create a dynamic client to connect to kube API
	dynamicKubeClient, mapper, err := kube.GetClient(
		kubernetesRuntimeInstance,
		true,
		r.APIClient,
		r.APIServer,
		r.EncryptionKey,
	)
	if err != nil {
		return 0, fmt.Errorf("failed to create dynamic kube API client: %w", err)
	}

	var namespace = &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Namespace",
			"metadata": map[string]interface{}{
				"name": *controlPlaneRuntimeInstance.Namespace,
			},
		},
	}

	// delete the namespace
	if err := kube.DeleteResource(namespace, dynamicKubeClient, *mapper); err != nil {
		return 0, fmt.Errorf("failed to delete the control plane namespace: %w", err)
	}

	// perform provider specific configuration
	switch *kubernetesRuntimeDefinition.InfraProvider {
	case v0.KubernetesRuntimeInfraProviderEKS:
		// create AWS config
		awsConf, err := builder_config.LoadAWSConfig(false, "", "", "", "", "")
		if err != nil {
			return 0, fmt.Errorf("failed to load AWS configuration with local config: %w", err)
		}
		awsConfigResourceManager := *awsConf

		// get account ID
		if _, err = provider.GetCallerIdentity(awsConf); err != nil {
			return 0, fmt.Errorf("failed to get caller identity: %w", err)
		}

		// delete resource manager role
		err = provider.DeleteResourceManagerRole(*controlPlaneRuntimeInstance.Name, awsConfigResourceManager)
		if err != nil {
			return 0, fmt.Errorf("failed to delete threeport AWS IAM resources: %w", err)
		}
	}

	return 0, nil
}
