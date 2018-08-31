//// Copyright © 2018 The Kubernetes Authors.
//// Licensed under the Apache License, Version 2.0 (the "License");
//// you may not use this file except in compliance with the License.
//// You may obtain a copy of the License at
////
////     http://www.apache.org/licenses/LICENSE-2.0
////
//// Unless required by applicable law or agreed to in writing, software
//// distributed under the License is distributed on an "AS IS" BASIS,
//// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//// See the License for the specific language governing permissions and
//// limitations under the License.
//
package providerconfig

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// LibvirtMachineProviderConfig is the type that will be embedded in a Machine.Spec.ProviderConfig field
// for an Libvirt instance.
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type LibvirtMachineProviderConfig struct {
	metav1.TypeMeta

	DomainMemory             int
	DomainVcpu               int
	IgnKey                   string
	Volume                   *Volume
	NetworkInterfaceName     string
	NetworkInterfaceHostname string
	NetworkInterfaceAddress  string
	NetworkUUID              string
	Autostart                bool
	Uri                      string
}

type Volume struct {
	PoolName     string
	BaseVolumeID string
	VolumeName   string
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type LibvirtClusterProviderConfig struct {
	metav1.TypeMeta
}

// LibvirtMachineProviderStatus is the type that will be embedded in a Machine.Status.ProviderStatus field.
// It contains Libvirt-specific status information.
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type LibvirtMachineProviderStatus struct {
	metav1.TypeMeta

	// InstanceID is the instance ID of the machine created in Libvirt
	InstanceID *string

	// InstanceState is the state of the Libvirt instance for this machine
	InstanceState *string

	// Conditions is a set of conditions associated with the Machine to indicate
	// errors or other status
	Conditions []LibvirtMachineProviderCondition
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
	Type LibvirtMachineProviderConditionType
	// Status is the status of the condition.
	Status corev1.ConditionStatus
	// LastProbeTime is the last time we probed the condition.
	// +optional
	LastProbeTime metav1.Time
	// LastTransitionTime is the last time the condition transitioned from one status to another.
	// +optional
	LastTransitionTime metav1.Time
	// Reason is a unique, one-word, CamelCase reason for the condition's last transition.
	// +optional
	Reason string
	// Message is a human-readable message indicating details about last transition.
	// +optional
	Message string
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type LibvirtClusterProviderStatus struct {
	metav1.TypeMeta
}
