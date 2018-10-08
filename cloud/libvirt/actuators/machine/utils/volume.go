package libvirt

import (
	"encoding/xml"
	"fmt"
	libvirt "github.com/libvirt/libvirt-go"
	"github.com/libvirt/libvirt-go-xml"
	"log"
	"strconv"
	"strings"
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

func CreateVolume(volumeName, poolName, baseVolumeName, source, volumeFormat string, client *Client) error {
	var volume *libvirt.StorageVol

	log.Printf("[DEBUG] Create a libvirt volume with name %s for pool %s from the base volume %s", volumeName, poolName, baseVolumeName)
	virConn := client.libvirt

	// TODO: lock pool
	//client.poolMutexKV.Lock(poolName)
	//defer client.poolMutexKV.Unlock(poolName)

	volume, err := getVolumeFromPool(volumeName, poolName, virConn)
	if err == nil {
		return fmt.Errorf("storage volume '%s' already exists", volumeName)
	}

	volumeDef := newDefVolume()
	volumeDef.Name = volumeName
	volumeDef.Target.Format.Type = volumeFormat
	var img image
	// an source image was given, this mean we can't choose size
	if source != "" {
		if baseVolumeName != "" {
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
		log.Printf("Image %s image is: %d bytes", img, size)
		volumeDef.Capacity.Unit = "B"
		volumeDef.Capacity.Value = size
	} else if baseVolumeName != "" {
		volume = nil
		volumeDef.Capacity.Value = uint64(size)
		baseVolume, err := getVolumeFromPool(baseVolumeName, poolName, virConn)
		if err != nil {
			return fmt.Errorf("Can't retrieve volume %s", baseVolumeName)
		}
		backingStoreDef, err := newDefBackingStoreFromLibvirt(baseVolume)
		if err != nil {
			return fmt.Errorf("Could not retrieve backing store %s", baseVolumeName)
		}
		volumeDef.BackingStore = &backingStoreDef
	}

	if volume == nil {
		volumeDefXML, err := xml.Marshal(volumeDef)
		if err != nil {
			return fmt.Errorf("Error serializing libvirt volume: %s", err)
		}

		// create the volume
		pool, err := virConn.LookupStoragePoolByName(poolName)
		defer pool.Free()

		// Refresh the pool of the volume so that libvirt knows it is
		// not longer in use.
		waitForSuccess("error refreshing pool for volume", func() error {
			return pool.Refresh(0)
		})
		if err != nil {
			return fmt.Errorf("can't find storage pool '%s'", poolName)
		}
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
		err = img.Import(newCopier(client.libvirt, volume, volumeDef.Capacity.Value), volumeDef)
		if err != nil {
			return fmt.Errorf("Error while uploading source %s: %s", img.String(), err)
		}
	}

	log.Printf("[INFO] Volume ID: %s", key)
	return nil
}

func getVolumeFromPool(volumeName, poolName string, virConn *libvirt.Connect) (*libvirt.StorageVol, error) {
	pool, err := virConn.LookupStoragePoolByName(poolName)
	if err != nil {
		return nil, fmt.Errorf("can't find storage pool %q: %v", poolName, err)
	}
	defer pool.Free()

	// Refresh the pool of the volume so that libvirt knows it is
	// not longer in use.
	waitForSuccess("error refreshing pool for volume", func() error {
		return pool.Refresh(0)
	})

	// Check whether the storage volume exists. Its name needs to be
	// unique.
	volume, err := pool.LookupStorageVolByName(volumeName)
	if err != nil {
		return nil, fmt.Errorf("can't retrieve volume %q: %v", volumeName, err)
	}
	//defer volume.Free()
	return volume, nil
}

// DeleteVolume removes the volume identified by `key` from libvirt
func DeleteVolume(name string, poolName string, client *Client) error {
	virConn := client.libvirt
	volume, err := getVolumeFromPool(name, poolName, virConn)
	if err != nil {
		return fmt.Errorf("failed getting volume from pool: %v", err)
	}
	// TODO: add locking support
	//poolName, err := pool.GetName()
	//if err != nil {
	//	return fmt.Errorf("Error retrieving name of volume: %s", err)
	//}
	//client.poolMutexKV.Lock(poolName)
	//defer client.poolMutexKV.Unlock(poolName)

	// Workaround for redhat#1293804
	// https://bugzilla.redhat.com/show_bug.cgi?id=1293804#c12
	// Does not solve the problem but it makes it happen less often.
	_, err = volume.GetXMLDesc(0)
	if err != nil {
		return fmt.Errorf("Can't retrieve volume %s XML desc: %s", name, err)
	}

	err = volume.Delete(0)
	if err != nil {
		return fmt.Errorf("Can't delete volume %s: %s", name, err)
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
