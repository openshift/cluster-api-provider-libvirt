package client

import (
	libvirt "github.com/libvirt/libvirt-go"
	providerconfigv1 "github.com/openshift/cluster-api-provider-libvirt/pkg/apis/libvirtproviderconfig/v1alpha1"
	"k8s.io/client-go/kubernetes"
)

// CreateDomainInput specifies input parameters for CreateDomain operation
type CreateDomainInput struct {
	// DomainName is name of domain to be created
	DomainName string

	// IgnKey is name of existing volume with ignition config (DEPRECATED)
	IgnKey string

	// Ignition configuration to be injected during bootstrapping
	Ignition *providerconfigv1.Ignition

	// CloudInit configuration to be run during bootstrapping
	CloudInit *providerconfigv1.CloudInit

	// VolumeName of volume to be added to domain definition
	VolumeName string

	// VolumePoolName of pool where VolumeName volume is located
	VolumePoolName string

	// NetworkInterfaceName as name of network interface
	NetworkInterfaceName string

	// NetworkInterfaceAddress as address of network interface
	NetworkInterfaceAddress string

	// HostName as network interface hostname
	HostName string

	// AddressRange as IP subnet address range
	AddressRange int

	// Autostart as domain autostart
	Autostart bool

	// DomainMemory allocated for running domain
	DomainMemory int

	// DomainVcpu allocated for running domain
	DomainVcpu int

	// KubeClient as kubernetes client
	KubeClient kubernetes.Interface

	// MachineNamespace with machine object
	MachineNamespace string
}

// CreateVolumeInput specifies input parameters for CreateVolume operation
type CreateVolumeInput struct {
	// VolumeName to be created
	VolumeName string

	// PoolName where VolumeName volume is located
	PoolName string

	// BaseVolumeID as base volume ID
	BaseVolumeID string

	// Source as location of base volume
	Source string

	// VolumeFormat as volume format
	VolumeFormat string
}

// Client is a wrapper object for actual libvirt library to allow for easier testing.
type Client interface {
	// Close closes the client's libvirt connection.
	Close() error

	// CreateDomain creates domain based on CreateDomainInput
	CreateDomain(CreateDomainInput) error

	// DeleteDomain deletes a domain
	DeleteDomain(name string) error

	// DomainExists checks if domain exists
	DomainExists(name string) (bool, error)

	// LookupDomainByName looks up a domain based on its name
	LookupDomainByName(name string) (*libvirt.Domain, error)

	// CreateVolume creates volume based on CreateVolumeInput
	CreateVolume(CreateVolumeInput) error

	// DeleteVolume deletes a domain based on its name
	DeleteVolume(name string) error
}
