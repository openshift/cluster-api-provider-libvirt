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
	"bytes"
	"fmt"

	"github.com/golang/glog"

	libvirt "github.com/libvirt/libvirt-go"

	libvirtutils "github.com/openshift/cluster-api-provider-libvirt/pkg/cloud/libvirt/actuators/machine/utils"
	providerconfigv1 "github.com/openshift/cluster-api-provider-libvirt/pkg/cloud/libvirt/providerconfig/v1alpha1"

	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	jsonserializer "k8s.io/apimachinery/pkg/runtime/serializer/json"
	clusterv1 "sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1"
	clusterclient "sigs.k8s.io/cluster-api/pkg/client/clientset_generated/clientset"
)

type errorWrapper struct {
	cluster *clusterv1.Cluster
	machine *clusterv1.Machine
}

func (e *errorWrapper) Error(err error, message string) error {
	return fmt.Errorf("%s/%s: %s: %v", e.cluster.Name, e.machine.Name, message, err)
}

func (e *errorWrapper) WithLog(err error, message string) error {
	wrapped := e.Error(err, message)
	glog.Error(wrapped)
	return wrapped
}

// Actuator is responsible for performing machine reconciliation
type Actuator struct {
	clusterClient clusterclient.Interface
	cidrOffset    int
	kubeClient    kubernetes.Interface
}

// ActuatorParams holds parameter information for Actuator
type ActuatorParams struct {
	ClusterClient clusterclient.Interface
	KubeClient    kubernetes.Interface
}

// NewActuator creates a new Actuator
func NewActuator(params ActuatorParams) (*Actuator, error) {
	return &Actuator{
		clusterClient: params.ClusterClient,
		cidrOffset:    50,
		kubeClient:    params.KubeClient,
	}, nil
}

// Create creates a machine and is invoked by the Machine Controller
func (a *Actuator) Create(cluster *clusterv1.Cluster, machine *clusterv1.Machine) error {
	glog.Infof("Creating machine %q for cluster %q.", machine.Name, cluster.Name)
	errWrapper := errorWrapper{cluster: cluster, machine: machine}

	client, err := clientForMachine(machine)
	if err != nil {
		return errWrapper.WithLog(err, "error creating libvirt client")
	}

	defer client.Close()

	// TODO: hack to increase IPs. Build proper logic in setNetworkInterfaces method
	a.cidrOffset++

	dom, err := createVolumeAndDomain(machine, a.cidrOffset, a.kubeClient, client)
	if err != nil {
		return errWrapper.WithLog(err, "error creating libvirt machine")
	}

	defer dom.Free()

	if err := a.updateStatus(machine, dom); err != nil {
		return errWrapper.WithLog(err, "error updating machine status")
	}

	return nil
}

// Delete deletes a machine and is invoked by the Machine Controller
func (a *Actuator) Delete(cluster *clusterv1.Cluster, machine *clusterv1.Machine) error {
	glog.Infof("Deleting machine %q for cluster %q.", machine.Name, cluster.Name)
	errWrapper := errorWrapper{cluster: cluster, machine: machine}

	client, err := clientForMachine(machine)
	if err != nil {
		return errWrapper.WithLog(err, "error creating libvirt client")
	}

	defer client.Close()

	return deleteVolumeAndDomain(machine, client)
}

// Update updates a machine and is invoked by the Machine Controller
func (a *Actuator) Update(cluster *clusterv1.Cluster, machine *clusterv1.Machine) error {
	glog.Infof("Updating machine %v for cluster %v.", machine.Name, cluster.Name)
	errWrapper := errorWrapper{cluster: cluster, machine: machine}

	client, err := clientForMachine(machine)
	if err != nil {
		return errWrapper.WithLog(err, "error creating libvirt client")
	}

	defer client.Close()

	dom, err := libvirtutils.LookupDomainByName(machine.Name, client)
	if err != nil {
		return errWrapper.WithLog(err, "failed to look up domain by name")
	}

	defer dom.Free()

	if err := a.updateStatus(machine, dom); err != nil {
		return errWrapper.WithLog(err, "error updating machine status")
	}

	return nil
}

// Exists test for the existance of a machine and is invoked by the Machine Controller
func (a *Actuator) Exists(cluster *clusterv1.Cluster, machine *clusterv1.Machine) (bool, error) {
	glog.Infof("Checking if machine %v for cluster %v exists.", machine.Name, cluster.Name)
	errWrapper := errorWrapper{cluster: cluster, machine: machine}

	client, err := clientForMachine(machine)
	if err != nil {
		return false, errWrapper.WithLog(err, "error creating libvirt client")
	}

	defer client.Close()

	return libvirtutils.DomainExists(machine.Name, client)
}

// CreateVolumeAndMachine creates a volume and domain which consumes the former one.
// Note: Upon success a pointer to the created domain is returned.  It
// is the caller's responsiblity to free this.
func createVolumeAndDomain(machine *clusterv1.Machine, offset int, kubeClient kubernetes.Interface, client *libvirtutils.Client) (*libvirt.Domain, error) {
	// decode config
	machineProviderConfig, err := machineProviderConfigFromClusterAPIMachineSpec(&machine.Spec)
	if err != nil {
		return nil, fmt.Errorf("error getting machineProviderConfig from spec: %v", err)
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
		return nil, fmt.Errorf("error creating volume: %v", err)
	}

	// Create domain
	if err = libvirtutils.CreateDomain(name, ignKey, name, name, networkInterfaceName, networkInterfaceAddress, autostart, memory, vcpu, offset, client, machineProviderConfig.CloudInit, kubeClient, machine.Namespace); err != nil {
		// Clean up the created volume if domain creation fails,
		// otherwise subsequent runs will fail.
		if err := libvirtutils.DeleteVolume(name, client); err != nil {
			glog.Errorf("error cleaning up volume: %v", err)
		}

		return nil, fmt.Errorf("error creating domain: %v", err)
	}

	// Lookup created domain for return.
	dom, err := libvirtutils.LookupDomainByName(name, client)
	if err != nil {
		return nil, fmt.Errorf("error looking up libvirt machine: %v", err)
	}

	return dom, nil
}

