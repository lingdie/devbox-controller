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

// DevBoxReleaseSpec defines the desired state of DevBoxRelease
type DevBoxReleaseSpec struct {
	// +kubebuilder:validation:Required
	UserInfo       UserInfo       `json:"userInfo,omitempty"`
	RepositoryInfo RepositoryInfo `json:"repositoryInfo,omitempty"`
	Notes          string         `json:"notes,omitempty"`
}

type UserInfo struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type RepositoryInfo struct {
	Name   string `json:"name"`
	Image  string `json:"image"`
	OldTag string `json:"oldTag,omitempty"`
	NewTag string `json:"newTag,omitempty"`
}

type DevboxReleasePhase string

const (
	// DevboxReleasePhaseTagged means the Devbox has been tagged
	DevboxReleasePhaseTagged DevboxReleasePhase = "Tagged"
	// DevboxReleasePhaseNotTagged means the Devbox has not been tagged
	DevboxReleasePhaseNotTagged DevboxReleasePhase = "NotTagged"
	// DevboxReleasePhaseFailed means the Devbox has not been tagged
	DevboxReleasePhaseFailed DevboxReleasePhase = "Failed"
)

// DevBoxReleaseStatus defines the observed state of DevBoxRelease
type DevBoxReleaseStatus struct {
	Phase DevboxReleasePhase `json:"phase"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// DevBoxRelease is the Schema for the devboxreleases API
type DevBoxRelease struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DevBoxReleaseSpec   `json:"spec,omitempty"`
	Status DevBoxReleaseStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// DevBoxReleaseList contains a list of DevBoxRelease
type DevBoxReleaseList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DevBoxRelease `json:"items"`
}

func init() {
	SchemeBuilder.Register(&DevBoxRelease{}, &DevBoxReleaseList{})
}
