package utils

import (
	"fmt"

	"github.com/golang/glog"
	libvirt "github.com/libvirt/libvirt-go"
	libvirtxml "github.com/libvirt/libvirt-go-xml"
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
	if input.DomainName == "" {
		return fmt.Errorf("Failed to create domain, name is empty")
	}
	glog.Infof("Create resource libvirt_domain")

	// Get default values from Host
	domainDef, err := newDomainDefForConnection(client.connection)
	if err != nil {
		return fmt.Errorf("Failed to newDomainDefForConnection: %s", err)
	}

	// Get values from machineProviderConfig
	if err := domainDefInit(&domainDef, input.DomainName, input.DomainMemory, input.DomainVcpu); err != nil {
		return fmt.Errorf("Failed to init domain definition from machineProviderConfig: %v", err)
	}

	glog.Infof("setCoreOSIgnition")
	if input.Ignition != nil {
		if err := SetIgnition(&domainDef, client, input.Ignition, input.KubeClient, input.MachineNamespace, input.VolumeName, input.VolumePoolName); err != nil {
			return err
		}
	} else if input.IgnKey != "" {
		if err := setCoreOSIgnition(&domainDef, input.IgnKey); err != nil {
			return err
		}
	} else if input.CloudInit != nil {
		if err := setCloudInit(&domainDef, client, input.CloudInit, input.KubeClient, input.MachineNamespace, input.VolumeName, input.VolumePoolName); err != nil {
			return err
		}
	} else {
		return fmt.Errorf("machine does not has a IgnKey nor CloudInit value")
	}

	glog.Infof("setDisks")
	VolumeKey := baseVolumePath + input.VolumeName
	if err := setDisks(&domainDef, client.connection, VolumeKey); err != nil {
		return fmt.Errorf("Failed to setDisks: %s", err)
	}

	glog.Infof("setNetworkInterfaces")
	var waitForLeases []*libvirtxml.DomainInterface
	hostName := input.HostName
	if hostName == "" {
		hostName = input.DomainName
	}
	// TODO: support more than 1 interface
	partialNetIfaces := make(map[string]*pendingMapping, 1)
	if err := setNetworkInterfaces(&domainDef, client.connection, partialNetIfaces, &waitForLeases,
		hostName, input.NetworkInterfaceName,
		input.NetworkInterfaceAddress, input.AddressRange); err != nil {
		return err
	}

	// TODO: support setFilesystems
	//if err := setFilesystems(d, &domainDef); err != nil {
	//	return err
	//}

	connectURI, err := client.connection.GetURI()
	if err != nil {
		return fmt.Errorf("error retrieving libvirt connection URI: %v", err)
	}
	glog.Infof("Creating libvirt domain at %s", connectURI)

	data, err := xmlMarshallIndented(domainDef)
	if err != nil {
		return fmt.Errorf("error serializing libvirt domain: %v", err)
	}

	glog.Infof("Creating libvirt domain with XML:\n%s", data)
	domain, err := client.connection.DomainDefineXML(data)
	if err != nil {
		return fmt.Errorf("error defining libvirt domain: %v", err)
	}

	if err := domain.SetAutostart(input.Autostart); err != nil {
		return fmt.Errorf("error setting Autostart: %v", err)
	}

	err = domain.Create()
	if err != nil {
		return fmt.Errorf("error creating libvirt domain: %v", err)
	}
	defer domain.Free()

	id, err := domain.GetUUIDString()
	if err != nil {
		return fmt.Errorf("error retrieving libvirt domain id: %v", err)
	}

	glog.Infof("Domain ID: %s", id)
	return nil
}

// LookupDomainByName looks up a domain by name and returns a pointer to it.
// Note: The caller is responsible for freeing the returned domain.
func (client *Client) LookupDomainByName(name string) (*libvirt.Domain, error) {
	glog.Infof("Lookup domain by name: %q", name)
	if client.connection == nil {
		return nil, ErrLibVirtConIsNil
	}

	domain, err := client.connection.LookupDomainByName(name)
	if err != nil {
		return nil, err
	}

	return domain, nil
}

// DomainExists checks if domain exists
func (client *Client) DomainExists(name string) (bool, error) {
	glog.Infof("Check if %q domain exists", name)
	if client.connection == nil {
		return false, ErrLibVirtConIsNil
	}

	domain, err := client.connection.LookupDomainByName(name)
	if err != nil {
		if err.(libvirt.Error).Code == libvirt.ERR_NO_DOMAIN {
			return false, nil
		}
		return false, err
	}
	defer domain.Free()

	return true, nil
}

// DeleteDomain deletes a domain
func (client *Client) DeleteDomain(name string) error {
	exists, err := client.DomainExists(name)
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
