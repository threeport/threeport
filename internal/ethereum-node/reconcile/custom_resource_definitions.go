/*
Copyright 2023.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package reconcile

import (
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	// "sigs.k8s.io/controller-runtime/pkg/client"

	// "github.com/nukleros/operator-builder-tools/pkg/controller/workload"

	// appsv1alpha1 "github.com/randalljohnson/operator-builder-kotal/apis/apps/v1alpha1"
	// "github.com/randalljohnson/operator-builder-kotal/apis/apps/v1alpha1/standaloneworkloadconfig/mutate"
)

// +kubebuilder:rbac:groups=apiextensions.k8s.io,resources=customresourcedefinitions,verbs=get;list;watch;create;update;patch;delete

// CreateCRDBeaconnodesEthereum2KotalIo creates the CustomResourceDefinition resource with name beaconnodes.ethereum2.kotal.io.
func CreateCRDBeaconnodesEthereum2KotalIo() (*unstructured.Unstructured) {

	var resourceObj = &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "apiextensions.k8s.io/v1",
			"kind":       "CustomResourceDefinition",
			"metadata": map[string]interface{}{
				"annotations": map[string]interface{}{
					"cert-manager.io/inject-ca-from":        "$(CERTIFICATE_NAMESPACE)/$(CERTIFICATE_NAME)",
					"controller-gen.kubebuilder.io/version": "v0.8.0",
				},
				"creationTimestamp": nil,
				"name":              "beaconnodes.ethereum2.kotal.io",
			},
			"spec": map[string]interface{}{
				"group": "ethereum2.kotal.io",
				"names": map[string]interface{}{
					"kind":     "BeaconNode",
					"listKind": "BeaconNodeList",
					"plural":   "beaconnodes",
					"singular": "beaconnode",
				},
				"scope": "Namespaced",
				"versions": []interface{}{
					map[string]interface{}{
						"additionalPrinterColumns": []interface{}{
							map[string]interface{}{
								"jsonPath": ".spec.client",
								"name":     "Client",
								"type":     "string",
							},
							map[string]interface{}{
								"jsonPath": ".spec.network",
								"name":     "Network",
								"type":     "string",
							},
						},
						"name": "v1alpha1",
						"schema": map[string]interface{}{
							"openAPIV3Schema": map[string]interface{}{
								"description": "BeaconNode is the Schema for the beaconnodes API",
								"properties": map[string]interface{}{
									"apiVersion": map[string]interface{}{
										"description": "APIVersion defines the versioned schema of this representation of an object. Servers should convert recognized schemas to the latest internal value, and may reject unrecognized values. More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#resources",
										"type":        "string",
									},
									"kind": map[string]interface{}{
										"description": "Kind is a string value representing the REST resource this object represents. Servers may infer this from the endpoint the client submits requests to. Cannot be updated. In CamelCase. More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#types-kinds",
										"type":        "string",
									},
									"metadata": map[string]interface{}{
										"type": "object",
									},
									"spec": map[string]interface{}{
										"description": "BeaconNodeSpec defines the desired state of BeaconNode",
										"properties": map[string]interface{}{
											"certSecretName": map[string]interface{}{
												"description": "CertSecretName is k8s secret name that holds tls.key and tls.cert",
												"type":        "string",
											},
											"checkpointSyncUrl": map[string]interface{}{
												"description": "CheckpointSyncURL is trusted beacon node rest api endpoint",
												"type":        "string",
											},
											"client": map[string]interface{}{
												"description": "Client is the Ethereum 2.0 client to use",
												"enum": []interface{}{
													"teku",
													"prysm",
													"lighthouse",
													"nimbus",
												},
												"type": "string",
											},
											"corsDomains": map[string]interface{}{
												"description": "CORSDomains is the domains from which to accept cross origin requests",
												"items": map[string]interface{}{
													"type": "string",
												},
												"type":                   "array",
												"x-kubernetes-list-type": "set",
											},
											"executionEngineEndpoint": map[string]interface{}{
												"description": "ExecutionEngineEndpoint is Ethereum Execution engine node endpoint",
												"type":        "string",
											},
											"feeRecipient": map[string]interface{}{
												"description": "FeeRecipient is ethereum address collecting transaction fees",
												"pattern":     "^0[xX][0-9a-fA-F]{40}$",
												"type":        "string",
											},
											"grpc": map[string]interface{}{
												"description": "GRPC enables GRPC gateway server",
												"type":        "boolean",
											},
											"grpcPort": map[string]interface{}{
												"description": "GRPCPort is GRPC gateway server port",
												"type":        "integer",
											},
											"hosts": map[string]interface{}{
												"description": "Hosts is a list of hostnames to to whitelist for API access",
												"items": map[string]interface{}{
													"type": "string",
												},
												"type":                   "array",
												"x-kubernetes-list-type": "set",
											},
											"image": map[string]interface{}{
												"description": "Image is Ethereum 2.0 Beacon node client image",
												"type":        "string",
											},
											"jwtSecretName": map[string]interface{}{
												"description": "JWTSecretName is kubernetes secret name holding JWT secret",
												"type":        "string",
											},
											"logging": map[string]interface{}{
												"description": "Logging is logging verboisty level",
												"enum": []interface{}{
													"off",
													"fatal",
													"error",
													"warn",
													"info",
													"debug",
													"trace",
													"all",
													"notice",
													"crit",
													"panic",
													"none",
												},
												"type": "string",
											},
											"network": map[string]interface{}{
												"description": "Network is the network to join",
												"type":        "string",
											},
											"p2pPort": map[string]interface{}{
												"description": "P2PPort is p2p and discovery port",
												"type":        "integer",
											},
											"resources": map[string]interface{}{
												"description": "Resources is node compute and storage resources",
												"properties": map[string]interface{}{
													"cpu": map[string]interface{}{
														"description": "CPU is cpu cores the node requires",
														"pattern":     "^[1-9][0-9]*m?$",
														"type":        "string",
													},
													"cpuLimit": map[string]interface{}{
														"description": "CPULimit is cpu cores the node is limited to",
														"pattern":     "^[1-9][0-9]*m?$",
														"type":        "string",
													},
													"memory": map[string]interface{}{
														"description": "Memory is memmory requirements",
														"pattern":     "^[1-9][0-9]*[KMGTPE]i$",
														"type":        "string",
													},
													"memoryLimit": map[string]interface{}{
														"description": "MemoryLimit is cpu cores the node is limited to",
														"pattern":     "^[1-9][0-9]*[KMGTPE]i$",
														"type":        "string",
													},
													"storage": map[string]interface{}{
														"description": "Storage is disk space storage requirements",
														"pattern":     "^[1-9][0-9]*[KMGTPE]i$",
														"type":        "string",
													},
													"storageClass": map[string]interface{}{
														"description": "StorageClass is the volume storage class",
														"type":        "string",
													},
												},
												"type": "object",
											},
											"rest": map[string]interface{}{
												"description": "REST enables Beacon REST API",
												"type":        "boolean",
											},
											"restPort": map[string]interface{}{
												"description": "RESTPort is Beacon REST API server port",
												"type":        "integer",
											},
											"rpc": map[string]interface{}{
												"description": "RPC enables RPC server",
												"type":        "boolean",
											},
											"rpcPort": map[string]interface{}{
												"description": "RPCPort is RPC server port",
												"type":        "integer",
											},
										},
										"required": []interface{}{
											"client",
											"executionEngineEndpoint",
											"jwtSecretName",
											"network",
										},
										"type": "object",
									},
									"status": map[string]interface{}{
										"description": "BeaconNodeStatus defines the observed state of BeaconNode",
										"type":        "object",
									},
								},
								"type": "object",
							},
						},
						"served":       true,
						"storage":      true,
						"subresources": map[string]interface{}{},
					},
				},
			},
			"status": map[string]interface{}{
				"acceptedNames": map[string]interface{}{
					"kind":   "",
					"plural": "",
				},
				"conditions":     []interface{}{},
				"storedVersions": []interface{}{},
			},
		},
	}

	return resourceObj

}


// CreateCRDNodesEthereumKotalIo creates the CustomResourceDefinition resource with name nodes.ethereum.kotal.io.
func CreateCRDNodesEthereumKotalIo() (*unstructured.Unstructured) {

	var resourceObj = &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "apiextensions.k8s.io/v1",
			"kind":       "CustomResourceDefinition",
			"metadata": map[string]interface{}{
				"annotations": map[string]interface{}{
					"cert-manager.io/inject-ca-from":        "$(CERTIFICATE_NAMESPACE)/$(CERTIFICATE_NAME)",
					"controller-gen.kubebuilder.io/version": "v0.8.0",
				},
				"creationTimestamp": nil,
				"name":              "nodes.ethereum.kotal.io",
			},
			"spec": map[string]interface{}{
				"group": "ethereum.kotal.io",
				"names": map[string]interface{}{
					"kind":     "Node",
					"listKind": "NodeList",
					"plural":   "nodes",
					"singular": "node",
				},
				"scope": "Namespaced",
				"versions": []interface{}{
					map[string]interface{}{
						"additionalPrinterColumns": []interface{}{
							map[string]interface{}{
								"jsonPath": ".spec.client",
								"name":     "Client",
								"type":     "string",
							},
							map[string]interface{}{
								"jsonPath": ".status.consensus",
								"name":     "Consensus",
								"type":     "string",
							},
							map[string]interface{}{
								"jsonPath": ".status.network",
								"name":     "Network",
								"type":     "string",
							},
							map[string]interface{}{
								"jsonPath": ".status.enodeURL",
								"name":     "enodeURL",
								"priority": 10,
								"type":     "string",
							},
						},
						"name": "v1alpha1",
						"schema": map[string]interface{}{
							"openAPIV3Schema": map[string]interface{}{
								"description": "Node is the Schema for the nodes API",
								"properties": map[string]interface{}{
									"apiVersion": map[string]interface{}{
										"description": "APIVersion defines the versioned schema of this representation of an object. Servers should convert recognized schemas to the latest internal value, and may reject unrecognized values. More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#resources",
										"type":        "string",
									},
									"kind": map[string]interface{}{
										"description": "Kind is a string value representing the REST resource this object represents. Servers may infer this from the endpoint the client submits requests to. Cannot be updated. In CamelCase. More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#types-kinds",
										"type":        "string",
									},
									"metadata": map[string]interface{}{
										"type": "object",
									},
									"spec": map[string]interface{}{
										"description": "NodeSpec is the specification of the node",
										"properties": map[string]interface{}{
											"bootnodes": map[string]interface{}{
												"description": "Bootnodes is set of ethereum node URLS for p2p discovery bootstrap",
												"items": map[string]interface{}{
													"description": "Enode is ethereum node url",
													"type":        "string",
												},
												"type":                   "array",
												"x-kubernetes-list-type": "set",
											},
											"client": map[string]interface{}{
												"description": "Client is ethereum client running on the node",
												"enum": []interface{}{
													"besu",
													"geth",
													"nethermind",
												},
												"type": "string",
											},
											"coinbase": map[string]interface{}{
												"description": "Coinbase is the account to which mining rewards are paid",
												"pattern":     "^0[xX][0-9a-fA-F]{40}$",
												"type":        "string",
											},
											"corsDomains": map[string]interface{}{
												"description": "CORSDomains is the domains from which to accept cross origin requests",
												"items": map[string]interface{}{
													"type": "string",
												},
												"type":                   "array",
												"x-kubernetes-list-type": "set",
											},
											"engine": map[string]interface{}{
												"description": "Engine enables authenticated Engine RPC APIs",
												"type":        "boolean",
											},
											"enginePort": map[string]interface{}{
												"description": "EnginePort is engine authenticated RPC APIs port",
												"type":        "integer",
											},
											"genesis": map[string]interface{}{
												"description": "Genesis is genesis block configuration",
												"properties": map[string]interface{}{
													"accounts": map[string]interface{}{
														"description": "Accounts is array of accounts to fund or associate with code and storage",
														"items": map[string]interface{}{
															"description": "Account is Ethereum account",
															"properties": map[string]interface{}{
																"address": map[string]interface{}{
																	"description": "Address is account address",
																	"pattern":     "^0[xX][0-9a-fA-F]{40}$",
																	"type":        "string",
																},
																"balance": map[string]interface{}{
																	"description": "Balance is account balance in wei",
																	"pattern":     "^0[xX][0-9a-fA-F]+$",
																	"type":        "string",
																},
																"code": map[string]interface{}{
																	"description": "Code is account contract byte code",
																	"pattern":     "^0[xX][0-9a-fA-F]+$",
																	"type":        "string",
																},
																"storage": map[string]interface{}{
																	"additionalProperties": map[string]interface{}{
																		"description": "HexString is String in hexadecial format",
																		"pattern":     "^0[xX][0-9a-fA-F]+$",
																		"type":        "string",
																	},
																	"description": "Storage is account contract storage as key value pair",
																	"type":        "object",
																},
															},
															"required": []interface{}{
																"address",
															},
															"type": "object",
														},
														"type": "array",
													},
													"chainId": map[string]interface{}{
														"description": "ChainID is the the chain ID used in transaction signature to prevent reply attack more details https://github.com/ethereum/EIPs/blob/master/EIPS/eip-155.md",
														"type":        "integer",
													},
													"clique": map[string]interface{}{
														"description": "Clique PoA engine cinfiguration",
														"properties": map[string]interface{}{
															"blockPeriod": map[string]interface{}{
																"description": "BlockPeriod is block time in seconds",
																"type":        "integer",
															},
															"epochLength": map[string]interface{}{
																"description": "EpochLength is the Number of blocks after which to reset all votes",
																"type":        "integer",
															},
															"signers": map[string]interface{}{
																"description": "Signers are PoA initial signers, at least one signer is required",
																"items": map[string]interface{}{
																	"description": "EthereumAddress is ethereum address",
																	"pattern":     "^0[xX][0-9a-fA-F]{40}$",
																	"type":        "string",
																},
																"minItems": 1,
																"type":     "array",
															},
														},
														"type": "object",
													},
													"coinbase": map[string]interface{}{
														"description": "Address to pay mining rewards to",
														"pattern":     "^0[xX][0-9a-fA-F]{40}$",
														"type":        "string",
													},
													"difficulty": map[string]interface{}{
														"description": "Difficulty is the diffculty of the genesis block",
														"pattern":     "^0[xX][0-9a-fA-F]+$",
														"type":        "string",
													},
													"ethash": map[string]interface{}{
														"description": "Ethash PoW engine configuration",
														"properties": map[string]interface{}{
															"fixedDifficulty": map[string]interface{}{
																"description": "FixedDifficulty is fixed difficulty to be used in private PoW networks",
																"type":        "integer",
															},
														},
														"type": "object",
													},
													"forks": map[string]interface{}{
														"description": "Forks is supported forks (network upgrade) and corresponding block number",
														"properties": map[string]interface{}{
															"arrowGlacier": map[string]interface{}{
																"description": "ArrowGlacier fork",
																"type":        "integer",
															},
															"berlin": map[string]interface{}{
																"description": "Berlin fork",
																"type":        "integer",
															},
															"byzantium": map[string]interface{}{
																"description": "Byzantium fork",
																"type":        "integer",
															},
															"constantinople": map[string]interface{}{
																"description": "Constantinople fork",
																"type":        "integer",
															},
															"dao": map[string]interface{}{
																"description": "DAO fork",
																"type":        "integer",
															},
															"eip150": map[string]interface{}{
																"description": "EIP150 (Tangerine Whistle) fork",
																"type":        "integer",
															},
															"eip155": map[string]interface{}{
																"description": "EIP155 (Spurious Dragon) fork",
																"type":        "integer",
															},
															"eip158": map[string]interface{}{
																"description": "EIP158 (state trie clearing) fork",
																"type":        "integer",
															},
															"homestead": map[string]interface{}{
																"description": "Homestead fork",
																"type":        "integer",
															},
															"istanbul": map[string]interface{}{
																"description": "Istanbul fork",
																"type":        "integer",
															},
															"london": map[string]interface{}{
																"description": "London fork",
																"type":        "integer",
															},
															"muirglacier": map[string]interface{}{
																"description": "MuirGlacier fork",
																"type":        "integer",
															},
															"petersburg": map[string]interface{}{
																"description": "Petersburg fork",
																"type":        "integer",
															},
														},
														"type": "object",
													},
													"gasLimit": map[string]interface{}{
														"description": "GastLimit is the total gas limit for all transactions in a block",
														"pattern":     "^0[xX][0-9a-fA-F]+$",
														"type":        "string",
													},
													"ibft2": map[string]interface{}{
														"description": "IBFT2 PoA engine configuration",
														"properties": map[string]interface{}{
															"blockPeriod": map[string]interface{}{
																"description": "BlockPeriod is block time in seconds",
																"type":        "integer",
															},
															"duplicateMessageLimit": map[string]interface{}{
																"description": "DuplicateMessageLimit is duplicate messages limit",
																"type":        "integer",
															},
															"epochLength": map[string]interface{}{
																"description": "EpochLength is the Number of blocks after which to reset all votes",
																"type":        "integer",
															},
															"futureMessagesLimit": map[string]interface{}{
																"description": "futureMessagesLimit is future messages buffer limit",
																"type":        "integer",
															},
															"futureMessagesMaxDistance": map[string]interface{}{
																"description": "FutureMessagesMaxDistance is maximum height from current chain height for buffering future messages",
																"type":        "integer",
															},
															"messageQueueLimit": map[string]interface{}{
																"description": "MessageQueueLimit is the message queue limit",
																"type":        "integer",
															},
															"requestTimeout": map[string]interface{}{
																"description": "RequestTimeout is the timeout for each consensus round in seconds",
																"type":        "integer",
															},
															"validators": map[string]interface{}{
																"description": "Validators are initial ibft2 validators",
																"items": map[string]interface{}{
																	"description": "EthereumAddress is ethereum address",
																	"pattern":     "^0[xX][0-9a-fA-F]{40}$",
																	"type":        "string",
																},
																"minItems": 1,
																"type":     "array",
															},
														},
														"type": "object",
													},
													"mixHash": map[string]interface{}{
														"description": "MixHash is hash combined with nonce to prove effort spent to create block",
														"pattern":     "^0[xX][0-9a-fA-F]{64}$",
														"type":        "string",
													},
													"networkId": map[string]interface{}{
														"description": "NetworkID is network id",
														"type":        "integer",
													},
													"nonce": map[string]interface{}{
														"description": "Nonce is random number used in block computation",
														"pattern":     "^0[xX][0-9a-fA-F]+$",
														"type":        "string",
													},
													"timestamp": map[string]interface{}{
														"description": "Timestamp is block creation date",
														"pattern":     "^0[xX][0-9a-fA-F]+$",
														"type":        "string",
													},
												},
												"required": []interface{}{
													"chainId",
													"networkId",
												},
												"type": "object",
											},
											"graphql": map[string]interface{}{
												"description": "GraphQL is whether GraphQL server is enabled or not",
												"type":        "boolean",
											},
											"graphqlPort": map[string]interface{}{
												"description": "GraphQLPort is the GraphQL server listening port",
												"type":        "integer",
											},
											"hosts": map[string]interface{}{
												"description": "Hosts is a list of hostnames to to whitelist for RPC access",
												"items": map[string]interface{}{
													"type": "string",
												},
												"type":                   "array",
												"x-kubernetes-list-type": "set",
											},
											"image": map[string]interface{}{
												"description": "Image is Ethereum node client image",
												"type":        "string",
											},
											"import": map[string]interface{}{
												"description": "import is account to import",
												"properties": map[string]interface{}{
													"passwordSecretName": map[string]interface{}{
														"description": "PasswordSecretName is the secret holding password used to encrypt account private key",
														"type":        "string",
													},
													"privateKeySecretName": map[string]interface{}{
														"description": "PrivateKeySecretName is the secret name holding account private key",
														"type":        "string",
													},
												},
												"required": []interface{}{
													"passwordSecretName",
													"privateKeySecretName",
												},
												"type": "object",
											},
											"jwtSecretName": map[string]interface{}{
												"description": "JWTSecretName is kubernetes secret name holding JWT secret",
												"type":        "string",
											},
											"logging": map[string]interface{}{
												"description": "Logging is logging verboisty level",
												"enum": []interface{}{
													"off",
													"fatal",
													"error",
													"warn",
													"info",
													"debug",
													"trace",
													"all",
												},
												"type": "string",
											},
											"miner": map[string]interface{}{
												"description": "Miner is whether node is mining/validating blocks or no",
												"type":        "boolean",
											},
											"network": map[string]interface{}{
												"description": "Network specifies the network to join",
												"type":        "string",
											},
											"nodePrivateKeySecretName": map[string]interface{}{
												"description": "NodePrivateKeySecretName is the secret name holding node private key",
												"type":        "string",
											},
											"p2pPort": map[string]interface{}{
												"description": "P2PPort is port used for peer to peer communication",
												"type":        "integer",
											},
											"resources": map[string]interface{}{
												"description": "Resources is node compute and storage resources",
												"properties": map[string]interface{}{
													"cpu": map[string]interface{}{
														"description": "CPU is cpu cores the node requires",
														"pattern":     "^[1-9][0-9]*m?$",
														"type":        "string",
													},
													"cpuLimit": map[string]interface{}{
														"description": "CPULimit is cpu cores the node is limited to",
														"pattern":     "^[1-9][0-9]*m?$",
														"type":        "string",
													},
													"memory": map[string]interface{}{
														"description": "Memory is memmory requirements",
														"pattern":     "^[1-9][0-9]*[KMGTPE]i$",
														"type":        "string",
													},
													"memoryLimit": map[string]interface{}{
														"description": "MemoryLimit is cpu cores the node is limited to",
														"pattern":     "^[1-9][0-9]*[KMGTPE]i$",
														"type":        "string",
													},
													"storage": map[string]interface{}{
														"description": "Storage is disk space storage requirements",
														"pattern":     "^[1-9][0-9]*[KMGTPE]i$",
														"type":        "string",
													},
													"storageClass": map[string]interface{}{
														"description": "StorageClass is the volume storage class",
														"type":        "string",
													},
												},
												"type": "object",
											},
											"rpc": map[string]interface{}{
												"description": "RPC is whether HTTP-RPC server is enabled or not",
												"type":        "boolean",
											},
											"rpcAPI": map[string]interface{}{
												"description": "RPCAPI is a list of rpc services to enable",
												"items": map[string]interface{}{
													"description": "API is RPC API to be exposed by RPC or web socket server",
													"enum": []interface{}{
														"admin",
														"clique",
														"debug",
														"eea",
														"eth",
														"ibft",
														"miner",
														"net",
														"perm",
														"plugins",
														"priv",
														"txpool",
														"web3",
													},
													"type": "string",
												},
												"type":                   "array",
												"x-kubernetes-list-type": "set",
											},
											"rpcPort": map[string]interface{}{
												"description": "RPCPort is HTTP-RPC server listening port",
												"type":        "integer",
											},
											"staticNodes": map[string]interface{}{
												"description": "StaticNodes is a set of ethereum nodes to maintain connection to",
												"items": map[string]interface{}{
													"description": "Enode is ethereum node url",
													"type":        "string",
												},
												"type":                   "array",
												"x-kubernetes-list-type": "set",
											},
											"syncMode": map[string]interface{}{
												"description": "SyncMode is the node synchronization mode",
												"enum": []interface{}{
													"fast",
													"full",
													"light",
													"snap",
												},
												"type": "string",
											},
											"ws": map[string]interface{}{
												"description": "WS is whether web socket server is enabled or not",
												"type":        "boolean",
											},
											"wsAPI": map[string]interface{}{
												"description": "WSAPI is a list of WS services to enable",
												"items": map[string]interface{}{
													"description": "API is RPC API to be exposed by RPC or web socket server",
													"enum": []interface{}{
														"admin",
														"clique",
														"debug",
														"eea",
														"eth",
														"ibft",
														"miner",
														"net",
														"perm",
														"plugins",
														"priv",
														"txpool",
														"web3",
													},
													"type": "string",
												},
												"type":                   "array",
												"x-kubernetes-list-type": "set",
											},
											"wsPort": map[string]interface{}{
												"description": "WSPort is the web socket server listening port",
												"type":        "integer",
											},
										},
										"required": []interface{}{
											"client",
										},
										"type": "object",
									},
									"status": map[string]interface{}{
										"description": "NodeStatus defines the observed state of Node",
										"properties": map[string]interface{}{
											"consensus": map[string]interface{}{
												"description": "Consensus is network consensus algorithm",
												"type":        "string",
											},
											"enodeURL": map[string]interface{}{
												"description": "EnodeURL is the node URL",
												"type":        "string",
											},
											"network": map[string]interface{}{
												"description": "Network is the network this node is joining",
												"type":        "string",
											},
										},
										"type": "object",
									},
								},
								"type": "object",
							},
						},
						"served":  true,
						"storage": true,
						"subresources": map[string]interface{}{
							"status": map[string]interface{}{},
						},
					},
				},
			},
			"status": map[string]interface{}{
				"acceptedNames": map[string]interface{}{
					"kind":   "",
					"plural": "",
				},
				"conditions":     []interface{}{},
				"storedVersions": []interface{}{},
			},
		},
	}

	return resourceObj

}