// deleteVolumeAndDomain deletes a domain and its referenced volume
func deleteVolumeAndDomain(machine *clusterv1.Machine, client *libvirtutils.Client) error {
	if err := libvirtutils.DeleteDomain(machine.Name, client); err != nil {
		return fmt.Errorf("error deleting domain: %v", err)
	}

	if err := libvirtutils.DeleteVolume(machine.Name, client); err != nil {
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

// updateStatus updates a machine object's status.
func (a *Actuator) updateStatus(machine *clusterv1.Machine, dom *libvirt.Domain) error {
	glog.Infof("Updating status for %s", machine.Name)

	status, err := ProviderStatusFromMachine(machine)
	if err != nil {
		return err
	}

	// Update the libvirt provider status in-place.
	if err := UpdateProviderStatus(status, dom); err != nil {
		return err
	}

	addrs, err := libvirtutils.NodeAddresses(dom)
	if err != nil {
		return err
	}

	if err := a.applyMachineStatus(machine, status, addrs); err != nil {
		return err
	}

	return nil
}

func (a *Actuator) applyMachineStatus(
	machine *clusterv1.Machine,
	status *providerconfigv1.LibvirtMachineProviderStatus,
	addrs []corev1.NodeAddress,
) error {
	// Encode the new status as a raw extension.
	rawStatus, err := EncodeLibvirtMachineProviderStatus(status)
	if err != nil {
		return err
	}

	machineCopy := machine.DeepCopy()
	machineCopy.Status.ProviderStatus = rawStatus

	if addrs != nil {
		machineCopy.Status.Addresses = addrs
	}

	if equality.Semantic.DeepEqual(machine.Status, machineCopy.Status) {
		glog.V(4).Infof("Machine %s status is unchanged", machine.Name)
		return nil
	}

	glog.Infof("Machine %s status has changed, updating", machine.Name)

	now := metav1.Now()
	machineCopy.Status.LastUpdated = &now
	_, err = a.clusterClient.ClusterV1alpha1().
		Machines(machineCopy.Namespace).UpdateStatus(machineCopy)

	return err
}

// EncodeLibvirtMachineProviderStatus encodes a libvirt provider
// status as a runtime.RawExtension for inclusion in a MachineStatus
// object.
func EncodeLibvirtMachineProviderStatus(status *providerconfigv1.LibvirtMachineProviderStatus) (*runtime.RawExtension, error) {
	serializer := jsonserializer.NewSerializer(jsonserializer.DefaultMetaFactory,
		providerconfigv1.Scheme, providerconfigv1.Scheme, false)

	var buffer bytes.Buffer
	if err := serializer.Encode(status, &buffer); err != nil {
		return nil, err
	}

	rawStatus := &runtime.RawExtension{
		Raw: bytes.TrimSpace(buffer.Bytes()),
	}

	return rawStatus, nil
}

// ProviderStatusFromMachine deserializes a libvirt provider status
// from a machine object.
func ProviderStatusFromMachine(machine *clusterv1.Machine) (*providerconfigv1.LibvirtMachineProviderStatus, error) {
	if machine.Status.ProviderStatus == nil {
		// Return a provider status empty apart from version and kind.
		emptyStatus := &providerconfigv1.LibvirtMachineProviderStatus{
			TypeMeta: metav1.TypeMeta{
				APIVersion: providerconfigv1.SchemeGroupVersion.String(),
				Kind:       "LibvirtMachineProviderStatus",
			},
		}

		return emptyStatus, nil
	}

	decoder := providerconfigv1.Codecs.UniversalDecoder(providerconfigv1.SchemeGroupVersion)
	obj, gvk, err := decoder.Decode([]byte(machine.Status.ProviderStatus.Raw), nil, nil)
	if err != nil {
		return nil, err
	}

	status, ok := obj.(*providerconfigv1.LibvirtMachineProviderStatus)
	if !ok {
		return nil, fmt.Errorf("unexpected object: %#v", gvk)
	}

	return status, nil
}

// UpdateProviderStatus updates the provider status in-place with info
// from the given libvirt domain.
func UpdateProviderStatus(status *providerconfigv1.LibvirtMachineProviderStatus, dom *libvirt.Domain) error {
	if dom == nil {
		status.InstanceID = nil
		status.InstanceState = nil

		return nil
	}

	uuid, err := dom.GetUUIDString()
	if err != nil {
		return err
	}

	state, _, err := dom.GetState()
	if err != nil {
		return err
	}

	stateString := libvirtutils.DomainStateString(state)

	status.InstanceID = &uuid
	status.InstanceState = &stateString

	return nil
}

// clientForMachine returns a libvirt client for the URI in the given
// machine's provider config.
func clientForMachine(machine *clusterv1.Machine) (*libvirtutils.Client, error) {
	machineProviderConfig, err := machineProviderConfigFromClusterAPIMachineSpec(&machine.Spec)
	if err != nil {
		return nil, fmt.Errorf("error getting machineProviderConfig from spec: %v", err)
	}

	return libvirtutils.BuildClient(machineProviderConfig.URI)
}
