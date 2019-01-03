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
	"context"
	"fmt"

	"github.com/golang/glog"

	libvirt "github.com/libvirt/libvirt-go"

	providerconfigv1 "github.com/openshift/cluster-api-provider-libvirt/pkg/apis/libvirtproviderconfig/v1alpha1"
	libvirtclient "github.com/openshift/cluster-api-provider-libvirt/pkg/cloud/libvirt/client"

	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/diff"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/record"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clusterv1 "sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1"
	clusterclient "sigs.k8s.io/cluster-api/pkg/client/clientset_generated/clientset"
	apierrors "sigs.k8s.io/cluster-api/pkg/errors"
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

var MachineActuator *Actuator

// Actuator is responsible for performing machine reconciliation
type Actuator struct {
	clusterClient clusterclient.Interface
	cidrOffset    int
	kubeClient    kubernetes.Interface
	clientBuilder libvirtclient.LibvirtClientBuilderFuncType
	codec         codec
	eventRecorder record.EventRecorder
}

type codec interface {
	DecodeFromProviderConfig(clusterv1.ProviderSpec, runtime.Object) error
	DecodeProviderStatus(*runtime.RawExtension, runtime.Object) error
	EncodeProviderStatus(runtime.Object) (*runtime.RawExtension, error)
}

// ActuatorParams holds parameter information for Actuator
type ActuatorParams struct {
	ClusterClient clusterclient.Interface
	KubeClient    kubernetes.Interface
	ClientBuilder libvirtclient.LibvirtClientBuilderFuncType
	Codec         codec
	EventRecorder record.EventRecorder
}

// NewActuator creates a new Actuator
func NewActuator(params ActuatorParams) (*Actuator, error) {
	return &Actuator{
		clusterClient: params.ClusterClient,
		cidrOffset:    50,
		kubeClient:    params.KubeClient,
		clientBuilder: params.ClientBuilder,
		codec:         params.Codec,
		eventRecorder: params.EventRecorder,
	}, nil
}

const (
	createEventAction = "Create"
	deleteEventAction = "Delete"
	noEventAction     = ""
)

// Set corresponding event based on error. It also returns the original error
// for convenience, so callers can do "return handleMachineError(...)".
func (a *Actuator) handleMachineError(machine *clusterv1.Machine, err *apierrors.MachineError, eventAction string) error {
	if eventAction != noEventAction {
		a.eventRecorder.Eventf(machine, corev1.EventTypeWarning, "Failed"+eventAction, "%v", err.Reason)
	}

	glog.Errorf("Machine error: %v", err.Message)
	return err
}

// Create creates a machine and is invoked by the Machine Controller
func (a *Actuator) Create(context context.Context, cluster *clusterv1.Cluster, machine *clusterv1.Machine) error {
	glog.Infof("Creating machine %q for cluster %q.", machine.Name, cluster.Name)
	errWrapper := errorWrapper{cluster: cluster, machine: machine}

	machineProviderConfig, err := ProviderConfigMachine(a.codec, &machine.Spec)
	if err != nil {
		return a.handleMachineError(machine, apierrors.InvalidMachineConfiguration("error getting machineProviderConfig from spec: %v", err), createEventAction)
	}

	client, err := a.clientBuilder(machineProviderConfig.URI)
	if err != nil {
		return a.handleMachineError(machine, apierrors.CreateMachine("error creating libvirt client: %v", err), createEventAction)
	}

	defer client.Close()

	// TODO: hack to increase IPs. Build proper logic in setNetworkInterfaces method
	a.cidrOffset++

	dom, err := a.createVolumeAndDomain(machine, machineProviderConfig, client)
	if err != nil {
		return errWrapper.WithLog(err, "error creating libvirt machine")
	}

	defer func() {
		if dom != nil {
			dom.Free()
		}
	}()

	if err := a.updateStatus(machine, dom); err != nil {
		return errWrapper.WithLog(err, "error updating machine status")
	}

	a.eventRecorder.Eventf(machine, corev1.EventTypeNormal, "Created", "Created Machine %v", machine.Name)

	return nil
}

