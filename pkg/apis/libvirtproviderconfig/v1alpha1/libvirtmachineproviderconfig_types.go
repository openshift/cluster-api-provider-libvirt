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

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// LibvirtMachineProviderConfigList contains a list of LibvirtMachineProviderConfig
type LibvirtMachineProviderConfigList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []LibvirtMachineProviderConfig `json:"items"`
}

// TBD
type LibvirtMachineProviderStatus struct{}

func init() {
	SchemeBuilder.Register(&LibvirtMachineProviderConfig{}, &LibvirtMachineProviderConfigList{})
}
