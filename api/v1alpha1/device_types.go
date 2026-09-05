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

// DeviceSpec defines the desired state of Device.
type DeviceSpec struct {
	// Address is the host or IP the gNMI target listens on.
	// +kubebuilder:validation:MinLength=1
	Address string `json:"address"`

	// Port is the gNMI target port.
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	// +kubebuilder:default=9339
	// +optional
	Port int32 `json:"port,omitempty"`

	// TLSSecretRef names a Secret holding ca.crt and optionally tls.crt/tls.key
	// used to dial the gNMI target.
	// +optional
	TLSSecretRef *corev1.LocalObjectReference `json:"tlsSecretRef,omitempty"`

	// Insecure disables TLS entirely. Lab use only.
	// +optional
	Insecure bool `json:"insecure,omitempty"`

	// ServerName overrides the hostname verified in the target certificate,
	// which is useful when Address is an IP.
	// +optional
	ServerName string `json:"serverName,omitempty"`
}

// DeviceStatus defines the observed state of Device.
type DeviceStatus struct {
	// Conditions represent the latest observations of the Device's state.
	// +optional
	// +patchMergeKey=type
	// +patchStrategy=merge
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type"`

	// GNMIVersion is the gNMI version reported by the target's Capabilities response.
	// +optional
	GNMIVersion string `json:"gnmiVersion,omitempty"`

	// LastContactTime is the last time the operator successfully reached the target.
	// +optional
	LastContactTime *metav1.Time `json:"lastContactTime,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Address",type=string,JSONPath=`.spec.address`
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`

// Device is the Schema for the devices API.
type Device struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DeviceSpec   `json:"spec,omitempty"`
	Status DeviceStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// DeviceList contains a list of Device.
type DeviceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Device `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Device{}, &DeviceList{})
}