// Delete deletes a machine and is invoked by the Machine Controller
func (a *Actuator) Delete(context context.Context, cluster *clusterv1.Cluster, machine *clusterv1.Machine) error {
	glog.Infof("Deleting machine %q for cluster %q.", machine.Name, cluster.Name)

	machineProviderConfig, err := ProviderConfigMachine(a.codec, &machine.Spec)
	if err != nil {
		return a.handleMachineError(machine, apierrors.InvalidMachineConfiguration("error getting machineProviderConfig from spec: %v", err), deleteEventAction)
	}

	client, err := a.clientBuilder(machineProviderConfig.URI)
	if err != nil {
		return a.handleMachineError(machine, apierrors.DeleteMachine("error creating libvirt client: %v", err), deleteEventAction)
	}

	defer client.Close()

	exists, err := client.DomainExists(machine.Name)
	if err != nil {
		return a.handleMachineError(machine, apierrors.DeleteMachine("error checking for domain existence: %v", err), deleteEventAction)
	}
	if exists {
		return a.deleteVolumeAndDomain(machine, client)
	}
	glog.Infof("Domain %s does not exist. Skipping deletion...", machine.Name)
	return nil
}

// Update updates a machine and is invoked by the Machine Controller
func (a *Actuator) Update(context context.Context, cluster *clusterv1.Cluster, machine *clusterv1.Machine) error {
	glog.Infof("Updating machine %v for cluster %v.", machine.Name, cluster.Name)
	errWrapper := errorWrapper{cluster: cluster, machine: machine}

	client, err := a.clientForMachine(a.codec, machine)
	if err != nil {
		return errWrapper.WithLog(err, "error creating libvirt client")
	}

	defer client.Close()

	dom, err := client.LookupDomainByName(machine.Name)
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
func (a *Actuator) Exists(context context.Context, cluster *clusterv1.Cluster, machine *clusterv1.Machine) (bool, error) {
	glog.Infof("Checking if machine %v for cluster %v exists.", machine.Name, cluster.Name)
	errWrapper := errorWrapper{cluster: cluster, machine: machine}

	client, err := a.clientForMachine(a.codec, machine)
	if err != nil {
		return false, errWrapper.WithLog(err, "error creating libvirt client")
	}

	defer client.Close()

	return client.DomainExists(machine.Name)
}

func cloudInitVolumeName(volumeName string) string {
	return fmt.Sprintf("%v_cloud-init", volumeName)
}

func ignitionVolumeName(volumeName string) string {
	return fmt.Sprintf("%v.ignition", volumeName)
}

// CreateVolumeAndMachine creates a volume and domain which consumes the former one.
// Note: Upon success a pointer to the created domain is returned.  It
// is the caller's responsiblity to free this.
func (a *Actuator) createVolumeAndDomain(machine *clusterv1.Machine, machineProviderConfig *providerconfigv1.LibvirtMachineProviderConfig, client libvirtclient.Client) (*libvirt.Domain, error) {
	domainName := machine.Name

	// Create volume
	if err := client.CreateVolume(
		libvirtclient.CreateVolumeInput{
			VolumeName:   domainName,
			PoolName:     machineProviderConfig.Volume.PoolName,
			BaseVolumeID: machineProviderConfig.Volume.BaseVolumeID,
			VolumeFormat: "qcow2",
		}); err != nil {
		return nil, a.handleMachineError(machine, apierrors.CreateMachine("error creating volume %v", err), createEventAction)
	}

	// Create domain
	if err := client.CreateDomain(libvirtclient.CreateDomainInput{
		DomainName:              domainName,
		IgnKey:                  machineProviderConfig.IgnKey,
		Ignition:                machineProviderConfig.Ignition,
		VolumeName:              domainName,
		CloudInitVolumeName:     cloudInitVolumeName(domainName),
		IgnitionVolumeName:      ignitionVolumeName(domainName),
		VolumePoolName:          machineProviderConfig.Volume.PoolName,
		NetworkInterfaceName:    machineProviderConfig.NetworkInterfaceName,
		NetworkInterfaceAddress: machineProviderConfig.NetworkInterfaceAddress,
		AddressRange:            a.cidrOffset,
		HostName:                domainName,
		Autostart:               machineProviderConfig.Autostart,
		DomainMemory:            machineProviderConfig.DomainMemory,
		DomainVcpu:              machineProviderConfig.DomainVcpu,
		CloudInit:               machineProviderConfig.CloudInit,
		KubeClient:              a.kubeClient,
		MachineNamespace:        machine.Namespace,
	}); err != nil {
		// Clean up the created volume if domain creation fails,
		// otherwise subsequent runs will fail.
		if err := client.DeleteVolume(domainName); err != nil {
			glog.Errorf("error cleaning up volume: %v", err)
		}

		return nil, a.handleMachineError(machine, apierrors.CreateMachine("error creating domain %v", err), createEventAction)
	}

	// Lookup created domain for return.
	dom, err := client.LookupDomainByName(domainName)
	if err != nil {
		return nil, a.handleMachineError(machine, apierrors.CreateMachine("error looking up libvirt machine %v", err), createEventAction)
	}

	return dom, nil
}

// deleteVolumeAndDomain deletes a domain and its referenced volume
func (a *Actuator) deleteVolumeAndDomain(machine *clusterv1.Machine, client libvirtclient.Client) error {
	if err := client.DeleteDomain(machine.Name); err != nil && err != libvirtclient.ErrDomainNotFound {
		return a.handleMachineError(machine, apierrors.DeleteMachine("error deleting %q domain %v", machine.Name, err), deleteEventAction)
	}

	// Delete machine volume
	if err := client.DeleteVolume(machine.Name); err != nil && err != libvirtclient.ErrVolumeNotFound {
		return a.handleMachineError(machine, apierrors.DeleteMachine("error deleting %q volume %v", machine.Name, err), deleteEventAction)
	}

	// Delete cloud init volume if exists
	if err := client.DeleteVolume(cloudInitVolumeName(machine.Name)); err != nil && err != libvirtclient.ErrVolumeNotFound {
		return a.handleMachineError(machine, apierrors.DeleteMachine("error deleting %q cloud init volume %v", cloudInitVolumeName(machine.Name), err), deleteEventAction)
	}

	// Delete cloud init volume if exists
	if err := client.DeleteVolume(ignitionVolumeName(machine.Name)); err != nil && err != libvirtclient.ErrVolumeNotFound {
		return a.handleMachineError(machine, apierrors.DeleteMachine("error deleting %q ignition volume %v", ignitionVolumeName(machine.Name), err), deleteEventAction)
	}

	a.eventRecorder.Eventf(machine, corev1.EventTypeNormal, "Deleted", "Deleted Machine %v", machine.Name)

	return nil
}

// ProviderConfigMachine gets the machine provider config MachineSetSpec from the
// specified cluster-api MachineSpec.
func ProviderConfigMachine(codec codec, ms *clusterv1.MachineSpec) (*providerconfigv1.LibvirtMachineProviderConfig, error) {
	// TODO(jchaloup): Remove providerConfig once all consumers migrate to providerSpec
	providerSpec := ms.ProviderConfig
	// providerSpec has higher priority over providerConfig
	if ms.ProviderSpec.Value != nil || ms.ProviderSpec.ValueFrom != nil {
		providerSpec = ms.ProviderSpec
		glog.Infof("Falling to default providerSpec\n")
	} else {
		glog.Infof("Falling to providerConfig\n")
	}

	if providerSpec.Value == nil {
		return nil, fmt.Errorf("no Value in ProviderConfig")
	}

	var config providerconfigv1.LibvirtMachineProviderConfig
	if err := codec.DecodeFromProviderConfig(providerSpec, &config); err != nil {
		return nil, err
	}

	return &config, nil
}

// updateStatus updates a machine object's status.
func (a *Actuator) updateStatus(machine *clusterv1.Machine, dom *libvirt.Domain) error {
	glog.Infof("Updating status for %s", machine.Name)

	status, err := ProviderStatusFromMachine(a.codec, machine)
	if err != nil {
		glog.Errorf("Unable to get provider status from machine: %v", err)
		return err
	}

	// Update the libvirt provider status in-place.
	if err := UpdateProviderStatus(status, dom); err != nil {
		glog.Errorf("Unable to update provider status: %v", err)
		return err
	}

	addrs, err := NodeAddresses(dom)
	if err != nil {
		glog.Errorf("Unable to get node addresses: %v", err)
		return err
	}

	if err := a.applyMachineStatus(machine, status, addrs); err != nil {
		glog.Errorf("Unable to apply machine status: %v", err)
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
	rawStatus, err := EncodeProviderStatus(a.codec, status)
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

	glog.Infof("Machine %s status has changed: %s", machine.Name, diff.ObjectReflectDiff(machine.Status, machineCopy.Status))

	now := metav1.Now()
	machineCopy.Status.LastUpdated = &now
	_, err = a.clusterClient.ClusterV1alpha1().
		Machines(machineCopy.Namespace).UpdateStatus(machineCopy)
	return err
}

// EncodeProviderStatus encodes a libvirt provider
// status as a runtime.RawExtension for inclusion in a MachineStatus
// object.
func EncodeProviderStatus(codec codec, status *providerconfigv1.LibvirtMachineProviderStatus) (*runtime.RawExtension, error) {
	return codec.EncodeProviderStatus(status)
}

// ProviderStatusFromMachine deserializes a libvirt provider status
// from a machine object.
func ProviderStatusFromMachine(codec codec, machine *clusterv1.Machine) (*providerconfigv1.LibvirtMachineProviderStatus, error) {
	status := &providerconfigv1.LibvirtMachineProviderStatus{}
	var err error
	if machine.Status.ProviderStatus != nil {
		err = codec.DecodeProviderStatus(machine.Status.ProviderStatus, status)
	}

	return status, err
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

	stateString := DomainStateString(state)

	status.InstanceID = &uuid
	status.InstanceState = &stateString

	return nil
}

// clientForMachine returns a libvirt client for the URI in the given
// machine's provider config.
func (a *Actuator) clientForMachine(codec codec, machine *clusterv1.Machine) (libvirtclient.Client, error) {
	machineProviderConfig, err := ProviderConfigMachine(codec, &machine.Spec)
	if err != nil {
		return nil, a.handleMachineError(machine, apierrors.InvalidMachineConfiguration("error getting machineProviderConfig from spec: %v", err), createEventAction)
	}

	return a.clientBuilder(machineProviderConfig.URI)
}

// NodeAddresses returns a slice of corev1.NodeAddress objects for a
// given libvirt domain.
func NodeAddresses(dom *libvirt.Domain) ([]corev1.NodeAddress, error) {
	addrs := []corev1.NodeAddress{}

	// If the domain is nil, return an empty address array.
	if dom == nil {
		return addrs, nil
	}

	ifaceSource := libvirt.DOMAIN_INTERFACE_ADDRESSES_SRC_LEASE
	ifaces, err := dom.ListAllInterfaceAddresses(ifaceSource)
	if err != nil {
		return nil, err
	}

	for _, iface := range ifaces {
		for _, addr := range iface.Addrs {
			addrs = append(addrs, corev1.NodeAddress{
				Type:    corev1.NodeInternalIP,
				Address: addr.Addr,
			})
		}
	}

	return addrs, nil
}

// DomainStateString returns a human-readable string for the given
// libvirt domain state.
func DomainStateString(state libvirt.DomainState) string {
	switch state {
	case libvirt.DOMAIN_NOSTATE:
		return "None"
	case libvirt.DOMAIN_RUNNING:
		return "Running"
	case libvirt.DOMAIN_BLOCKED:
		return "Blocked"
	case libvirt.DOMAIN_PAUSED:
		return "Paused"
	case libvirt.DOMAIN_SHUTDOWN:
		return "Shutdown"
	case libvirt.DOMAIN_CRASHED:
		return "Crashed"
	case libvirt.DOMAIN_PMSUSPENDED:
		return "Suspended"
	case libvirt.DOMAIN_SHUTOFF:
		return "Shutoff"
	default:
		return "Unknown"
	}
}
