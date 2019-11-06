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
	"time"

	"github.com/golang/glog"

	libvirt "github.com/libvirt/libvirt-go"

	providerconfigv1 "github.com/openshift/cluster-api-provider-libvirt/pkg/apis/libvirtproviderconfig/v1beta1"
	libvirtclient "github.com/openshift/cluster-api-provider-libvirt/pkg/cloud/libvirt/client"
	controllererrors "github.com/openshift/cluster-api/pkg/controller/error"

	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/diff"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/record"

	clusterv1 "github.com/openshift/cluster-api/pkg/apis/cluster/v1alpha1"
	machinev1 "github.com/openshift/cluster-api/pkg/apis/machine/v1beta1"
	clusterclient "github.com/openshift/cluster-api/pkg/client/clientset_generated/clientset"
	apierrors "github.com/openshift/cluster-api/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type errorWrapper struct {
	machine *machinev1.Machine
}

func (e *errorWrapper) Error(err error, message string) error {
	return fmt.Errorf("%s: %s: %v", e.machine.Name, message, err)
}

func (e *errorWrapper) WithLog(err error, message string) error {
	wrapped := e.Error(err, message)
	glog.Error(wrapped)
	return wrapped
}

var MachineActuator *Actuator

// Actuator is responsible for performing machine reconciliation
type Actuator struct {
	clusterClient  clusterclient.Interface
	reservedLeases *libvirtclient.Leases
	kubeClient     kubernetes.Interface
	clientBuilder  libvirtclient.LibvirtClientBuilderFuncType
	codec          codec
	eventRecorder  record.EventRecorder
}

type codec interface {
	DecodeFromProviderSpec(machinev1.ProviderSpec, runtime.Object) error
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
		kubeClient:    params.KubeClient,
		clientBuilder: params.ClientBuilder,
		codec:         params.Codec,
		eventRecorder: params.EventRecorder,
	}, nil
}

const (
	createEventAction = "Create"
	updateEventAction = "Update"
	deleteEventAction = "Delete"
	noEventAction     = ""
)

// Set corresponding event based on error. It also returns the original error
// for convenience, so callers can do "return handleMachineError(...)".
func (a *Actuator) handleMachineError(machine *machinev1.Machine, err *apierrors.MachineError, eventAction string) error {
	if eventAction != noEventAction {
		a.eventRecorder.Eventf(machine, corev1.EventTypeWarning, "Failed"+eventAction, "%v", err.Reason)
	}

	glog.Errorf("Machine error: %v", err.Message)
	return err
}

// Create creates a machine and is invoked by the Machine Controller
func (a *Actuator) Create(context context.Context, cluster *clusterv1.Cluster, machine *machinev1.Machine) error {
	glog.Infof("Creating machine %q", machine.Name)
	errWrapper := errorWrapper{machine: machine}

	machineProviderConfig, err := ProviderConfigMachine(a.codec, &machine.Spec)
	if err != nil {
		return a.handleMachineError(machine, apierrors.InvalidMachineConfiguration("error getting machineProviderConfig from spec: %v", err), createEventAction)
	}

	client, err := a.clientBuilder(machineProviderConfig.URI, machineProviderConfig.Volume.PoolName)
	if err != nil {
		return a.handleMachineError(machine, apierrors.CreateMachine("error creating libvirt client: %v", err), createEventAction)
	}

	defer client.Close()

	// fill researvedLeases on the first call to the create method
	if a.reservedLeases == nil {
		a.reservedLeases = &libvirtclient.Leases{Items: map[string]string{}}
		libvirtLeases, err := client.GetDHCPLeasesByNetwork(machineProviderConfig.NetworkInterfaceName)
		if err != nil {
			return errWrapper.WithLog(err, "error getting the dhcp leases from the libvirt")
		}
		libvirtclient.FillReservedLeases(a.reservedLeases, libvirtLeases)
	}

	dom, err := a.createVolumeAndDomain(machine, machineProviderConfig, client)
	if err != nil {
		return errWrapper.WithLog(err, "error creating libvirt machine")
	}

	defer func() {
		if dom != nil {
			dom.Free()
		}
	}()

	if err := a.updateStatus(machine, dom, client); err != nil {
		return errWrapper.WithLog(err, "error updating machine status")
	}

	a.eventRecorder.Eventf(machine, corev1.EventTypeNormal, "Created", "Created Machine %v", machine.Name)

	return nil
}

