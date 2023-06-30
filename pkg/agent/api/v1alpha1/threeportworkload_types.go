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

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var (
	// Resource is the name of the ThreeportWorkload resource
	ThreeportWorkloadResource = "threeportworkloads"

	// Kind is the name of the the ThreeportWorkload kind
	ThreeportWorkloadKind = "ThreeportWorkload"

	// GroupVersionResource is the group version resource used with dynamic
	// clients
	ThreeportWorkloadGVR = schema.GroupVersionResource{
		Group:    GroupVersion.Group,
		Version:  GroupVersion.Version,
		Resource: ThreeportWorkloadResource,
	}
)

// ThreeportWorkloadSpec defines the desired state of ThreeportWorkload
type ThreeportWorkloadSpec struct {
	// WorkloadInstance is the unique ID for a threeport object that represents
	// a deployed instance of a workload.
	WorkloadInstanceID uint `json:"workloadInstanceId,omitempty"`

	// WorkloadResources is a slice of WorkloadResource objects.
	WorkloadResourceInstances []WorkloadResourceInstance `json:"workloadResourceInstances,omitempty"`
}

// WorkloadResource is a Kubernetes resource that should be watched and reported
// upon by the threeport agent.
type WorkloadResourceInstance struct {
	Name        string `json:"name,omitempty"`
	Namespace   string `json:"namespace,omitempty"`
	Group       string `json:"group,omitempty"`
	Version     string `json:"version,omitempty"`
	Kind        string `json:"kind,omitempty"`
	ThreeportID uint   `json:"threeportID,omitempty"`
}

// ThreeportWorkloadStatus defines the observed state of ThreeportWorkload
type ThreeportWorkloadStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:resource:scope=Cluster

// ThreeportWorkload is the Schema for the threeportworkloads API
type ThreeportWorkload struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ThreeportWorkloadSpec   `json:"spec,omitempty"`
	Status ThreeportWorkloadStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// ThreeportWorkloadList contains a list of ThreeportWorkload
type ThreeportWorkloadList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ThreeportWorkload `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ThreeportWorkload{}, &ThreeportWorkloadList{})
}
