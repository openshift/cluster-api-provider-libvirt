package utils

import (
	"encoding/xml"
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

	if client.connection == nil {
		return ErrLibVirtConIsNil
	}

	glog.Infof("Deleting domain %s", name)

	domain, err := client.connection.LookupDomainByName(name)
	if err != nil {
		return fmt.Errorf("Error retrieving libvirt domain: %s", err)
	}
	defer domain.Free()

	state, _, err := domain.GetState()
	if err != nil {
		return fmt.Errorf("Couldn't get info about domain: %s", err)
	}

	if state == libvirt.DOMAIN_RUNNING || state == libvirt.DOMAIN_PAUSED {
		if err := domain.Destroy(); err != nil {
			return fmt.Errorf("Couldn't destroy libvirt domain: %s", err)
		}
	}

	if err := domain.UndefineFlags(libvirt.DOMAIN_UNDEFINE_NVRAM); err != nil {
		if e := err.(libvirt.Error); e.Code == libvirt.ERR_NO_SUPPORT || e.Code == libvirt.ERR_INVALID_ARG {
			glog.Infof("libvirt does not support undefine flags: will try again without flags")
			if err := domain.Undefine(); err != nil {
				return fmt.Errorf("Couldn't undefine libvirt domain: %s", err)
			}
		} else {
			return fmt.Errorf("Couldn't undefine libvirt domain with flags: %s", err)
		}
	}

	return nil
}

// CreateVolume creates volume based on CreateVolumeInput
func (client *Client) CreateVolume(input client.CreateVolumeInput) error {
	var volume *libvirt.StorageVol

	glog.Infof("Create a libvirt volume with name %s for pool %s from the base volume %s", input.VolumeName, input.PoolName, input.BaseVolumeID)

	// TODO: lock pool
	//client.poolMutexKV.Lock(input.PoolName)
	//defer client.poolMutexKV.Unlock(input.PoolName)

	pool, err := client.connection.LookupStoragePoolByName(input.PoolName)
	if err != nil {
		return fmt.Errorf("can't find storage pool '%s'", input.PoolName)
	}
	defer pool.Free()

	// Refresh the pool of the volume so that libvirt knows it is
	// not longer in use.
	waitForSuccess("error refreshing pool for volume", func() error {
		return pool.Refresh(0)
	})

	// Check whether the storage volume already exists. Its name needs to be
	// unique.
	if _, err := pool.LookupStorageVolByName(input.VolumeName); err == nil {
		return fmt.Errorf("storage volume '%s' already exists", input.VolumeName)
	}

	volumeDef := newDefVolume()
	volumeDef.Name = input.VolumeName
	volumeDef.Target.Format.Type = input.VolumeFormat
	var img image
	// an source image was given, this mean we can't choose size
	if input.Source != "" {
		if input.BaseVolumeID != "" {
			return fmt.Errorf("'base_volume_id' can't be specified when also 'source' is given")
		}

		if img, err = newImage(input.Source); err != nil {
			return err
		}

		// update the image in the description, even if the file has not changed
		size, err := img.Size()
		if err != nil {
			return err
		}
		glog.Infof("Image %s image is: %d bytes", img, size)
		volumeDef.Capacity.Unit = "B"
		volumeDef.Capacity.Value = size
	} else if input.BaseVolumeID != "" {
		volume = nil
		volumeDef.Capacity.Value = uint64(size)
		baseVolume, err := client.connection.LookupStorageVolByKey(input.BaseVolumeID)
		if err != nil {
			return fmt.Errorf("Can't retrieve volume %s", input.BaseVolumeID)
		}
		backingStoreDef, err := newDefBackingStoreFromLibvirt(baseVolume)
		if err != nil {
			return fmt.Errorf("Could not retrieve backing store %s", input.BaseVolumeID)
		}
		volumeDef.BackingStore = &backingStoreDef
	}

	if volume == nil {
		volumeDefXML, err := xml.Marshal(volumeDef)
		if err != nil {
			return fmt.Errorf("Error serializing libvirt volume: %s", err)
		}

		// create the volume
		v, err := pool.StorageVolCreateXML(string(volumeDefXML), 0)
		if err != nil {
			return fmt.Errorf("Error creating libvirt volume: %s", err)
		}
		volume = v
		defer volume.Free()
	}

	// we use the key as the id
	key, err := volume.GetKey()
	if err != nil {
		return fmt.Errorf("Error retrieving volume key: %s", err)
	}

	if input.Source != "" {
		err = img.Import(newCopier(client.connection, volume, volumeDef.Capacity.Value), volumeDef)
		if err != nil {
			return fmt.Errorf("Error while uploading source %s: %s", img.String(), err)
		}
	}

	glog.Infof("Volume ID: %s", key)
	return nil
}

// VolumeExists checks if a volume exists
func (client *Client) VolumeExists(volumeName string) (bool, error) {
	glog.Infof("Check if %q volume exists", volumeName)
	if client.connection == nil {
		return false, ErrLibVirtConIsNil
	}

	volumePath := fmt.Sprintf(baseVolumePath+"%s", volumeName)
	volume, err := client.connection.LookupStorageVolByPath(volumePath)
	if err != nil {
		return false, nil
	}
	volume.Free()
	return true, nil
}

// DeleteVolume deletes a domain based on its name
func (client *Client) DeleteVolume(name string) error {
	exists, err := client.VolumeExists(name)
	if err != nil {
		return err
	}
	if !exists {
		glog.Infof("Volume %s does not exists", name)
		return ErrVolumeNotFound
	}
	glog.Infof("Deleting volume %s", name)

	volumePath := fmt.Sprintf(baseVolumePath+"%s", name)
	volume, err := client.connection.LookupStorageVolByPath(volumePath)
	if err != nil {
		return fmt.Errorf("Can't retrieve volume %s", volumePath)
	}
	defer volume.Free()

	// Refresh the pool of the volume so that libvirt knows it is
	// not longer in use.
	volPool, err := volume.LookupPoolByVolume()
	if err != nil {
		return fmt.Errorf("Error retrieving pool for volume: %s", err)
	}
	defer volPool.Free()

	// TODO: add locking support
	//poolName, err := volPool.GetName()
	//if err != nil {
	//	return fmt.Errorf("Error retrieving name of volume: %s", err)
	//}
	//client.poolMutexKV.Lock(poolName)
	//defer client.poolMutexKV.Unlock(poolName)

	waitForSuccess("Error refreshing pool for volume", func() error {
		return volPool.Refresh(0)
	})

	// Workaround for redhat#1293804
	// https://bugzilla.redhat.com/show_bug.cgi?id=1293804#c12
	// Does not solve the problem but it makes it happen less often.
	_, err = volume.GetXMLDesc(0)
	if err != nil {
		return fmt.Errorf("Can't retrieve volume %s XML desc: %s", volumePath, err)
	}

	err = volume.Delete(0)
	if err != nil {
		return fmt.Errorf("Can't delete volume %s: %s", volumePath, err)
	}

	return nil
}
