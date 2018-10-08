// Copyright © 2018 The Kubernetes Authors.
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package machine

import (
	"fmt"

	"github.com/golang/glog"
	libvirtutils "github.com/openshift/cluster-api-provider-libvirt/cloud/libvirt/actuators/machine/utils"
	providerconfigv1 "github.com/openshift/cluster-api-provider-libvirt/cloud/libvirt/providerconfig/v1alpha1"
	clusterv1 "sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1"
	clusterclient "sigs.k8s.io/cluster-api/pkg/client/clientset_generated/clientset"
)

// Actuator is responsible for performing machine reconciliation
type Actuator struct {
	clusterClient clusterclient.Interface
	cidrOffset    int
}

// ActuatorParams holds parameter information for Actuator
type ActuatorParams struct {
	ClusterClient clusterclient.Interface
}

// NewActuator creates a new Actuator
func NewActuator(params ActuatorParams) (*Actuator, error) {
	return &Actuator{
		clusterClient: params.ClusterClient,
		cidrOffset:    50,
	}, nil
}

// Create creates a machine and is invoked by the Machine Controller
func (a *Actuator) Create(cluster *clusterv1.Cluster, machine *clusterv1.Machine) error {
	glog.Infof("Creating machine %q for cluster %q.", machine.Name, cluster.Name)
	// TODO: hack to increase IPs. Build proper logic in setNetworkInterfaces method
	a.cidrOffset++
	if err := createVolumeAndDomain(machine, a.cidrOffset); err != nil {
		glog.Errorf("Could not create libvirt machine: %v", err)
		return fmt.Errorf("error creating machine %v", err)
	}
	return nil
}

// Delete deletes a machine and is invoked by the Machine Controller
func (a *Actuator) Delete(cluster *clusterv1.Cluster, machine *clusterv1.Machine) error {
	glog.Infof("Deleting machine %q for cluster %q.", machine.Name, cluster.Name)
	return deleteVolumeAndDomain(machine)
}

// Update updates a machine and is invoked by the Machine Controller
func (a *Actuator) Update(cluster *clusterv1.Cluster, machine *clusterv1.Machine) error {
	glog.Infof("Updating machine %v for cluster %v.", machine.Name, cluster.Name)
	return fmt.Errorf("TODO: Not yet implemented")
}

// Exists test for the existance of a machine and is invoked by the Machine Controller
func (a *Actuator) Exists(cluster *clusterv1.Cluster, machine *clusterv1.Machine) (bool, error) {
	glog.Infof("Checking if machine %v for cluster %v exists.", machine.Name, cluster.Name)

	// decode config
	machineProviderConfig, err := machineProviderConfigFromClusterAPIMachineSpec(&machine.Spec)
	if err != nil {
		return false, fmt.Errorf("error getting machineProviderConfig from spec: %v", err)
	}

	// build libvirtd client
	client, err := libvirtutils.BuildClient(machineProviderConfig.URI)
	if err != nil {
		return false, fmt.Errorf("Failed to build libvirt client: %s", err)
	}
	return libvirtutils.DomainExists(machine.Name, client)
}

// CreateVolumeAndMachine creates a volume and domain which consumes the former one
func createVolumeAndDomain(machine *clusterv1.Machine, offset int) error {
	// decode config
	machineProviderConfig, err := machineProviderConfigFromClusterAPIMachineSpec(&machine.Spec)
	if err != nil {
		return fmt.Errorf("error getting machineProviderConfig from spec: %v", err)
	}

	// build libvirtd client
	client, err := libvirtutils.BuildClient(machineProviderConfig.URI)
	if err != nil {
		return fmt.Errorf("failed to build libvirt client: %v", err)
	}

	// TODO(alberto) create struct and function for converting machineconfig->libvirtConfig
	name := machine.Name
	baseVolume := machineProviderConfig.Volume.BaseVolumeID
	pool := machineProviderConfig.Volume.PoolName
	ignKey := machineProviderConfig.IgnKey
	networkInterfaceName := machineProviderConfig.NetworkInterfaceName
	networkInterfaceAddress := machineProviderConfig.NetworkInterfaceAddress
	autostart := machineProviderConfig.Autostart
	memory := machineProviderConfig.DomainMemory
	vcpu := machineProviderConfig.DomainVcpu

	// Create volume
	if err := libvirtutils.CreateVolume(name, pool, baseVolume, "", "qcow2", client); err != nil {
		return fmt.Errorf("error creating volume: %v", err)
	}

	// Create domain
	if err = libvirtutils.CreateDomain(name, ignKey, pool, name, name, networkInterfaceName, networkInterfaceAddress, autostart, memory, vcpu, offset, client); err != nil {
		return fmt.Errorf("error creating domain: %v", err)
	}
	return nil
}

// deleteVolumeAndDomain deletes a domain and its referenced volume
func deleteVolumeAndDomain(machine *clusterv1.Machine) error {
	// decode config
	machineProviderConfig, err := machineProviderConfigFromClusterAPIMachineSpec(&machine.Spec)
	if err != nil {
		return fmt.Errorf("error getting machineProviderConfig from spec: %v", err)
	}

	// build libvirtd client
	client, err := libvirtutils.BuildClient(machineProviderConfig.URI)
	if err != nil {
		return fmt.Errorf("Failed to build libvirt client: %s", err)
	}

	// delete domain
	if err := libvirtutils.DeleteDomain(machine.Name, client); err != nil {
		return fmt.Errorf("error deleting domain: %v", err)
	}

	// delete volume
	if err := libvirtutils.DeleteVolume(machine.Name, machineProviderConfig.Volume.PoolName, client); err != nil {
		return fmt.Errorf("error deleting volume: %v", err)
	}
	return nil
}

// MachineProviderConfigFromClusterAPIMachineSpec gets the machine provider config MachineSetSpec from the
// specified cluster-api MachineSpec.
func machineProviderConfigFromClusterAPIMachineSpec(ms *clusterv1.MachineSpec) (*providerconfigv1.LibvirtMachineProviderConfig, error) {
	if ms.ProviderConfig.Value == nil {
		return nil, fmt.Errorf("no Value in ProviderConfig")
	}
	obj, gvk, err := providerconfigv1.Codecs.UniversalDecoder(providerconfigv1.SchemeGroupVersion).Decode([]byte(ms.ProviderConfig.Value.Raw), nil, nil)
	if err != nil {
		return nil, err
	}
	spec, ok := obj.(*providerconfigv1.LibvirtMachineProviderConfig)
	if !ok {
		return nil, fmt.Errorf("unexpected object when parsing machine provider config: %#v", gvk)
	}
	return spec, nil
}
