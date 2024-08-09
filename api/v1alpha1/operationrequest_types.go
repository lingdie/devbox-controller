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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// OperationRequestSpec defines the desired state of OperationRequest
type OperationRequestSpec struct {
	DevBoxName string `json:"devBoxName"`
	// +kubebuilder:validation:Enum=Grant;Update;Deprive
	Action ActionType `json:"action,omitempty"`
}

type ActionType string

const (
	Grant   ActionType = "Grant"
	Update  ActionType = "Update"
	Deprive ActionType = "Deprive"
)

type RoleType string

const (
	OwnerRoleType     RoleType = "Owner"
	ManagerRoleType   RoleType = "Manager"
	DeveloperRoleType RoleType = "Developer"
)

// OperationRequestStatus defines the observed state of OperationRequest
type OperationRequestStatus struct {

	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	//+kubebuilder:default:=Pending
	//+kubebuilder:validation:Enum=Pending;Processing;Completed;Failed
	Phase RequestPhase `json:"phase,omitempty"`
}

type RequestPhase string

// These are the valid phases of node.
const (
	RequestPending    RequestPhase = "Pending"
	RequestProcessing RequestPhase = "Processing"
	RequestCompleted  RequestPhase = "Completed"
	RequestFailed     RequestPhase = "Failed"
)

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// OperationRequest is the Schema for the operationrequests API
type OperationRequest struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   OperationRequestSpec   `json:"spec,omitempty"`
	Status OperationRequestStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// OperationRequestList contains a list of OperationRequest
type OperationRequestList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []OperationRequest `json:"items"`
}

func init() {
	SchemeBuilder.Register(&OperationRequest{}, &OperationRequestList{})
}
