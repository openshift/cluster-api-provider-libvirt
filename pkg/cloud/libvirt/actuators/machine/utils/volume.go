package utils

import (
	"encoding/xml"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/golang/glog"

	libvirt "github.com/libvirt/libvirt-go"
	libvirtxml "github.com/libvirt/libvirt-go-xml"
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
		glog.Infof("%s. Re-trying.\n", err)

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

func CreateVolume(volumeName, poolName, baseVolumeID, source, volumeFormat string, client *Client) error {
	var volume *libvirt.StorageVol

	glog.Infof("Create a libvirt volume with name %s for pool %s from the base volume %s", volumeName, poolName, baseVolumeID)

	// TODO: lock pool
	//client.poolMutexKV.Lock(poolName)
	//defer client.poolMutexKV.Unlock(poolName)

	pool, err := client.connection.LookupStoragePoolByName(poolName)
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
	var img image
	// an source image was given, this mean we can't choose size
	if source != "" {
		if baseVolumeID != "" {
			return fmt.Errorf("'base_volume_id' can't be specified when also 'source' is given")
		}

		if img, err = newImage(source); err != nil {
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
	} else if baseVolumeID != "" {
		volume = nil
		volumeDef.Capacity.Value = uint64(size)
		baseVolume, err := client.connection.LookupStorageVolByKey(baseVolumeID)
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

	if source != "" {
		err = img.Import(newCopier(client.connection, volume, volumeDef.Capacity.Value), volumeDef)
		if err != nil {
			return fmt.Errorf("Error while uploading source %s: %s", img.String(), err)
		}
	}

	glog.Infof("Volume ID: %s", key)
	return nil
}

func EnsureVolumeIsDeleted(volumeName string, client *Client) error {
	exists, err := VolumeExists(volumeName, client)
	if err != nil {
		return err
	}
	if !exists {
		glog.Infof("Volume %s does not exists", volumeName)
		return nil
	}
	return DeleteVolume(volumeName, client)
}

// VolumeExists checks if a volume exists
func VolumeExists(volumeName string, client *Client) (bool, error) {
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

// DeleteVolume removes the volume identified by `key` from libvirt
func DeleteVolume(name string, client *Client) error {
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

func timeFromEpoch(str string) time.Time {
	var s, ns int

	ts := strings.Split(str, ".")
	if len(ts) == 2 {
		ns, _ = strconv.Atoi(ts[1])
	}
	s, _ = strconv.Atoi(ts[0])

	return time.Unix(int64(s), int64(ns))
}
