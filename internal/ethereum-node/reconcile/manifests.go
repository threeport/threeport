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


func CreateCRDAptosKotalIo() (*unstructured.Unstructured) {
	var resourceObj = &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "apiextensions.k8s.io/v1",
			"kind":       "CustomResourceDefinition",
			"metadata": map[string]interface{}{
				"annotations": map[string]interface{}{
					"cert-manager.io/inject-ca-from":        "kotal/serving-cert",
					"controller-gen.kubebuilder.io/version": "v0.8.0",
				},
				"creationTimestamp": nil,
				"name":              "nodes.aptos.kotal.io",
			},
			"spec": map[string]interface{}{
				"group": "aptos.kotal.io",
				"names": map[string]interface{}{
					"kind":     "Node",
					"listKind": "NodeList",
					"plural":   "nodes",
					"singular": "node",
				},
				"scope": "Namespaced",
				"versions": []interface{}{
					map[string]interface{}{
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
										"description": "NodeSpec defines the desired state of Node",
										"properties": map[string]interface{}{
											"api": map[string]interface{}{
												"description": "API enables REST API server",
												"type":        "boolean",
											},
											"apiPort": map[string]interface{}{
												"description": "APIPort is api server port",
												"type":        "integer",
											},
											"genesisConfigmapName": map[string]interface{}{
												"description": "GenesisConfigmapName is Kubernetes configmap name holding genesis blob",
												"type":        "string",
											},
											"image": map[string]interface{}{
												"description": "Image is Aptos node client image",
												"type":        "string",
											},
											"network": map[string]interface{}{
												"description": "Network is Aptos network to join and sync",
												"enum": []interface{}{
													"devnet",
													"testnet",
												},
												"type": "string",
											},
											"nodePrivateKeySecretName": map[string]interface{}{
												"description": "NodePrivateKeySecretName is the secret name holding node private key",
												"type":        "string",
											},
											"p2pPort": map[string]interface{}{
												"description": "P2PPort is p2p communications port",
												"type":        "integer",
											},
											"peerId": map[string]interface{}{
												"description": "PeerId is the node identity",
												"type":        "string",
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
											"seedPeers": map[string]interface{}{
												"description": "SeedPeers is seed peers",
												"items": map[string]interface{}{
													"description": "Peer is Aptos network peer",
													"properties": map[string]interface{}{
														"addresses": map[string]interface{}{
															"description": "Addresses is array of peer multiaddress",
															"items": map[string]interface{}{
																"type": "string",
															},
															"minItems":               1,
															"type":                   "array",
															"x-kubernetes-list-type": "set",
														},
														"id": map[string]interface{}{
															"description": "ID is peer identifier",
															"type":        "string",
														},
													},
													"required": []interface{}{
														"addresses",
														"id",
													},
													"type": "object",
												},
												"type": "array",
											},
											"validator": map[string]interface{}{
												"description": "Validator enables validator mode",
												"type":        "boolean",
											},
											"waypoint": map[string]interface{}{
												"description": "Waypoint provides an off-chain mechanism to verify the sync process after restart or epoch change",
												"type":        "string",
											},
										},
										"required": []interface{}{
											"genesisConfigmapName",
											"network",
											"waypoint",
										},
										"type": "object",
									},
									"status": map[string]interface{}{
										"description": "NodeStatus defines the observed state of Node",
										"type":        "object",
									},
								},
								"type": "object",
							},
						},
						"served":  true,
						"storage": true,
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

func CreateCRDBitcoinKotalIo() (*unstructured.Unstructured) {
	var resourceObj = &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "apiextensions.k8s.io/v1",
			"kind":       "CustomResourceDefinition",
			"metadata": map[string]interface{}{
				"annotations": map[string]interface{}{
					"cert-manager.io/inject-ca-from":        "kotal/serving-cert",
					"controller-gen.kubebuilder.io/version": "v0.8.0",
				},
				"creationTimestamp": nil,
				"name":              "nodes.bitcoin.kotal.io",
			},
			"spec": map[string]interface{}{
				"group": "bitcoin.kotal.io",
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
								"jsonPath": ".spec.network",
								"name":     "Network",
								"type":     "string",
							},
							map[string]interface{}{
								"jsonPath": ".status.client",
								"name":     "Client",
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
										"description": "NodeSpec defines the desired state of Node",
										"properties": map[string]interface{}{
											"image": map[string]interface{}{
												"description": "Image is Bitcoin node client image",
												"type":        "string",
											},
											"network": map[string]interface{}{
												"description": "Network is Bitcoin network to join and sync",
												"enum": []interface{}{
													"mainnet",
													"testnet",
												},
												"type": "string",
											},
											"p2pPort": map[string]interface{}{
												"description": "P2PPort is p2p communications port",
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
												"description": "RPC enables JSON-RPC server",
												"type":        "boolean",
											},
											"rpcPort": map[string]interface{}{
												"description": "RPCPort is JSON-RPC server port",
												"type":        "integer",
											},
											"rpcUsers": map[string]interface{}{
												"description": "RPCUsers is JSON-RPC users credentials",
												"items": map[string]interface{}{
													"description": "RPCUsers is JSON-RPC users credentials",
													"properties": map[string]interface{}{
														"passwordSecretName": map[string]interface{}{
															"description": "PasswordSecretName is k8s secret name holding JSON-RPC user password",
															"type":        "string",
														},
														"username": map[string]interface{}{
															"description": "Username is JSON-RPC username",
															"type":        "string",
														},
													},
													"required": []interface{}{
														"passwordSecretName",
														"username",
													},
													"type": "object",
												},
												"type": "array",
											},
											"txIndex": map[string]interface{}{
												"description": "TransactionIndex maintains a full tx index",
												"type":        "boolean",
											},
											"wallet": map[string]interface{}{
												"description": "Wallet load wallet and enables wallet RPC calls",
												"type":        "boolean",
											},
										},
										"required": []interface{}{
											"network",
										},
										"type": "object",
									},
									"status": map[string]interface{}{
										"description": "NodeStatus defines the observed state of Node",
										"properties": map[string]interface{}{
											"client": map[string]interface{}{
												"type": "string",
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

func CreateCRDChainlinkKotalIo() (*unstructured.Unstructured) {
	var resourceObj = &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "apiextensions.k8s.io/v1",
			"kind":       "CustomResourceDefinition",
			"metadata": map[string]interface{}{
				"annotations": map[string]interface{}{
					"cert-manager.io/inject-ca-from":        "kotal/serving-cert",
					"controller-gen.kubebuilder.io/version": "v0.8.0",
				},
				"creationTimestamp": nil,
				"name":              "nodes.chainlink.kotal.io",
			},
			"spec": map[string]interface{}{
				"group": "chainlink.kotal.io",
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
								"jsonPath": ".status.client",
								"name":     "Client",
								"type":     "string",
							},
							map[string]interface{}{
								"jsonPath": ".spec.ethereumChainId",
								"name":     "EthereumChainId",
								"type":     "number",
							},
							map[string]interface{}{
								"jsonPath": ".spec.linkContractAddress",
								"name":     "LinkContractAddress",
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
										"description": "NodeSpec defines the desired state of Node",
										"properties": map[string]interface{}{
											"api": map[string]interface{}{
												"description": "API enables node API server",
												"type":        "boolean",
											},
											"apiCredentials": map[string]interface{}{
												"description": "APICredentials is api credentials",
												"properties": map[string]interface{}{
													"email": map[string]interface{}{
														"description": "Email is user email",
														"type":        "string",
													},
													"passwordSecretName": map[string]interface{}{
														"description": "PasswordSecretName is the k8s secret name that holds password",
														"type":        "string",
													},
												},
												"required": []interface{}{
													"email",
													"passwordSecretName",
												},
												"type": "object",
											},
											"apiPort": map[string]interface{}{
												"description": "APIPort is port used for node API and GUI",
												"type":        "integer",
											},
											"certSecretName": map[string]interface{}{
												"description": "CertSecretName is k8s secret name that holds tls.key and tls.cert",
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
											"databaseURL": map[string]interface{}{
												"description": "DatabaseURL is postgres database connection URL",
												"type":        "string",
											},
											"ethereumChainId": map[string]interface{}{
												"description": "EthereumChainId is ethereum chain id",
												"type":        "integer",
											},
											"ethereumHttpEndpoints": map[string]interface{}{
												"description": "EthereumHTTPEndpoints is ethereum http endpoints",
												"items": map[string]interface{}{
													"type": "string",
												},
												"type":                   "array",
												"x-kubernetes-list-type": "set",
											},
											"ethereumWsEndpoint": map[string]interface{}{
												"description": "EthereumWSEndpoint is ethereum websocket endpoint",
												"type":        "string",
											},
											"image": map[string]interface{}{
												"description": "Image is Chainlink node client image",
												"type":        "string",
											},
											"keystorePasswordSecretName": map[string]interface{}{
												"description": "KeystorePasswordSecretName is k8s secret name that holds keystore password",
												"type":        "string",
											},
											"linkContractAddress": map[string]interface{}{
												"description": "LinkContractAddress is link contract address",
												"type":        "string",
											},
											"logging": map[string]interface{}{
												"description": "Logging is logging verboisty level",
												"enum": []interface{}{
													"debug",
													"info",
													"warn",
													"error",
													"panic",
												},
												"type": "string",
											},
											"p2pPort": map[string]interface{}{
												"description": "P2PPort is port used for p2p communcations",
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
											"secureCookies": map[string]interface{}{
												"description": "SecureCookies enables secure cookies for authentication",
												"type":        "boolean",
											},
											"tlsPort": map[string]interface{}{
												"description": "TLSPort is port used for HTTPS connections",
												"type":        "integer",
											},
										},
										"required": []interface{}{
											"apiCredentials",
											"databaseURL",
											"ethereumChainId",
											"ethereumWsEndpoint",
											"keystorePasswordSecretName",
											"linkContractAddress",
										},
										"type": "object",
									},
									"status": map[string]interface{}{
										"description": "NodeStatus defines the observed state of Node",
										"properties": map[string]interface{}{
											"client": map[string]interface{}{
												"type": "string",
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

func CreateCRDIpfsKotalIo() (*unstructured.Unstructured) {
	var resourceObj = &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "apiextensions.k8s.io/v1",
			"kind":       "CustomResourceDefinition",
			"metadata": map[string]interface{}{
				"annotations": map[string]interface{}{
					"cert-manager.io/inject-ca-from":        "kotal/serving-cert",
					"controller-gen.kubebuilder.io/version": "v0.8.0",
				},
				"creationTimestamp": nil,
				"name":              "clusterpeers.ipfs.kotal.io",
			},
			"spec": map[string]interface{}{
				"group": "ipfs.kotal.io",
				"names": map[string]interface{}{
					"kind":     "ClusterPeer",
					"listKind": "ClusterPeerList",
					"plural":   "clusterpeers",
					"singular": "clusterpeer",
				},
				"scope": "Namespaced",
				"versions": []interface{}{
					map[string]interface{}{
						"additionalPrinterColumns": []interface{}{
							map[string]interface{}{
								"jsonPath": ".status.client",
								"name":     "Client",
								"type":     "string",
							},
							map[string]interface{}{
								"jsonPath": ".spec.consensus",
								"name":     "Consensus",
								"type":     "string",
							},
						},
						"name": "v1alpha1",
						"schema": map[string]interface{}{
							"openAPIV3Schema": map[string]interface{}{
								"description": "ClusterPeer is the Schema for the clusterpeers API",
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
										"description": "ClusterPeerSpec defines the desired state of ClusterPeer",
										"properties": map[string]interface{}{
											"bootstrapPeers": map[string]interface{}{
												"description": "BootstrapPeers are ipfs cluster peers to connect to",
												"items": map[string]interface{}{
													"type": "string",
												},
												"type":                   "array",
												"x-kubernetes-list-type": "set",
											},
											"clusterSecretName": map[string]interface{}{
												"description": "ClusterSecretName is k8s secret holding cluster secret",
												"type":        "string",
											},
											"consensus": map[string]interface{}{
												"description": "Consensus is ipfs cluster consensus algorithm",
												"enum": []interface{}{
													"crdt",
													"raft",
												},
												"type": "string",
											},
											"id": map[string]interface{}{
												"description": "ID is the the cluster peer id",
												"type":        "string",
											},
											"image": map[string]interface{}{
												"description": "Image is ipfs cluster peer client image",
												"type":        "string",
											},
											"logging": map[string]interface{}{
												"description": "Logging is logging verboisty level",
												"enum": []interface{}{
													"error",
													"warn",
													"info",
													"debug",
												},
												"type": "string",
											},
											"peerEndpoint": map[string]interface{}{
												"description": "PeerEndpoint is ipfs peer http API endpoint",
												"type":        "string",
											},
											"privateKeySecretName": map[string]interface{}{
												"description": "PrivateKeySecretName is k8s secret holding private key",
												"type":        "string",
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
											"trustedPeers": map[string]interface{}{
												"description": "TrustedPeers is CRDT trusted cluster peers who can manage the pinset",
												"items": map[string]interface{}{
													"type": "string",
												},
												"type":                   "array",
												"x-kubernetes-list-type": "set",
											},
										},
										"required": []interface{}{
											"clusterSecretName",
											"peerEndpoint",
										},
										"type": "object",
									},
									"status": map[string]interface{}{
										"description": "ClusterPeerStatus defines the observed state of ClusterPeer",
										"properties": map[string]interface{}{
											"client": map[string]interface{}{
												"type": "string",
											},
											"consensus": map[string]interface{}{
												"type": "string",
											},
										},
										"required": []interface{}{
											"client",
											"consensus",
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

func CreateManifestClusterRole() (*unstructured.Unstructured) {
	var resourceObj = &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "rbac.authorization.k8s.io/v1",
			"kind":       "ClusterRole",
			"metadata": map[string]interface{}{
				"creationTimestamp": nil,
				"name":              "manager-role",
			},
			"rules": []interface{}{
				map[string]interface{}{
					"apiGroups": []interface{}{
						"apps",
					},
					"resources": []interface{}{
						"statefulsets",
					},
					"verbs": []interface{}{
						"create",
						"delete",
						"get",
						"list",
						"update",
						"watch",
					},
				},
				map[string]interface{}{
					"apiGroups": []interface{}{
						"aptos.kotal.io",
					},
					"resources": []interface{}{
						"nodes",
					},
					"verbs": []interface{}{
						"create",
						"delete",
						"get",
						"list",
						"patch",
						"update",
						"watch",
					},
				},
				map[string]interface{}{
					"apiGroups": []interface{}{
						"aptos.kotal.io",
					},
					"resources": []interface{}{
						"nodes/status",
					},
					"verbs": []interface{}{
						"get",
						"patch",
						"update",
					},
				},
				map[string]interface{}{
					"apiGroups": []interface{}{
						"bitcoin.kotal.io",
					},
					"resources": []interface{}{
						"nodes",
					},
					"verbs": []interface{}{
						"create",
						"delete",
						"get",
						"list",
						"patch",
						"update",
						"watch",
					},
				},
				map[string]interface{}{
					"apiGroups": []interface{}{
						"bitcoin.kotal.io",
					},
					"resources": []interface{}{
						"nodes/status",
					},
					"verbs": []interface{}{
						"get",
						"patch",
						"update",
					},
				},
				map[string]interface{}{
					"apiGroups": []interface{}{
						"chainlink.kotal.io",
					},
					"resources": []interface{}{
						"nodes",
					},
					"verbs": []interface{}{
						"create",
						"delete",
						"get",
						"list",
						"patch",
						"update",
						"watch",
					},
				},
				map[string]interface{}{
					"apiGroups": []interface{}{
						"chainlink.kotal.io",
					},
					"resources": []interface{}{
						"nodes/status",
					},
					"verbs": []interface{}{
						"get",
						"patch",
						"update",
					},
				},
				map[string]interface{}{
					"apiGroups": []interface{}{
						"",
					},
					"resources": []interface{}{
						"configmaps",
						"persistentvolumeclaims",
					},
					"verbs": []interface{}{
						"create",
						"delete",
						"get",
						"list",
						"update",
						"watch",
					},
				},
				map[string]interface{}{
					"apiGroups": []interface{}{
						"",
					},
					"resources": []interface{}{
						"configmaps",
						"persistentvolumeclaims",
						"secrets",
						"services",
					},
					"verbs": []interface{}{
						"create",
						"delete",
						"get",
						"list",
						"update",
						"watch",
					},
				},
				map[string]interface{}{
					"apiGroups": []interface{}{
						"",
					},
					"resources": []interface{}{
						"configmaps",
						"persistentvolumeclaims",
						"services",
					},
					"verbs": []interface{}{
						"create",
						"delete",
						"get",
						"list",
						"update",
						"watch",
					},
				},
				map[string]interface{}{
					"apiGroups": []interface{}{
						"",
					},
					"resources": []interface{}{
						"configmaps",
						"services",
					},
					"verbs": []interface{}{
						"create",
						"delete",
						"get",
						"list",
						"update",
						"watch",
					},
				},
				map[string]interface{}{
					"apiGroups": []interface{}{
						"",
					},
					"resources": []interface{}{
						"persistentvolumeclaims",
					},
					"verbs": []interface{}{
						"create",
						"delete",
						"get",
						"list",
						"update",
						"watch",
					},
				},
				map[string]interface{}{
					"apiGroups": []interface{}{
						"",
					},
					"resources": []interface{}{
						"persistentvolumeclaims",
						"services",
					},
					"verbs": []interface{}{
						"create",
						"delete",
						"get",
						"list",
						"update",
						"watch",
					},
				},
				map[string]interface{}{
					"apiGroups": []interface{}{
						"ethereum.kotal.io",
					},
					"resources": []interface{}{
						"nodes",
					},
					"verbs": []interface{}{
						"create",
						"delete",
						"get",
						"list",
						"patch",
						"update",
						"watch",
					},
				},
				map[string]interface{}{
					"apiGroups": []interface{}{
						"ethereum.kotal.io",
					},
					"resources": []interface{}{
						"nodes/status",
					},
					"verbs": []interface{}{
						"get",
						"patch",
						"update",
					},
				},
				map[string]interface{}{
					"apiGroups": []interface{}{
						"ethereum2.kotal.io",
					},
					"resources": []interface{}{
						"beaconnodes",
					},
					"verbs": []interface{}{
						"create",
						"delete",
						"get",
						"list",
						"patch",
						"update",
						"watch",
					},
				},
				map[string]interface{}{
					"apiGroups": []interface{}{
						"ethereum2.kotal.io",
					},
					"resources": []interface{}{
						"beaconnodes/status",
					},
					"verbs": []interface{}{
						"get",
						"patch",
						"update",
					},
				},
				map[string]interface{}{
					"apiGroups": []interface{}{
						"ethereum2.kotal.io",
					},
					"resources": []interface{}{
						"validators",
					},
					"verbs": []interface{}{
						"create",
						"delete",
						"get",
						"list",
						"patch",
						"update",
						"watch",
					},
				},
				map[string]interface{}{
					"apiGroups": []interface{}{
						"ethereum2.kotal.io",
					},
					"resources": []interface{}{
						"validators/status",
					},
					"verbs": []interface{}{
						"get",
						"patch",
						"update",
					},
				},
				map[string]interface{}{
					"apiGroups": []interface{}{
						"filecoin.kotal.io",
					},
					"resources": []interface{}{
						"nodes",
					},
					"verbs": []interface{}{
						"create",
						"delete",
						"get",
						"list",
						"patch",
						"update",
						"watch",
					},
				},
				map[string]interface{}{
					"apiGroups": []interface{}{
						"filecoin.kotal.io",
					},
					"resources": []interface{}{
						"nodes/status",
					},
					"verbs": []interface{}{
						"get",
						"patch",
						"update",
					},
				},
				map[string]interface{}{
					"apiGroups": []interface{}{
						"graph.kotal.io",
					},
					"resources": []interface{}{
						"nodes",
					},
					"verbs": []interface{}{
						"create",
						"delete",
						"get",
						"list",
						"patch",
						"update",
						"watch",
					},
				},
				map[string]interface{}{
					"apiGroups": []interface{}{
						"graph.kotal.io",
					},
					"resources": []interface{}{
						"nodes/finalizers",
					},
					"verbs": []interface{}{
						"update",
					},
				},
				map[string]interface{}{
					"apiGroups": []interface{}{
						"graph.kotal.io",
					},
					"resources": []interface{}{
						"nodes/status",
					},
					"verbs": []interface{}{
						"get",
						"patch",
						"update",
					},
				},
				map[string]interface{}{
					"apiGroups": []interface{}{
						"ipfs.kotal.io",
					},
					"resources": []interface{}{
						"clusterpeers",
					},
					"verbs": []interface{}{
						"create",
						"delete",
						"get",
						"list",
						"patch",
						"update",
						"watch",
					},
				},
				map[string]interface{}{
					"apiGroups": []interface{}{
						"ipfs.kotal.io",
					},
					"resources": []interface{}{
						"clusterpeers/status",
					},
					"verbs": []interface{}{
						"get",
						"patch",
						"update",
					},
				},
				map[string]interface{}{
					"apiGroups": []interface{}{
						"ipfs.kotal.io",
					},
					"resources": []interface{}{
						"peers",
					},
					"verbs": []interface{}{
						"create",
						"delete",
						"get",
						"list",
						"patch",
						"update",
						"watch",
					},
				},
				map[string]interface{}{
					"apiGroups": []interface{}{
						"ipfs.kotal.io",
					},
					"resources": []interface{}{
						"peers/status",
					},
					"verbs": []interface{}{
						"get",
						"patch",
						"update",
					},
				},
				map[string]interface{}{
					"apiGroups": []interface{}{
						"near.kotal.io",
					},
					"resources": []interface{}{
						"nodes",
					},
					"verbs": []interface{}{
						"create",
						"delete",
						"get",
						"list",
						"patch",
						"update",
						"watch",
					},
				},
				map[string]interface{}{
					"apiGroups": []interface{}{
						"near.kotal.io",
					},
					"resources": []interface{}{
						"nodes/status",
					},
					"verbs": []interface{}{
						"get",
						"patch",
						"update",
					},
				},
				map[string]interface{}{
					"apiGroups": []interface{}{
						"polkadot.kotal.io",
					},
					"resources": []interface{}{
						"nodes",
					},
					"verbs": []interface{}{
						"create",
						"delete",
						"get",
						"list",
						"patch",
						"update",
						"watch",
					},
				},
				map[string]interface{}{
					"apiGroups": []interface{}{
						"polkadot.kotal.io",
					},
					"resources": []interface{}{
						"nodes/status",
					},
					"verbs": []interface{}{
						"get",
						"patch",
						"update",
					},
				},
				map[string]interface{}{
					"apiGroups": []interface{}{
						"stacks.kotal.io",
					},
					"resources": []interface{}{
						"nodes",
					},
					"verbs": []interface{}{
						"create",
						"delete",
						"get",
						"list",
						"patch",
						"update",
						"watch",
					},
				},
				map[string]interface{}{
					"apiGroups": []interface{}{
						"stacks.kotal.io",
					},
					"resources": []interface{}{
						"nodes/status",
					},
					"verbs": []interface{}{
						"get",
						"patch",
						"update",
					},
				},
			},
		},
	}

	return resourceObj

}

func CreateManifestClusterRoleMetricsReader() (*unstructured.Unstructured) {
	var resourceObj = &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "rbac.authorization.k8s.io/v1",
			"kind":       "ClusterRole",
			"metadata": map[string]interface{}{
				"name": "metrics-reader",
			},
			"rules": []interface{}{
				map[string]interface{}{
					"nonResourceURLs": []interface{}{
						"/metrics",
					},
					"verbs": []interface{}{
						"get",
					},
				},
			},
		},
	}

	return resourceObj

}

func CreateManifestClusterRoleProxyRole() (*unstructured.Unstructured) {
	var resourceObj = &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "rbac.authorization.k8s.io/v1",
			"kind":       "ClusterRole",
			"metadata": map[string]interface{}{
				"name": "proxy-role",
			},
			"rules": []interface{}{
				map[string]interface{}{
					"apiGroups": []interface{}{
						"authentication.k8s.io",
					},
					"resources": []interface{}{
						"tokenreviews",
					},
					"verbs": []interface{}{
						"create",
					},
				},
				map[string]interface{}{
					"apiGroups": []interface{}{
						"authorization.k8s.io",
					},
					"resources": []interface{}{
						"subjectaccessreviews",
					},
					"verbs": []interface{}{
						"create",
					},
				},
			},
		},
	}

	return resourceObj

}

func CreateManifestRoleBindingLeaderElection() (*unstructured.Unstructured) {
	var resourceObj = &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "rbac.authorization.k8s.io/v1",
			"kind":       "RoleBinding",
			"metadata": map[string]interface{}{
				"name":      "leader-election-rolebinding",
				"namespace": "kotal",
			},
			"roleRef": map[string]interface{}{
				"apiGroup": "rbac.authorization.k8s.io",
				"kind":     "Role",
				"name":     "leader-election-role",
			},
			"subjects": []interface{}{
				map[string]interface{}{
					"kind":      "ServiceAccount",
					"name":      "controller-manager",
					"namespace": "kotal",
				},
			},
		},
	}

	return resourceObj

}

func CreateManifestClusterRoleBindingManager() (*unstructured.Unstructured) {
	var resourceObj = &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "rbac.authorization.k8s.io/v1",
			"kind":       "ClusterRoleBinding",
			"metadata": map[string]interface{}{
				"name": "manager-rolebinding",
			},
			"roleRef": map[string]interface{}{
				"apiGroup": "rbac.authorization.k8s.io",
				"kind":     "ClusterRole",
				"name":     "manager-role",
			},
			"subjects": []interface{}{
				map[string]interface{}{
					"kind":      "ServiceAccount",
					"name":      "controller-manager",
					"namespace": "kotal",
				},
			},
		},
	}
	return resourceObj

}

func CreateManifestClusterRoleBindingProxy() (*unstructured.Unstructured) {
	var resourceObj = &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "rbac.authorization.k8s.io/v1",
			"kind":       "ClusterRoleBinding",
			"metadata": map[string]interface{}{
				"name": "proxy-rolebinding",
			},
			"roleRef": map[string]interface{}{
				"apiGroup": "rbac.authorization.k8s.io",
				"kind":     "ClusterRole",
				"name":     "proxy-role",
			},
			"subjects": []interface{}{
				map[string]interface{}{
					"kind":      "ServiceAccount",
					"name":      "controller-manager",
					"namespace": "kotal",
				},
			},
		},
	}
	return resourceObj

}

func CreateManifestDeploymentControllerManager() (*unstructured.Unstructured) {
	var resourceObj = &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "apps/v1",
			"kind":       "Deployment",
			"metadata": map[string]interface{}{
				"labels": map[string]interface{}{
					"control-plane": "controller-manager",
				},
				"name":      "controller-manager",
				"namespace": "kotal",
			},
			"spec": map[string]interface{}{
				"replicas": 1,
				"selector": map[string]interface{}{
					"matchLabels": map[string]interface{}{
						"control-plane": "controller-manager",
					},
				},
				"template": map[string]interface{}{
					"metadata": map[string]interface{}{
						"annotations": map[string]interface{}{
							"kubectl.kubernetes.io/default-container": "manager",
						},
						"labels": map[string]interface{}{
							"control-plane": "controller-manager",
						},
					},
					"spec": map[string]interface{}{
						"containers": []interface{}{
							map[string]interface{}{
								"args": []interface{}{
									"--health-probe-bind-address=:8081",
									"--metrics-bind-address=127.0.0.1:8080",
									"--leader-elect",
								},
								"command": []interface{}{
									"/manager",
								},
								"image": "localhost/kotal-controller-manager:latest",
								"imagePullPolicy": "IfNotPresent",
								"livenessProbe": map[string]interface{}{
									"httpGet": map[string]interface{}{
										"path": "/healthz",
										"port": 8081,
									},
									"initialDelaySeconds": 15,
									"periodSeconds":       20,
								},
								"name": "manager",
								"ports": []interface{}{
									map[string]interface{}{
										"containerPort": 9443,
										"name":          "webhook-server",
										"protocol":      "TCP",
									},
								},
								"readinessProbe": map[string]interface{}{
									"httpGet": map[string]interface{}{
										"path": "/readyz",
										"port": 8081,
									},
									"initialDelaySeconds": 5,
									"periodSeconds":       10,
								},
								"resources": map[string]interface{}{
									"requests": map[string]interface{}{
										"cpu":    "50m",
										"memory": "100Mi",
									},
								},
								"securityContext": map[string]interface{}{
									"allowPrivilegeEscalation": false,
								},
								"volumeMounts": []interface{}{
									map[string]interface{}{
										"mountPath": "/tmp/k8s-webhook-server/serving-certs",
										"name":      "cert",
										"readOnly":  true,
									},
								},
							},
							map[string]interface{}{
								"args": []interface{}{
									"--secure-listen-address=0.0.0.0:8443",
									"--upstream=http://127.0.0.1:8080/",
									"--logtostderr=true",
									"--v=0",
								},
								"image": "gcr.io/kubebuilder/kube-rbac-proxy:v0.11.0",
								"name":  "kube-rbac-proxy",
								"ports": []interface{}{
									map[string]interface{}{
										"containerPort": 8443,
										"name":          "https",
										"protocol":      "TCP",
									},
								},
								"resources": map[string]interface{}{
									"limits": map[string]interface{}{
										"cpu":    "500m",
										"memory": "128Mi",
									},
									"requests": map[string]interface{}{
										"cpu":    "5m",
										"memory": "64Mi",
									},
								},
							},
						},
						"securityContext": map[string]interface{}{
							"runAsNonRoot": true,
						},
						"serviceAccountName":            "controller-manager",
						"terminationGracePeriodSeconds": 10,
						"volumes": []interface{}{
							map[string]interface{}{
								"name": "cert",
								"secret": map[string]interface{}{
									"defaultMode": 420,
									"secretName":  "webhook-server-cert",
								},
							},
						},
					},
				},
			},
		},
	}

	return resourceObj

}

func CreateCRDFilecoinKotalIo() (*unstructured.Unstructured) {
	var resourceObj = &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "apiextensions.k8s.io/v1",
			"kind":       "CustomResourceDefinition",
			"metadata": map[string]interface{}{
				"annotations": map[string]interface{}{
					"cert-manager.io/inject-ca-from":        "kotal/serving-cert",
					"controller-gen.kubebuilder.io/version": "v0.8.0",
				},
				"creationTimestamp": nil,
				"name":              "nodes.filecoin.kotal.io",
			},
			"spec": map[string]interface{}{
				"group": "filecoin.kotal.io",
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
								"jsonPath": ".spec.network",
								"name":     "Network",
								"type":     "string",
							},
							map[string]interface{}{
								"jsonPath": ".status.client",
								"name":     "Client",
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
										"description": "NodeSpec defines the desired state of Node",
										"properties": map[string]interface{}{
											"api": map[string]interface{}{
												"description": "API enables API server",
												"type":        "boolean",
											},
											"apiPort": map[string]interface{}{
												"description": "APIPort is API server listening port",
												"type":        "integer",
											},
											"apiRequestTimeout": map[string]interface{}{
												"description": "APIRequestTimeout is API request timeout in seconds",
												"type":        "integer",
											},
											"disableMetadataLog": map[string]interface{}{
												"description": "DisableMetadataLog disables metadata log",
												"type":        "boolean",
											},
											"image": map[string]interface{}{
												"description": "Image is Filecoin node client image",
												"type":        "string",
											},
											"ipfsForRetrieval": map[string]interface{}{
												"description": "IPFSForRetrieval uses ipfs for retrieval",
												"type":        "boolean",
											},
											"ipfsOnlineMode": map[string]interface{}{
												"description": "IPFSOnlineMode sets ipfs online mode",
												"type":        "boolean",
											},
											"ipfsPeerEndpoint": map[string]interface{}{
												"description": "IPFSPeerEndpoint is ipfs peer endpoint",
												"type":        "string",
											},
											"logging": map[string]interface{}{
												"description": "Logging is logging verboisty level",
												"enum": []interface{}{
													"error",
													"warn",
													"info",
													"debug",
												},
												"type": "string",
											},
											"network": map[string]interface{}{
												"description": "Network is the Filecoin network the node will join and sync",
												"enum": []interface{}{
													"mainnet",
													"calibration",
												},
												"type": "string",
											},
											"p2pPort": map[string]interface{}{
												"description": "P2PPort is p2p port",
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
										},
										"required": []interface{}{
											"network",
										},
										"type": "object",
									},
									"status": map[string]interface{}{
										"description": "NodeStatus defines the observed state of Node",
										"properties": map[string]interface{}{
											"client": map[string]interface{}{
												"type": "string",
											},
										},
										"required": []interface{}{
											"client",
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

func CreateCRDGraphKotalIo() (*unstructured.Unstructured) {
	var resourceObj = &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "apiextensions.k8s.io/v1",
			"kind":       "CustomResourceDefinition",
			"metadata": map[string]interface{}{
				"annotations": map[string]interface{}{
					"cert-manager.io/inject-ca-from":        "kotal/serving-cert",
					"controller-gen.kubebuilder.io/version": "v0.8.0",
				},
				"creationTimestamp": nil,
				"name":              "nodes.graph.kotal.io",
			},
			"spec": map[string]interface{}{
				"group": "graph.kotal.io",
				"names": map[string]interface{}{
					"kind":     "Node",
					"listKind": "NodeList",
					"plural":   "nodes",
					"singular": "node",
				},
				"scope": "Namespaced",
				"versions": []interface{}{
					map[string]interface{}{
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
										"description": "NodeSpec defines the desired state of Node",
										"properties": map[string]interface{}{
											"image": map[string]interface{}{
												"description": "TODO: default node image Image is Graph node client image",
												"type":        "string",
											},
										},
										"type": "object",
									},
									"status": map[string]interface{}{
										"description": "NodeStatus defines the observed state of Node",
										"type":        "object",
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

func CreateManifestIssuer() (*unstructured.Unstructured) {
	var resourceObj = &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "cert-manager.io/v1",
			"kind":       "Issuer",
			"metadata": map[string]interface{}{
				"name":      "selfsigned-issuer",
				"namespace": "kotal",
			},
			"spec": map[string]interface{}{
				"selfSigned": map[string]interface{}{},
			},
		},
	}

	return resourceObj

}

func CreateManifestMutatingWebhookConfiguration() (*unstructured.Unstructured) {
	var resourceObj = &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "admissionregistration.k8s.io/v1",
			"kind":       "MutatingWebhookConfiguration",
			"metadata": map[string]interface{}{
				"annotations": map[string]interface{}{
					"cert-manager.io/inject-ca-from": "kotal/serving-cert",
				},
				"creationTimestamp": nil,
				"name":              "mutating-webhook-configuration",
			},
			"webhooks": []interface{}{
				map[string]interface{}{
					"admissionReviewVersions": []interface{}{
						"v1",
					},
					"clientConfig": map[string]interface{}{
						"service": map[string]interface{}{
							"name":      "webhook-service",
							"namespace": "kotal",
							"path":      "/mutate-aptos-kotal-io-v1alpha1-node",
						},
					},
					"failurePolicy": "Fail",
					"name":          "mutate-aptos-v1alpha1-node.kb.io",
					"rules": []interface{}{
						map[string]interface{}{
							"apiGroups": []interface{}{
								"aptos.kotal.io",
							},
							"apiVersions": []interface{}{
								"v1alpha1",
							},
							"operations": []interface{}{
								"CREATE",
								"UPDATE",
							},
							"resources": []interface{}{
								"nodes",
							},
						},
					},
					"sideEffects": "None",
				},
				map[string]interface{}{
					"admissionReviewVersions": []interface{}{
						"v1",
					},
					"clientConfig": map[string]interface{}{
						"service": map[string]interface{}{
							"name":      "webhook-service",
							"namespace": "kotal",
							"path":      "/mutate-bitcoin-kotal-io-v1alpha1-node",
						},
					},
					"failurePolicy": "Fail",
					"name":          "mutate-bitcoin-v1alpha1-node.kb.io",
					"rules": []interface{}{
						map[string]interface{}{
							"apiGroups": []interface{}{
								"bitcoin.kotal.io",
							},
							"apiVersions": []interface{}{
								"v1alpha1",
							},
							"operations": []interface{}{
								"CREATE",
								"UPDATE",
							},
							"resources": []interface{}{
								"nodes",
							},
						},
					},
					"sideEffects": "None",
				},
				map[string]interface{}{
					"admissionReviewVersions": []interface{}{
						"v1",
					},
					"clientConfig": map[string]interface{}{
						"service": map[string]interface{}{
							"name":      "webhook-service",
							"namespace": "kotal",
							"path":      "/mutate-chainlink-kotal-io-v1alpha1-node",
						},
					},
					"failurePolicy": "Fail",
					"name":          "mutate-chainlink-v1alpha1-node.kb.io",
					"rules": []interface{}{
						map[string]interface{}{
							"apiGroups": []interface{}{
								"chainlink.kotal.io",
							},
							"apiVersions": []interface{}{
								"v1alpha1",
							},
							"operations": []interface{}{
								"CREATE",
								"UPDATE",
							},
							"resources": []interface{}{
								"nodes",
							},
						},
					},
					"sideEffects": "None",
				},
				map[string]interface{}{
					"admissionReviewVersions": []interface{}{
						"v1",
					},
					"clientConfig": map[string]interface{}{
						"service": map[string]interface{}{
							"name":      "webhook-service",
							"namespace": "kotal",
							"path":      "/mutate-ethereum-kotal-io-v1alpha1-node",
						},
					},
					"failurePolicy": "Fail",
					"name":          "mutate-ethereum-v1alpha1-node.kb.io",
					"rules": []interface{}{
						map[string]interface{}{
							"apiGroups": []interface{}{
								"ethereum.kotal.io",
							},
							"apiVersions": []interface{}{
								"v1alpha1",
							},
							"operations": []interface{}{
								"CREATE",
								"UPDATE",
							},
							"resources": []interface{}{
								"nodes",
							},
						},
					},
					"sideEffects": "None",
				},
				map[string]interface{}{
					"admissionReviewVersions": []interface{}{
						"v1",
					},
					"clientConfig": map[string]interface{}{
						"service": map[string]interface{}{
							"name":      "webhook-service",
							"namespace": "kotal",
							"path":      "/mutate-ethereum2-kotal-io-v1alpha1-beaconnode",
						},
					},
					"failurePolicy": "Fail",
					"name":          "mutate-ethereum2-v1alpha1-beaconnode.kb.io",
					"rules": []interface{}{
						map[string]interface{}{
							"apiGroups": []interface{}{
								"ethereum2.kotal.io",
							},
							"apiVersions": []interface{}{
								"v1alpha1",
							},
							"operations": []interface{}{
								"CREATE",
								"UPDATE",
							},
							"resources": []interface{}{
								"beaconnodes",
							},
						},
					},
					"sideEffects": "None",
				},
				map[string]interface{}{
					"admissionReviewVersions": []interface{}{
						"v1",
					},
					"clientConfig": map[string]interface{}{
						"service": map[string]interface{}{
							"name":      "webhook-service",
							"namespace": "kotal",
							"path":      "/mutate-ethereum2-kotal-io-v1alpha1-validator",
						},
					},
					"failurePolicy": "Fail",
					"name":          "mutate-ethereum2-v1alpha1-validator.kb.io",
					"rules": []interface{}{
						map[string]interface{}{
							"apiGroups": []interface{}{
								"ethereum2.kotal.io",
							},
							"apiVersions": []interface{}{
								"v1alpha1",
							},
							"operations": []interface{}{
								"CREATE",
								"UPDATE",
							},
							"resources": []interface{}{
								"validators",
							},
						},
					},
					"sideEffects": "None",
				},
				map[string]interface{}{
					"admissionReviewVersions": []interface{}{
						"v1",
					},
					"clientConfig": map[string]interface{}{
						"service": map[string]interface{}{
							"name":      "webhook-service",
							"namespace": "kotal",
							"path":      "/mutate-filecoin-kotal-io-v1alpha1-node",
						},
					},
					"failurePolicy": "Fail",
					"name":          "mutate-filecoin-v1alpha1-node.kb.io",
					"rules": []interface{}{
						map[string]interface{}{
							"apiGroups": []interface{}{
								"filecoin.kotal.io",
							},
							"apiVersions": []interface{}{
								"v1alpha1",
							},
							"operations": []interface{}{
								"CREATE",
								"UPDATE",
							},
							"resources": []interface{}{
								"nodes",
							},
						},
					},
					"sideEffects": "None",
				},
				map[string]interface{}{
					"admissionReviewVersions": []interface{}{
						"v1",
					},
					"clientConfig": map[string]interface{}{
						"service": map[string]interface{}{
							"name":      "webhook-service",
							"namespace": "kotal",
							"path":      "/mutate-ipfs-kotal-io-v1alpha1-clusterpeer",
						},
					},
					"failurePolicy": "Fail",
					"name":          "mutate-ipfs-v1alpha1-clusterpeer.kb.io",
					"rules": []interface{}{
						map[string]interface{}{
							"apiGroups": []interface{}{
								"ipfs.kotal.io",
							},
							"apiVersions": []interface{}{
								"v1alpha1",
							},
							"operations": []interface{}{
								"CREATE",
								"UPDATE",
							},
							"resources": []interface{}{
								"clusterpeers",
							},
						},
					},
					"sideEffects": "None",
				},
				map[string]interface{}{
					"admissionReviewVersions": []interface{}{
						"v1",
					},
					"clientConfig": map[string]interface{}{
						"service": map[string]interface{}{
							"name":      "webhook-service",
							"namespace": "kotal",
							"path":      "/mutate-ipfs-kotal-io-v1alpha1-peer",
						},
					},
					"failurePolicy": "Fail",
					"name":          "mutate-ipfs-v1alpha1-peer.kb.io",
					"rules": []interface{}{
						map[string]interface{}{
							"apiGroups": []interface{}{
								"ipfs.kotal.io",
							},
							"apiVersions": []interface{}{
								"v1alpha1",
							},
							"operations": []interface{}{
								"CREATE",
								"UPDATE",
							},
							"resources": []interface{}{
								"peers",
							},
						},
					},
					"sideEffects": "None",
				},
				map[string]interface{}{
					"admissionReviewVersions": []interface{}{
						"v1",
					},
					"clientConfig": map[string]interface{}{
						"service": map[string]interface{}{
							"name":      "webhook-service",
							"namespace": "kotal",
							"path":      "/mutate-near-kotal-io-v1alpha1-node",
						},
					},
					"failurePolicy": "Fail",
					"name":          "mutate-near-v1alpha1-node.kb.io",
					"rules": []interface{}{
						map[string]interface{}{
							"apiGroups": []interface{}{
								"near.kotal.io",
							},
							"apiVersions": []interface{}{
								"v1alpha1",
							},
							"operations": []interface{}{
								"CREATE",
								"UPDATE",
							},
							"resources": []interface{}{
								"nodes",
							},
						},
					},
					"sideEffects": "None",
				},
				map[string]interface{}{
					"admissionReviewVersions": []interface{}{
						"v1",
					},
					"clientConfig": map[string]interface{}{
						"service": map[string]interface{}{
							"name":      "webhook-service",
							"namespace": "kotal",
							"path":      "/mutate-polkadot-kotal-io-v1alpha1-node",
						},
					},
					"failurePolicy": "Fail",
					"name":          "mutate-polkadot-v1alpha1-node.kb.io",
					"rules": []interface{}{
						map[string]interface{}{
							"apiGroups": []interface{}{
								"polkadot.kotal.io",
							},
							"apiVersions": []interface{}{
								"v1alpha1",
							},
							"operations": []interface{}{
								"CREATE",
								"UPDATE",
							},
							"resources": []interface{}{
								"nodes",
							},
						},
					},
					"sideEffects": "None",
				},
				map[string]interface{}{
					"admissionReviewVersions": []interface{}{
						"v1",
					},
					"clientConfig": map[string]interface{}{
						"service": map[string]interface{}{
							"name":      "webhook-service",
							"namespace": "kotal",
							"path":      "/mutate-stacks-kotal-io-v1alpha1-node",
						},
					},
					"failurePolicy": "Fail",
					"name":          "mutate-stacks-v1alpha1-node.kb.io",
					"rules": []interface{}{
						map[string]interface{}{
							"apiGroups": []interface{}{
								"stacks.kotal.io",
							},
							"apiVersions": []interface{}{
								"v1alpha1",
							},
							"operations": []interface{}{
								"CREATE",
								"UPDATE",
							},
							"resources": []interface{}{
								"nodes",
							},
						},
					},
					"sideEffects": "None",
				},
			},
		},
	}

	return resourceObj

}

func CreateManifestNamespace() (*unstructured.Unstructured) {
	var resourceObj = &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Namespace",
			"metadata": map[string]interface{}{
				"labels": map[string]interface{}{
					"control-plane": "controller-manager",
				},
				"name": "kotal",
			},
		},
	}

	return resourceObj

}

func CreateCRDNearKotalIo() (*unstructured.Unstructured) {
	var resourceObj = &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "apiextensions.k8s.io/v1",
			"kind":       "CustomResourceDefinition",
			"metadata": map[string]interface{}{
				"annotations": map[string]interface{}{
					"cert-manager.io/inject-ca-from":        "kotal/serving-cert",
					"controller-gen.kubebuilder.io/version": "v0.8.0",
				},
				"creationTimestamp": nil,
				"name":              "nodes.near.kotal.io",
			},
			"spec": map[string]interface{}{
				"group": "near.kotal.io",
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
								"jsonPath": ".spec.network",
								"name":     "Network",
								"type":     "string",
							},
							map[string]interface{}{
								"jsonPath": ".status.client",
								"name":     "Client",
								"type":     "string",
							},
							map[string]interface{}{
								"jsonPath": ".spec.validator",
								"name":     "Validator",
								"priority": 10,
								"type":     "boolean",
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
										"description": "NodeSpec defines the desired state of Node",
										"properties": map[string]interface{}{
											"archive": map[string]interface{}{
												"description": "Archive keeps old blocks in the storage",
												"type":        "boolean",
											},
											"bootnodes": map[string]interface{}{
												"description": "Bootnodes is array of boot nodes to bootstrap network from",
												"items": map[string]interface{}{
													"type": "string",
												},
												"type":                   "array",
												"x-kubernetes-list-type": "set",
											},
											"image": map[string]interface{}{
												"description": "Image is NEAR node client image",
												"type":        "string",
											},
											"minPeers": map[string]interface{}{
												"description": "MinPeers is minimum number of peers to start syncing/producing blocks",
												"type":        "integer",
											},
											"network": map[string]interface{}{
												"description": "Network is NEAR network to join and sync",
												"enum": []interface{}{
													"mainnet",
													"testnet",
													"betanet",
												},
												"type": "string",
											},
											"nodePrivateKeySecretName": map[string]interface{}{
												"description": "NodePrivateKeySecretName is the secret name holding node Ed25519 private key",
												"type":        "string",
											},
											"p2pPort": map[string]interface{}{
												"description": "P2PPort is p2p port",
												"type":        "integer",
											},
											"prometheusPort": map[string]interface{}{
												"description": "PrometheusPort is prometheus exporter port",
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
												"description": "RPC enables JSON-RPC server",
												"type":        "boolean",
											},
											"rpcPort": map[string]interface{}{
												"description": "RPCPort is JSON-RPC server listening port",
												"type":        "integer",
											},
											"telemetryURL": map[string]interface{}{
												"description": "TelemetryURL is telemetry service URL",
												"type":        "string",
											},
											"validatorSecretName": map[string]interface{}{
												"description": "ValidatorSecretName is the secret name holding node Ed25519 validator key",
												"type":        "string",
											},
										},
										"required": []interface{}{
											"network",
										},
										"type": "object",
									},
									"status": map[string]interface{}{
										"description": "NodeStatus defines the observed state of Node",
										"properties": map[string]interface{}{
											"client": map[string]interface{}{
												"type": "string",
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

func CreateManifestRole() (*unstructured.Unstructured) {
	var resourceObj = &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "rbac.authorization.k8s.io/v1",
			"kind":       "Role",
			"metadata": map[string]interface{}{
				"name":      "leader-election-role",
				"namespace": "kotal",
			},
			"rules": []interface{}{
				map[string]interface{}{
					"apiGroups": []interface{}{
						"",
					},
					"resources": []interface{}{
						"configmaps",
					},
					"verbs": []interface{}{
						"get",
						"list",
						"watch",
						"create",
						"update",
						"patch",
						"delete",
					},
				},
				map[string]interface{}{
					"apiGroups": []interface{}{
						"coordination.k8s.io",
					},
					"resources": []interface{}{
						"leases",
					},
					"verbs": []interface{}{
						"get",
						"list",
						"watch",
						"create",
						"update",
						"patch",
						"delete",
					},
				},
				map[string]interface{}{
					"apiGroups": []interface{}{
						"",
					},
					"resources": []interface{}{
						"events",
					},
					"verbs": []interface{}{
						"create",
						"patch",
					},
				},
			},
		},
	}

	return resourceObj

}

func CreateManifestServiceMetrics() (*unstructured.Unstructured) {
	var resourceObj = &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Service",
			"metadata": map[string]interface{}{
				"labels": map[string]interface{}{
					"control-plane": "controller-manager",
				},
				"name":      "controller-manager-metrics-service",
				"namespace": "kotal",
			},
			"spec": map[string]interface{}{
				"ports": []interface{}{
					map[string]interface{}{
						"name":       "https",
						"port":       8443,
						"protocol":   "TCP",
						"targetPort": "https",
					},
				},
				"selector": map[string]interface{}{
					"control-plane": "controller-manager",
				},
			},
		},
	}

	return resourceObj

}

func CreateManifestServiceWebhook() (*unstructured.Unstructured) {
	var resourceObj = &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Service",
			"metadata": map[string]interface{}{
				"name":      "webhook-service",
				"namespace": "kotal",
			},
			"spec": map[string]interface{}{
				"ports": []interface{}{
					map[string]interface{}{
						"port":       443,
						"targetPort": 9443,
					},
				},
				"selector": map[string]interface{}{
					"control-plane": "controller-manager",
				},
			},
		},
	}

	return resourceObj

}

func CreateManifestServiceAccount() (*unstructured.Unstructured) {
	var resourceObj = &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "ServiceAccount",
			"metadata": map[string]interface{}{
				"name":      "controller-manager",
				"namespace": "kotal",
			},
		},
	}

	return resourceObj

}

func CreateManifestValidatingWebhookConfiguration() (*unstructured.Unstructured) {
	var resourceObj = &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "admissionregistration.k8s.io/v1",
			"kind":       "ValidatingWebhookConfiguration",
			"metadata": map[string]interface{}{
				"annotations": map[string]interface{}{
					"cert-manager.io/inject-ca-from": "kotal/serving-cert",
				},
				"creationTimestamp": nil,
				"name":              "validating-webhook-configuration",
			},
			"webhooks": []interface{}{
				map[string]interface{}{
					"admissionReviewVersions": []interface{}{
						"v1",
					},
					"clientConfig": map[string]interface{}{
						"service": map[string]interface{}{
							"name":      "webhook-service",
							"namespace": "kotal",
							"path":      "/validate-aptos-kotal-io-v1alpha1-node",
						},
					},
					"failurePolicy": "Fail",
					"name":          "validate-aptos-v1alpha1-node.kb.io",
					"rules": []interface{}{
						map[string]interface{}{
							"apiGroups": []interface{}{
								"aptos.kotal.io",
							},
							"apiVersions": []interface{}{
								"v1alpha1",
							},
							"operations": []interface{}{
								"CREATE",
								"UPDATE",
							},
							"resources": []interface{}{
								"nodes",
							},
						},
					},
					"sideEffects": "None",
				},
				map[string]interface{}{
					"admissionReviewVersions": []interface{}{
						"v1",
					},
					"clientConfig": map[string]interface{}{
						"service": map[string]interface{}{
							"name":      "webhook-service",
							"namespace": "kotal",
							"path":      "/validate-bitcoin-kotal-io-v1alpha1-node",
						},
					},
					"failurePolicy": "Fail",
					"name":          "validate-bitcoin-v1alpha1-node.kb.io",
					"rules": []interface{}{
						map[string]interface{}{
							"apiGroups": []interface{}{
								"bitcoin.kotal.io",
							},
							"apiVersions": []interface{}{
								"v1alpha1",
							},
							"operations": []interface{}{
								"CREATE",
								"UPDATE",
							},
							"resources": []interface{}{
								"nodes",
							},
						},
					},
					"sideEffects": "None",
				},
				map[string]interface{}{
					"admissionReviewVersions": []interface{}{
						"v1",
					},
					"clientConfig": map[string]interface{}{
						"service": map[string]interface{}{
							"name":      "webhook-service",
							"namespace": "kotal",
							"path":      "/validate-chainlink-kotal-io-v1alpha1-node",
						},
					},
					"failurePolicy": "Fail",
					"name":          "validate-chainlink-v1alpha1-node.kb.io",
					"rules": []interface{}{
						map[string]interface{}{
							"apiGroups": []interface{}{
								"chainlink.kotal.io",
							},
							"apiVersions": []interface{}{
								"v1alpha1",
							},
							"operations": []interface{}{
								"CREATE",
								"UPDATE",
							},
							"resources": []interface{}{
								"nodes",
							},
						},
					},
					"sideEffects": "None",
				},
				map[string]interface{}{
					"admissionReviewVersions": []interface{}{
						"v1",
					},
					"clientConfig": map[string]interface{}{
						"service": map[string]interface{}{
							"name":      "webhook-service",
							"namespace": "kotal",
							"path":      "/validate-ethereum-kotal-io-v1alpha1-node",
						},
					},
					"failurePolicy": "Fail",
					"name":          "validate-ethereum-v1alpha1-node.kb.io",
					"rules": []interface{}{
						map[string]interface{}{
							"apiGroups": []interface{}{
								"ethereum.kotal.io",
							},
							"apiVersions": []interface{}{
								"v1alpha1",
							},
							"operations": []interface{}{
								"CREATE",
								"UPDATE",
							},
							"resources": []interface{}{
								"nodes",
							},
						},
					},
					"sideEffects": "None",
				},
				map[string]interface{}{
					"admissionReviewVersions": []interface{}{
						"v1",
					},
					"clientConfig": map[string]interface{}{
						"service": map[string]interface{}{
							"name":      "webhook-service",
							"namespace": "kotal",
							"path":      "/validate-ethereum2-kotal-io-v1alpha1-beaconnode",
						},
					},
					"failurePolicy": "Fail",
					"name":          "validate-ethereum2-v1alpha1-beaconnode.kb.io",
					"rules": []interface{}{
						map[string]interface{}{
							"apiGroups": []interface{}{
								"ethereum2.kotal.io",
							},
							"apiVersions": []interface{}{
								"v1alpha1",
							},
							"operations": []interface{}{
								"CREATE",
								"UPDATE",
							},
							"resources": []interface{}{
								"beaconnodes",
							},
						},
					},
					"sideEffects": "None",
				},
				map[string]interface{}{
					"admissionReviewVersions": []interface{}{
						"v1",
					},
					"clientConfig": map[string]interface{}{
						"service": map[string]interface{}{
							"name":      "webhook-service",
							"namespace": "kotal",
							"path":      "/validate-ethereum2-kotal-io-v1alpha1-validator",
						},
					},
					"failurePolicy": "Fail",
					"name":          "validate-ethereum2-v1alpha1-validator.kb.io",
					"rules": []interface{}{
						map[string]interface{}{
							"apiGroups": []interface{}{
								"ethereum2.kotal.io",
							},
							"apiVersions": []interface{}{
								"v1alpha1",
							},
							"operations": []interface{}{
								"CREATE",
								"UPDATE",
							},
							"resources": []interface{}{
								"validators",
							},
						},
					},
					"sideEffects": "None",
				},
				map[string]interface{}{
					"admissionReviewVersions": []interface{}{
						"v1",
					},
					"clientConfig": map[string]interface{}{
						"service": map[string]interface{}{
							"name":      "webhook-service",
							"namespace": "kotal",
							"path":      "/validate-filecoin-kotal-io-v1alpha1-node",
						},
					},
					"failurePolicy": "Fail",
					"name":          "validate-filecoin-v1alpha1-node.kb.io",
					"rules": []interface{}{
						map[string]interface{}{
							"apiGroups": []interface{}{
								"filecoin.kotal.io",
							},
							"apiVersions": []interface{}{
								"v1alpha1",
							},
							"operations": []interface{}{
								"CREATE",
								"UPDATE",
							},
							"resources": []interface{}{
								"nodes",
							},
						},
					},
					"sideEffects": "None",
				},
				map[string]interface{}{
					"admissionReviewVersions": []interface{}{
						"v1",
					},
					"clientConfig": map[string]interface{}{
						"service": map[string]interface{}{
							"name":      "webhook-service",
							"namespace": "kotal",
							"path":      "/validate-ipfs-kotal-io-v1alpha1-clusterpeer",
						},
					},
					"failurePolicy": "Fail",
					"name":          "validate-ipfs-v1alpha1-clusterpeer.kb.io",
					"rules": []interface{}{
						map[string]interface{}{
							"apiGroups": []interface{}{
								"ipfs.kotal.io",
							},
							"apiVersions": []interface{}{
								"v1alpha1",
							},
							"operations": []interface{}{
								"CREATE",
								"UPDATE",
							},
							"resources": []interface{}{
								"clusterpeers",
							},
						},
					},
					"sideEffects": "None",
				},
				map[string]interface{}{
					"admissionReviewVersions": []interface{}{
						"v1",
					},
					"clientConfig": map[string]interface{}{
						"service": map[string]interface{}{
							"name":      "webhook-service",
							"namespace": "kotal",
							"path":      "/validate-ipfs-kotal-io-v1alpha1-peer",
						},
					},
					"failurePolicy": "Fail",
					"name":          "validate-ipfs-v1alpha1-peer.kb.io",
					"rules": []interface{}{
						map[string]interface{}{
							"apiGroups": []interface{}{
								"ipfs.kotal.io",
							},
							"apiVersions": []interface{}{
								"v1alpha1",
							},
							"operations": []interface{}{
								"CREATE",
								"UPDATE",
							},
							"resources": []interface{}{
								"peers",
							},
						},
					},
					"sideEffects": "None",
				},
				map[string]interface{}{
					"admissionReviewVersions": []interface{}{
						"v1",
					},
					"clientConfig": map[string]interface{}{
						"service": map[string]interface{}{
							"name":      "webhook-service",
							"namespace": "kotal",
							"path":      "/validate-near-kotal-io-v1alpha1-node",
						},
					},
					"failurePolicy": "Fail",
					"name":          "validate-near-v1alpha1-node.kb.io",
					"rules": []interface{}{
						map[string]interface{}{
							"apiGroups": []interface{}{
								"near.kotal.io",
							},
							"apiVersions": []interface{}{
								"v1alpha1",
							},
							"operations": []interface{}{
								"CREATE",
								"UPDATE",
							},
							"resources": []interface{}{
								"nodes",
							},
						},
					},
					"sideEffects": "None",
				},
				map[string]interface{}{
					"admissionReviewVersions": []interface{}{
						"v1",
					},
					"clientConfig": map[string]interface{}{
						"service": map[string]interface{}{
							"name":      "webhook-service",
							"namespace": "kotal",
							"path":      "/validate-polkadot-kotal-io-v1alpha1-node",
						},
					},
					"failurePolicy": "Fail",
					"name":          "validate-polkadot-v1alpha1-node.kb.io",
					"rules": []interface{}{
						map[string]interface{}{
							"apiGroups": []interface{}{
								"polkadot.kotal.io",
							},
							"apiVersions": []interface{}{
								"v1alpha1",
							},
							"operations": []interface{}{
								"CREATE",
								"UPDATE",
							},
							"resources": []interface{}{
								"nodes",
							},
						},
					},
					"sideEffects": "None",
				},
				map[string]interface{}{
					"admissionReviewVersions": []interface{}{
						"v1",
					},
					"clientConfig": map[string]interface{}{
						"service": map[string]interface{}{
							"name":      "webhook-service",
							"namespace": "kotal",
							"path":      "/validate-stacks-kotal-io-v1alpha1-node",
						},
					},
					"failurePolicy": "Fail",
					"name":          "validate-stacks-v1alpha1-node.kb.io",
					"rules": []interface{}{
						map[string]interface{}{
							"apiGroups": []interface{}{
								"stacks.kotal.io",
							},
							"apiVersions": []interface{}{
								"v1alpha1",
							},
							"operations": []interface{}{
								"CREATE",
								"UPDATE",
							},
							"resources": []interface{}{
								"nodes",
							},
						},
					},
					"sideEffects": "None",
				},
			},
		},
	}

	return resourceObj

}

func CreateManifestCertificate() (*unstructured.Unstructured) {
	var resourceObj = &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "cert-manager.io/v1",
			"kind":       "Certificate",
			"metadata": map[string]interface{}{
				"name":      "serving-cert",
				"namespace": "kotal",
			},
			"spec": map[string]interface{}{
				"dnsNames": []interface{}{
					"webhook-service.kotal.svc",
					"webhook-service.kotal.svc.cluster.local",
				},
				"issuerRef": map[string]interface{}{
					"kind": "Issuer",
					"name": "selfsigned-issuer",
				},
				"secretName": "webhook-server-cert",
			},
		},
	}

	return resourceObj

}

