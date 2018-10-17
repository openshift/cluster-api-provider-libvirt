/*
Copyright 2018 “The.

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

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// LibvirtMachineProviderConfig is the Schema for the libvirtmachineproviderconfigs API
// +k8s:openapi-gen=true
type LibvirtMachineProviderConfig struct {
	metav1.TypeMeta          `json:",inline"`
	metav1.ObjectMeta        `json:"metadata,omitempty"`
	DomainMemory             int
	DomainVcpu               int
	IgnKey                   string
	CloudInit                *CloudInit
	Volume                   *Volume
	NetworkInterfaceName     string
	NetworkInterfaceHostname string
	NetworkInterfaceAddress  string
	NetworkUUID              string
	Autostart                bool
	URI                      string
}

// CloudInit contains location of user data to be run during bootstrapping
// with ISO image with a cloud-init file running the user data
type CloudInit struct {
	// UserDataSecret requires ISOImagePath to be set
	UserDataSecret string
	// ISOImagePath is path to ISO image with cloud-init
	ISOImagePath string
}

// Volume contains the info for the actuator to create a volume
type Volume struct {
	PoolName     string
	BaseVolumeID string
	VolumeName   string
}

// LibvirtMachineProviderStatus is the type that will be embedded in a Machine.Status.ProviderStatus field.
// It contains Libvirt-specific status information.
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type LibvirtMachineProviderStatus struct {
	metav1.TypeMeta `json:",inline"`

	// InstanceID is the instance ID of the machine created in Libvirt
	InstanceID *string `json:"instanceID"`

	// InstanceState is the state of the Libvirt instance for this machine
	InstanceState *string `json:"instanceState"`

	// Conditions is a set of conditions associated with the Machine to indicate
	// errors or other status
	Conditions []LibvirtMachineProviderCondition `json:"conditions"`
}

// LibvirtMachineProviderConditionType is a valid value for LibvirtMachineProviderCondition.Type
type LibvirtMachineProviderConditionType string

// Valid conditions for an Libvirt machine instance
const (
	// MachineCreated indicates whether the machine has been created or not. If not,
	// it should include a reason and message for the failure.
	MachineCreated LibvirtMachineProviderConditionType = "MachineCreated"
)

// LibvirtMachineProviderCondition is a condition in a LibvirtMachineProviderStatus
type LibvirtMachineProviderCondition struct {
	// Type is the type of the condition.
	Type LibvirtMachineProviderConditionType `json:"type"`
	// Status is the status of the condition.
	Status corev1.ConditionStatus `json:"status"`
	// LastProbeTime is the last time we probed the condition.
	// +optional
	LastProbeTime metav1.Time `json:"lastProbeTime"`
	// LastTransitionTime is the last time the condition transitioned from one status to another.
	// +optional
	LastTransitionTime metav1.Time `json:"lastTransitionTime"`
	// Reason is a unique, one-word, CamelCase reason for the condition's last transition.
	// +optional
	Reason string `json:"reason"`
	// Message is a human-readable message indicating details about last transition.
	// +optional
	Message string `json:"message"`
}

// LibvirtClusterProviderStatus is the type that will be embedded in a Cluster.Status.ProviderStatus field.
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type LibvirtClusterProviderStatus struct {
	metav1.TypeMeta `json:",inline"`
}

// LibvirtMachineProviderConfigList contains a list of AWSMachineProviderConfig
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type LibvirtMachineProviderConfigList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []LibvirtMachineProviderConfig `json:"items"`
}

func init() {
	//SchemeBuilder.Register(&LibvirtMachineProviderConfigList{}, &LibvirtMachineProviderConfig{}, &LibvirtMachineProviderStatus{})
	SchemeBuilder.Register(&LibvirtMachineProviderConfigList{}, &LibvirtMachineProviderConfig{})
}