// Delete deletes a machine and is invoked by the Machine Controller
func (a *Actuator) Delete(context context.Context, cluster *clusterv1.Cluster, machine *machinev1.Machine) error {
	glog.Infof("Deleting machine %q", machine.Name)

	machineProviderConfig, err := ProviderConfigMachine(a.codec, &machine.Spec)
	if err != nil {
		return a.handleMachineError(machine, apierrors.InvalidMachineConfiguration("error getting machineProviderConfig from spec: %v", err), deleteEventAction)
	}

	client, err := a.clientBuilder(machineProviderConfig.URI, machineProviderConfig.Volume.PoolName)
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
func (a *Actuator) Update(context context.Context, cluster *clusterv1.Cluster, machine *machinev1.Machine) error {
	glog.Infof("Updating machine %v", machine.Name)
	errWrapper := errorWrapper{machine: machine}

	machineProviderConfig, err := ProviderConfigMachine(a.codec, &machine.Spec)
	if err != nil {
		return a.handleMachineError(machine, apierrors.InvalidMachineConfiguration("error getting machineProviderConfig from spec: %v", err), updateEventAction)
	}

	client, err := a.clientBuilder(machineProviderConfig.URI, machineProviderConfig.Volume.PoolName)
	if err != nil {
		return a.handleMachineError(machine, apierrors.UpdateMachine("error creating libvirt client: %v", err), updateEventAction)
	}

	defer client.Close()

	dom, err := client.LookupDomainByName(machine.Name)
	if err != nil {
		return a.handleMachineError(machine, apierrors.UpdateMachine("failed to look up domain by name: %v", err), updateEventAction)
	}

	defer dom.Free()

	a.eventRecorder.Eventf(machine, corev1.EventTypeNormal, "Updated", "Updated Machine %v", machine.Name)

	if err := a.updateStatus(machine, dom, client); err != nil {
		return errWrapper.WithLog(err, "error updating machine status")
	}

	return nil
}

// Exists test for the existance of a machine and is invoked by the Machine Controller
func (a *Actuator) Exists(context context.Context, cluster *clusterv1.Cluster, machine *machinev1.Machine) (bool, error) {
	glog.Infof("Checking if machine %v exists.", machine.Name)
	errWrapper := errorWrapper{machine: machine}

	machineProviderConfig, err := ProviderConfigMachine(a.codec, &machine.Spec)
	if err != nil {
		return false, a.handleMachineError(machine, apierrors.InvalidMachineConfiguration("error getting machineProviderConfig from spec: %v", err), noEventAction)
	}

	client, err := a.clientBuilder(machineProviderConfig.URI, machineProviderConfig.Volume.PoolName)
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
func (a *Actuator) createVolumeAndDomain(machine *machinev1.Machine, machineProviderConfig *providerconfigv1.LibvirtMachineProviderConfig, client libvirtclient.Client) (*libvirt.Domain, error) {
	domainName := machine.Name

	// Create volume
	if err := client.CreateVolume(
		libvirtclient.CreateVolumeInput{
			VolumeName:     domainName,
			BaseVolumeName: machineProviderConfig.Volume.BaseVolumeID,
			VolumeFormat:   "qcow2",
			VolumeSize:     machineProviderConfig.Volume.VolumeSize,
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
		NetworkInterfaceName:    machineProviderConfig.NetworkInterfaceName,
		NetworkInterfaceAddress: machineProviderConfig.NetworkInterfaceAddress,
		ReservedLeases:          a.reservedLeases,
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
			glog.Errorf("Error cleaning up volume: %v", err)
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
func (a *Actuator) deleteVolumeAndDomain(machine *machinev1.Machine, client libvirtclient.Client) error {
	if err := client.DeleteDomain(machine.Name); err != nil && err != libvirtclient.ErrDomainNotFound {
		return a.handleMachineError(machine, apierrors.DeleteMachine("error deleting %q domain %v", machine.Name, err), deleteEventAction)
	}

	if a.reservedLeases != nil {
		for _, addr := range machine.Status.Addresses {
			if addr.Type == corev1.NodeInternalIP {
				if _, ok := a.reservedLeases.Items[addr.Address]; ok {
					delete(a.reservedLeases.Items, addr.Address)
				}
			}
		}
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
func ProviderConfigMachine(codec codec, ms *machinev1.MachineSpec) (*providerconfigv1.LibvirtMachineProviderConfig, error) {
	providerSpec := ms.ProviderSpec
	if providerSpec.Value == nil {
		return nil, fmt.Errorf("no Value in ProviderConfig")
	}

	var config providerconfigv1.LibvirtMachineProviderConfig
	if err := codec.DecodeFromProviderSpec(providerSpec, &config); err != nil {
		return nil, err
	}

	return &config, nil
}

// updateStatus updates a machine object's status.
func (a *Actuator) updateStatus(machine *machinev1.Machine, dom *libvirt.Domain, client libvirtclient.Client) error {
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

	machineProviderConfig, err := ProviderConfigMachine(a.codec, &machine.Spec)
	if err != nil {
		glog.Errorf("Unable to get provider config from the machine %s", machine.Name)
	}

	addrs, err := NodeAddresses(client, dom, machineProviderConfig.NetworkInterfaceName)
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
	machine *machinev1.Machine,
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

	glog.Infof("Machine %s status has changed: %q", machine.Name, diff.ObjectReflectDiff(machine.Status, machineCopy.Status))

	now := metav1.Now()
	machineCopy.Status.LastUpdated = &now
	_, err = a.clusterClient.MachineV1beta1().
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
func ProviderStatusFromMachine(codec codec, machine *machinev1.Machine) (*providerconfigv1.LibvirtMachineProviderStatus, error) {
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

// NodeAddresses returns a slice of corev1.NodeAddress objects for a
// given libvirt domain.
func NodeAddresses(client libvirtclient.Client, dom *libvirt.Domain, networkInterfaceName string) ([]corev1.NodeAddress, error) {
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

	if len(ifaces) == 0 {
		glog.Infof("The domain does not have any network interfaces")
		return nil, &controllererrors.RequeueAfterError{RequeueAfter: time.Second}
	}

	for _, iface := range ifaces {
		for _, addr := range iface.Addrs {
			addrs = append(addrs, corev1.NodeAddress{
				Type:    corev1.NodeInternalIP,
				Address: addr.Addr,
			})

			if networkInterfaceName != "" {
				hostname, err := client.LookupDomainHostnameByDHCPLease(addr.Addr, networkInterfaceName)
				if err != nil {
					return addrs, err
				}

				addrs = append(addrs, corev1.NodeAddress{
					Type:    corev1.NodeHostName,
					Address: hostname,
				})

				addrs = append(addrs, corev1.NodeAddress{
					Type:    corev1.NodeInternalDNS,
					Address: hostname,
				})
			}
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
