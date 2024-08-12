/*
Copyright 2024.

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
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type ResourceName string

const (
	// ResourceCPU CPU, in cores. (500m = .5 cores)
	ResourceCPU ResourceName = "cpu"
	// ResourceMemory Memory, in bytes. (500Gi = 500GiB = 500 * 1024 * 1024 * 1024)
	ResourceMemory ResourceName = "memory"
)

type ResourceList map[ResourceName]resource.Quantity

type RuntimeRef struct {
	// +kubebuilder:validation:Required
	Name string `json:"name"`
}

type NetworkSpec struct {
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Enum=NodePort;Tailnet
	Type NetworkType `json:"type"`
}

// DevboxSpec defines the desired state of Devbox
type DevboxSpec struct {
	// +kubebuilder:validation:Required
	Resource ResourceList `json:"resource"`
	// +kubebuilder:validation:Required
	RuntimeRef RuntimeRef `json:"runtimeRef"`
	// +kubebuilder:validation:Required
	NetworkSpec NetworkSpec `json:"network"`
}

type DevboxPhase string

const (
	// DevboxPhasePending means the Devbox is pending
	DevboxPhasePending DevboxPhase = "Pending"
	// DevboxPhaseRunning means the Devbox is running
	DevboxPhaseRunning DevboxPhase = "Running"
	// DevboxPhaseStopped means the Devbox is stopped
	DevboxPhaseStopped DevboxPhase = "Stopped"
	// DevboxPhaseFailed means the Devbox has failed
	DevboxPhaseFailed DevboxPhase = "Failed"
	// DevboxPhaseUnknown means the Devbox is in an unknown state
	DevboxPhaseUnknown DevboxPhase = "Unknown"
)

type NetworkType string

const (
	NetworkTypeNodePort NetworkType = "NodePort"
	NetworkTypeTailnet  NetworkType = "Tailnet"
)

type NetworkStatus struct {
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Enum=NodePort;Tailnet
	Type        NetworkType `json:"type"`
	NodePort    int32       `json:"nodePort"`
	ServiceName string      `json:"serviceName"`
	// todo TailNet
	TailNet string `json:"tailnet"`
}

type CommitStatus struct {
	CommitID       string `json:"commitID"`
	CommitImageRef string `json:"commitImageRef"`
}

// DevboxStatus defines the observed state of Devbox
type DevboxStatus struct {
	Phase   DevboxPhase   `json:"phase"`
	Network NetworkStatus `json:"network"`
	Commit  CommitStatus  `json:"commit"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// Devbox is the Schema for the devboxes API
type Devbox struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DevboxSpec   `json:"spec,omitempty"`
	Status DevboxStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// DevboxList contains a list of Devbox
type DevboxList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Devbox `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Devbox{}, &DevboxList{})
}