func CreateCRDIpfsPeerKotalIo() (*unstructured.Unstructured) {

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
				"name":              "peers.ipfs.kotal.io",
			},
			"spec": map[string]interface{}{
				"group": "ipfs.kotal.io",
				"names": map[string]interface{}{
					"kind":     "Peer",
					"listKind": "PeerList",
					"plural":   "peers",
					"singular": "peer",
				},
				"scope": "Namespaced",
				"versions": []interface{}{
					map[string]interface{}{
						"additionalPrinterColumns": []interface{}{
							map[string]interface{}{
								"jsonPath": ".status.client",
								"name":     "Client",
								"type":     "string",
							},
						},
						"name": "v1alpha1",
						"schema": map[string]interface{}{
							"openAPIV3Schema": map[string]interface{}{
								"description": "Peer is the Schema for the peers API",
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
										"description": "PeerSpec defines the desired state of Peer",
										"properties": map[string]interface{}{
											"api": map[string]interface{}{
												"description": "API enables API server",
												"type":        "boolean",
											},
											"apiPort": map[string]interface{}{
												"description": "APIPort is api server port",
												"type":        "integer",
											},
											"gateway": map[string]interface{}{
												"description": "Gateway enables IPFS gateway server",
												"type":        "boolean",
											},
											"gatewayPort": map[string]interface{}{
												"description": "GatewayPort is local gateway port",
												"type":        "integer",
											},
											"image": map[string]interface{}{
												"description": "Image is ipfs peer client image",
												"type":        "string",
											},
											"initProfiles": map[string]interface{}{
												"description": "InitProfiles is the intial profiles to apply during",
												"items": map[string]interface{}{
													"description": "Profile is ipfs configuration",
													"enum": []interface{}{
														"server",
														"randomports",
														"default-datastore",
														"local-discovery",
														"test",
														"default-networking",
														"flatfs",
														"badgerds",
														"lowpower",
													},
													"type": "string",
												},
												"type":                   "array",
												"x-kubernetes-list-type": "set",
											},
											"logging": map[string]interface{}{
												"description": "Logging is logging verboisty level",
												"enum": []interface{}{
													"error",
													"warn",
													"info",
													"debug",
													"notice",
												},
												"type": "string",
											},
											"profiles": map[string]interface{}{
												"description": "Profiles is the configuration profiles to apply after peer initialization",
												"items": map[string]interface{}{
													"description": "Profile is ipfs configuration",
													"enum": []interface{}{
														"server",
														"randomports",
														"default-datastore",
														"local-discovery",
														"test",
														"default-networking",
														"flatfs",
														"badgerds",
														"lowpower",
													},
													"type": "string",
												},
												"type":                   "array",
												"x-kubernetes-list-type": "set",
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
											"routing": map[string]interface{}{
												"description": "Routing is the content routing mechanism",
												"enum": []interface{}{
													"none",
													"dht",
													"dhtclient",
													"dhtserver",
												},
												"type": "string",
											},
											"swarmKeySecretName": map[string]interface{}{
												"description": "SwarmKeySecretName is the k8s secret holding swarm key",
												"type":        "string",
											},
										},
										"type": "object",
									},
									"status": map[string]interface{}{
										"description": "PeerStatus defines the observed state of Peer",
										"properties": map[string]interface{}{
											"client": map[string]interface{}{
												"type": "string",
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

func CreateCRDPolkadotKotalIo() (*unstructured.Unstructured) {

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
				"name":              "nodes.polkadot.kotal.io",
			},
			"spec": map[string]interface{}{
				"group": "polkadot.kotal.io",
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
								"jsonPath": ".spec.network",
								"name":     "Network",
								"type":     "string",
							},
							map[string]interface{}{
								"jsonPath": ".spec.validator",
								"name":     "Validator",
								"type":     "boolean",
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
										"description": "NodeSpec defines the desired state of Node",
										"properties": map[string]interface{}{
											"corsDomains": map[string]interface{}{
												"description": "CORSDomains is browser origins allowed to access the JSON-RPC HTTP and WS servers",
												"items": map[string]interface{}{
													"type": "string",
												},
												"type":                   "array",
												"x-kubernetes-list-type": "set",
											},
											"image": map[string]interface{}{
												"description": "Image is Polkadot node client image",
												"type":        "string",
											},
											"logging": map[string]interface{}{
												"description": "Logging is logging verboisty level",
												"enum": []interface{}{
													"error",
													"warn",
													"info",
													"debug",
													"trace",
												},
												"type": "string",
											},
											"network": map[string]interface{}{
												"description": "Network is the polkadot network/chain to join",
												"type":        "string",
											},
											"nodePrivateKeySecretName": map[string]interface{}{
												"description": "NodePrivateKeySecretName is the secret name holding node Ed25519 private key",
												"type":        "string",
											},
											"p2pPort": map[string]interface{}{
												"description": "P2PPort is p2p protocol tcp port",
												"type":        "integer",
											},
											"prometheus": map[string]interface{}{
												"description": "Prometheus exposes a prometheus exporter endpoint.",
												"type":        "boolean",
											},
											"prometheusPort": map[string]interface{}{
												"description": "PrometheusPort is prometheus exporter port",
												"type":        "integer",
											},
											"pruning": map[string]interface{}{
												"description": "Pruning keeps recent or all blocks",
												"type":        "boolean",
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
											"retainedBlocks": map[string]interface{}{
												"description": "RetainedBlocks is the number of blocks to keep state for",
												"type":        "integer",
											},
											"rpc": map[string]interface{}{
												"description": "RPC enables JSON-RPC server",
												"type":        "boolean",
											},
											"rpcPort": map[string]interface{}{
												"description": "RPCPort is JSON-RPC server port",
												"type":        "integer",
											},
											"syncMode": map[string]interface{}{
												"description": "SyncMode is the blockchain synchronization mode",
												"enum": []interface{}{
													"fast",
													"full",
												},
												"type": "string",
											},
											"telemetry": map[string]interface{}{
												"description": "Telemetry enables connecting to telemetry server",
												"type":        "boolean",
											},
											"telemetryURL": map[string]interface{}{
												"description": "TelemetryURL is telemetry service URL",
												"type":        "string",
											},
											"validator": map[string]interface{}{
												"description": "Validator enables validator mode",
												"type":        "boolean",
											},
											"ws": map[string]interface{}{
												"description": "WS enables Websocket server",
												"type":        "boolean",
											},
											"wsPort": map[string]interface{}{
												"description": "WSPort is Websocket server port",
												"type":        "integer",
											},
										},
										"required": []interface{}{
											"network",
										},
										"type": "object",
									},
									"status": map[string]interface{}{
										"description": "NodeStatus defines the observed state of Node",
										"type":        "object",
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


func CreateCRDStacksKotalIo() (*unstructured.Unstructured) {
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
				"name":              "nodes.stacks.kotal.io",
			},
			"spec": map[string]interface{}{
				"group": "stacks.kotal.io",
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
								"jsonPath": ".spec.network",
								"name":     "Network",
								"type":     "string",
							},
							map[string]interface{}{
								"jsonPath": ".status.client",
								"name":     "Client",
								"type":     "string",
							},
							map[string]interface{}{
								"jsonPath": ".spec.miner",
								"name":     "Miner",
								"type":     "boolean",
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
										"description": "NodeSpec defines the desired state of Node",
										"properties": map[string]interface{}{
											"bitcoinNode": map[string]interface{}{
												"description": "BitcoinNode is Bitcoin node",
												"properties": map[string]interface{}{
													"endpoint": map[string]interface{}{
														"description": "Endpoint is bitcoin node JSON-RPC endpoint",
														"type":        "string",
													},
													"p2pPort": map[string]interface{}{
														"description": "P2pPort is bitcoin node p2p port",
														"type":        "integer",
													},
													"rpcPasswordSecretName": map[string]interface{}{
														"description": "RpcPasswordSecretName is k8s secret name holding bitcoin node JSON-RPC password",
														"type":        "string",
													},
													"rpcPort": map[string]interface{}{
														"description": "RpcPort is bitcoin node JSON-RPC port",
														"type":        "integer",
													},
													"rpcUsername": map[string]interface{}{
														"description": "RpcUsername is bitcoin node JSON-RPC username",
														"type":        "string",
													},
												},
												"required": []interface{}{
													"endpoint",
													"p2pPort",
													"rpcPasswordSecretName",
													"rpcPort",
													"rpcUsername",
												},
												"type": "object",
											},
											"image": map[string]interface{}{
												"description": "Image is Stacks node client image",
												"type":        "string",
											},
											"mineMicroblocks": map[string]interface{}{
												"description": "MineMicroblocks mines Stacks micro blocks",
												"type":        "boolean",
											},
											"miner": map[string]interface{}{
												"description": "Miner enables mining",
												"type":        "boolean",
											},
											"network": map[string]interface{}{
												"description": "Network is stacks network",
												"enum": []interface{}{
													"mainnet",
													"testnet",
												},
												"type": "string",
											},
											"nodePrivateKeySecretName": map[string]interface{}{
												"description": "NodePrivateKeySecretName is k8s secret holding node private key",
												"type":        "string",
											},
											"p2pPort": map[string]interface{}{
												"description": "P2PPort is p2p bind port",
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
												"description": "RPC enables JSON-RPC server",
												"type":        "boolean",
											},
											"rpcPort": map[string]interface{}{
												"description": "RPCPort is JSON-RPC server port",
												"type":        "integer",
											},
											"seedPrivateKeySecretName": map[string]interface{}{
												"description": "SeedPrivateKeySecretName is k8s secret holding seed private key used for mining",
												"type":        "string",
											},
										},
										"required": []interface{}{
											"bitcoinNode",
											"network",
										},
										"type": "object",
									},
									"status": map[string]interface{}{
										"description": "NodeStatus defines the observed state of Node",
										"properties": map[string]interface{}{
											"client": map[string]interface{}{
												"type": "string",
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



func CreateCRDValidatorKotalIo() (*unstructured.Unstructured) {
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
				"name":              "validators.ethereum2.kotal.io",
			},
			"spec": map[string]interface{}{
				"group": "ethereum2.kotal.io",
				"names": map[string]interface{}{
					"kind":     "Validator",
					"listKind": "ValidatorList",
					"plural":   "validators",
					"singular": "validator",
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
								"description": "Validator is the Schema for the validators API",
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
										"description": "ValidatorSpec defines the desired state of Validator",
										"properties": map[string]interface{}{
											"beaconEndpoints": map[string]interface{}{
												"description": "BeaconEndpoints is beacon node endpoints",
												"items": map[string]interface{}{
													"type": "string",
												},
												"minItems":               1,
												"type":                   "array",
												"x-kubernetes-list-type": "set",
											},
											"certSecretName": map[string]interface{}{
												"description": "CertSecretName is k8s secret name that holds tls.crt",
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
											"feeRecipient": map[string]interface{}{
												"description": "FeeRecipient is ethereum address collecting transaction fees",
												"pattern":     "^0[xX][0-9a-fA-F]{40}$",
												"type":        "string",
											},
											"graffiti": map[string]interface{}{
												"description": "Graffiti is the text to include in proposed blocks",
												"type":        "string",
											},
											"image": map[string]interface{}{
												"description": "Image is Ethereum 2.0 validator client image",
												"type":        "string",
											},
											"keystores": map[string]interface{}{
												"description": "Keystores is a list of Validator keystores",
												"items": map[string]interface{}{
													"description": "Keystore is Ethereum 2.0 validator EIP-2335 BLS12-381 keystore https://eips.ethereum.org/EIPS/eip-2335",
													"properties": map[string]interface{}{
														"publicKey": map[string]interface{}{
															"description": "PublicKey is the validator public key in hexadecimal",
															"pattern":     "^0[xX][0-9a-fA-F]{96}$",
															"type":        "string",
														},
														"secretName": map[string]interface{}{
															"description": "SecretName is the kubernetes secret holding [keystore] and [password]",
															"type":        "string",
														},
													},
													"required": []interface{}{
														"secretName",
													},
													"type": "object",
												},
												"minItems": 1,
												"type":     "array",
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
												"description": "Network is the network this validator is validating blocks for",
												"type":        "string",
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
											"walletPasswordSecret": map[string]interface{}{
												"description": "WalletPasswordSecret is wallet password secret",
												"type":        "string",
											},
										},
										"required": []interface{}{
											"beaconEndpoints",
											"client",
											"keystores",
											"network",
										},
										"type": "object",
									},
									"status": map[string]interface{}{
										"description": "ValidatorStatus defines the observed state of Validator",
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