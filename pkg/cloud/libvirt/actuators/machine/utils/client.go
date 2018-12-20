package utils

import (
	"github.com/golang/glog"
	libvirt "github.com/libvirt/libvirt-go"
	"github.com/openshift/cluster-api-provider-libvirt/pkg/cloud/libvirt/client"
)

// Client libvirt
type Client struct {
	connection *libvirt.Connect
}

var _ client.Client = &Client{}

// Close closes the client's libvirt connection.
func (client *Client) Close() error {
	glog.Infof("Closing libvirt connection: %p", client.connection)

	_, err := client.connection.Close()
	if err != nil {
		glog.Infof("Error closing libvirt connection: %v", err)
	}

	return err
}

// CreateDomain creates domain based on CreateDomainInput
func (client *Client) CreateDomain(input client.CreateDomainInput) error {
	return CreateDomain(
		input.DomainName,
		input.IgnKey,
		input.Ignition,
		input.VolumeName,
		input.HostName,
		input.NetworkInterfaceName,
		input.NetworkInterfaceAddress,
		input.VolumePoolName,
		input.Autostart,
		input.DomainMemory,
		input.DomainVcpu,
		input.AddressRange,
		client,
		input.CloudInit,
		input.KubeClient,
		input.MachineNamespace)
}

// LookupDomainByName looks up a domain based on its name
func (client *Client) LookupDomainByName(name string) (*libvirt.Domain, error) {
	return LookupDomainByName(name, client)
}

// DomainExists checks if domain exists
func (client *Client) DomainExists(name string) (bool, error) {
	return DomainExists(name, client)
}

// DeleteDomain deletes a domain
func (client *Client) DeleteDomain(name string) error {
	exists, err := DomainExists(name, client)
	if err != nil {
		return err
	}
	if !exists {
		return ErrDomainNotFound
	}
	return DeleteDomain(name, client)
}

// CreateVolume creates volume based on CreateVolumeInput
func (client *Client) CreateVolume(input client.CreateVolumeInput) error {
	return CreateVolume(
		input.VolumeName,
		input.PoolName,
		input.BaseVolumeID,
		input.Source,
		input.VolumeFormat,
		client,
	)
}

// DeleteVolume deletes a domain based on its name
func (client *Client) DeleteVolume(name string) error {
	exists, err := VolumeExists(name, client)
	if err != nil {
		return err
	}
	if !exists {
		glog.Infof("Volume %s does not exists", name)
		return ErrVolumeNotFound
	}
	return DeleteVolume(name, client)
}
