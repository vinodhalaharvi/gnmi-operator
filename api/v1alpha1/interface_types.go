/*
Copyright 2026.

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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// InterfaceSpec defines the desired state of Interface.
type InterfaceSpec struct {
	// DeviceRef references the Device in the same namespace this interface lives on.
	// +kubebuilder:validation:XValidation:rule="self.name != ''",message="deviceRef.name must not be empty"
	DeviceRef corev1.LocalObjectReference `json:"deviceRef"`

	// Name is the interface name on the device (e.g. eth1).
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// Enabled desired administrative state of the interface.
	// A nil value means "do not manage" — distinct from an explicit false.
	// +optional
	Enabled *bool `json:"enabled,omitempty"`

	// MTU desired MTU in bytes.
	// A nil value means "do not manage" — distinct from an explicit value.
	// +kubebuilder:validation:Minimum=68
	// +kubebuilder:validation:Maximum=65535
	// +optional
	MTU *uint16 `json:"mtu,omitempty"`

	// Description is a free-form description propagated to the interface.
	// +optional
	Description string `json:"description,omitempty"`
}

// InterfaceStatus defines the observed state of Interface.
type InterfaceStatus struct {
	// Conditions represent the latest observations of the Interface's state.
	// +optional
	// +patchMergeKey=type
	// +patchStrategy=merge
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type"`

	// ObservedMTU is the MTU last read from the device.
	// +optional
	ObservedMTU *uint16 `json:"observedMTU,omitempty"`

	// ObservedEnabled is the administrative state last read from the device.
	// +optional
	ObservedEnabled *bool `json:"observedEnabled,omitempty"`

	// LastSyncTime is the last time the operator successfully reconciled this interface.
	// +optional
	LastSyncTime *metav1.Time `json:"lastSyncTime,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Device",type=string,JSONPath=`.spec.deviceRef.name`
// +kubebuilder:printcolumn:name="Name",type=string,JSONPath=`.spec.name`
// +kubebuilder:printcolumn:name="MTU",type=integer,JSONPath=`.spec.mtu`
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`

// Interface is the Schema for the interfaces API.
type Interface struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   InterfaceSpec   `json:"spec,omitempty"`
	Status InterfaceStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// InterfaceList contains a list of Interface.
type InterfaceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Interface `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Interface{}, &InterfaceList{})
}
