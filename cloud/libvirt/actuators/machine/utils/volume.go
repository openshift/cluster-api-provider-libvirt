package machine

import (
	"encoding/xml"
	"fmt"
	libvirt "github.com/libvirt/libvirt-go"
	"github.com/libvirt/libvirt-go-xml"
	providerconfigv1 "github.com/openshift/cluster-api-provider-libvirt/cloud/libvirt/providerconfig/v1alpha1"
	"io"
	"log"
	"time"
)

const (
	// TODO: support size in the API
	size = 17706254336
)

// WaitSleepInterval time
var WaitSleepInterval = 1 * time.Second

// WaitTimeout time
var WaitTimeout = 5 * time.Minute

// waitForSuccess wait for success and timeout after 5 minutes.
func waitForSuccess(errorMessage string, f func() error) error {
	start := time.Now()
	for {
		err := f()
		if err == nil {
			return nil
		}
		log.Printf("[DEBUG] %s. Re-trying.\n", err)

		time.Sleep(WaitSleepInterval)
		if time.Since(start) > WaitTimeout {
			return fmt.Errorf("%s: %s", errorMessage, err)
		}
	}
}

func newDefVolume() libvirtxml.StorageVolume {
	return libvirtxml.StorageVolume{
		Target: &libvirtxml.StorageVolumeTarget{
			Format: &libvirtxml.StorageVolumeTargetFormat{
				Type: "qcow2",
			},
			Permissions: &libvirtxml.StorageVolumeTargetPermissions{
				Mode: "644",
			},
		},
		Capacity: &libvirtxml.StorageVolumeSize{
			Unit:  "bytes",
			Value: 1,
		},
	}
}

// network transparent image
type image interface {
	Size() (uint64, error)
	Import(func(io.Reader) error, libvirtxml.StorageVolume) error
	String() string
}

func newDefBackingStoreFromLibvirt(baseVolume *libvirt.StorageVol) (libvirtxml.StorageVolumeBackingStore, error) {
	baseVolumeDef, err := newDefVolumeFromLibvirt(baseVolume)
	if err != nil {
		return libvirtxml.StorageVolumeBackingStore{}, fmt.Errorf("could not get volume: %s", err)
	}
	baseVolPath, err := baseVolume.GetPath()
	if err != nil {
		return libvirtxml.StorageVolumeBackingStore{}, fmt.Errorf("could not get base image path: %s", err)
	}
	backingStoreDef := libvirtxml.StorageVolumeBackingStore{
		Path: baseVolPath,
		Format: &libvirtxml.StorageVolumeTargetFormat{
			Type: baseVolumeDef.Target.Format.Type,
		},
	}
	return backingStoreDef, nil
}

func newDefVolumeFromLibvirt(volume *libvirt.StorageVol) (libvirtxml.StorageVolume, error) {
	name, err := volume.GetName()
	if err != nil {
		return libvirtxml.StorageVolume{}, fmt.Errorf("could not get name for volume: %s", err)
	}
	volumeDefXML, err := volume.GetXMLDesc(0)
	if err != nil {
		return libvirtxml.StorageVolume{}, fmt.Errorf("could not get XML description for volume %s: %s", name, err)
	}
	volumeDef, err := newDefVolumeFromXML(volumeDefXML)
	if err != nil {
		return libvirtxml.StorageVolume{}, fmt.Errorf("could not get a volume definition from XML for %s: %s", volumeDef.Name, err)
	}
	return volumeDef, nil
}

// Creates a volume definition from a XML
func newDefVolumeFromXML(s string) (libvirtxml.StorageVolume, error) {
	var volumeDef libvirtxml.StorageVolume
	err := xml.Unmarshal([]byte(s), &volumeDef)
	if err != nil {
		return libvirtxml.StorageVolume{}, err
	}
	return volumeDef, nil
}

func createVolume(machineName string, machineProviderConfig *providerconfigv1.LibvirtMachineProviderConfig) error {
	var poolName string
	var baseVolumeID string
	var volumeName string
	var volumeFormat = "qcow2"
	var volume *libvirt.StorageVol

	if machineProviderConfig.Volume.PoolName != "" {
		poolName = machineProviderConfig.Volume.PoolName
	} else {
		return fmt.Errorf("machine does not has a Volume.PoolName value")
	}

	if machineProviderConfig.Volume.BaseVolumeID != "" {
		baseVolumeID = machineProviderConfig.Volume.BaseVolumeID
	} else {
		return fmt.Errorf("machine does not has a Volume.BaseVolumeID value")
	}

	if machineProviderConfig.Volume.VolumeName != "" {
		volumeName = machineProviderConfig.Volume.VolumeName
	} else if machineName != "" {
		volumeName = machineName
	} else {
		return fmt.Errorf("machine does not has a Volume.VolumeName value")
	}

	client, err := buildClient(machineProviderConfig.Uri)
	if err != nil {
		return fmt.Errorf("Failed to build libvirt client: %s", err)
	}

	log.Printf("[DEBUG] Create a libvirt volume with name %s for pool %s from the base volume %s", volumeName, poolName, baseVolumeID)
	virConn := client.libvirt

	// TODO: lock pool
	//client.poolMutexKV.Lock(poolName)
	//defer client.poolMutexKV.Unlock(poolName)

	pool, err := virConn.LookupStoragePoolByName(poolName)
	if err != nil {
		return fmt.Errorf("can't find storage pool '%s'", poolName)
	}
	defer pool.Free()

	// Refresh the pool of the volume so that libvirt knows it is
	// not longer in use.
	waitForSuccess("error refreshing pool for volume", func() error {
		return pool.Refresh(0)
	})

	// Check whether the storage volume already exists. Its name needs to be
	// unique.
	if _, err := pool.LookupStorageVolByName(volumeName); err == nil {
		return fmt.Errorf("storage volume '%s' already exists", volumeName)
	}

	volumeDef := newDefVolume()
	volumeDef.Name = volumeName
	volumeDef.Target.Format.Type = volumeFormat

	if baseVolumeID != "" {
		volume = nil
		volumeDef.Capacity.Value = uint64(size)
		baseVolume, err := client.libvirt.LookupStorageVolByKey(baseVolumeID)
		if err != nil {
			return fmt.Errorf("Can't retrieve volume %s", baseVolumeID)
		}
		backingStoreDef, err := newDefBackingStoreFromLibvirt(baseVolume)
		if err != nil {
			return fmt.Errorf("Could not retrieve backing store %s", baseVolumeID)
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

	log.Printf("[INFO] Volume ID: %s", key)
	return nil
}

func deleteVolume(volumeName string, uri string) error {
	client, err := buildClient(uri)
	if err != nil {
		return fmt.Errorf("Failed to build libvirt client: %s", err)
	}

	return removeVolume(client, volumeName)
}

// removeVolume removes the volume identified by `key` from libvirt
func removeVolume(client *Client, name string) error {
	volumePath := fmt.Sprintf(baseVolumePath+"%s", name)
	volume, err := client.libvirt.LookupStorageVolByPath(volumePath)
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
