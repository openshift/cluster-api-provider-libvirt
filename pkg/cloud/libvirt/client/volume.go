package client

import (
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	libvirt "github.com/digitalocean/go-libvirt"
	"github.com/golang/glog"
	libvirtxml "github.com/libvirt/libvirt-go-xml"
)

const (
	defaultSize = 17706254336
)

// ErrVolumeNotFound is returned when a domain is not found
var ErrVolumeNotFound = errors.New("Domain not found")

var waitSleepInterval = 1 * time.Second

// waitTimeout time
var waitTimeout = 5 * time.Minute

// waitForSuccess wait for success and timeout after 5 minutes.
func waitForSuccess(errorMessage string, f func() error) error {
	start := time.Now()
	for {
		err := f()
		if err == nil {
			return nil
		}
		glog.Infof("%s. Re-trying.\n", err)

		time.Sleep(waitSleepInterval)
		if time.Since(start) > waitTimeout {
			return fmt.Errorf("%s: %s", errorMessage, err)
		}
	}
}

func newDefVolume(name string) libvirtxml.StorageVolume {
	return libvirtxml.StorageVolume{
		Name: name,
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

func newDefBackingStoreFromLibvirt(virtConn *libvirt.Libvirt, baseVolume *libvirt.StorageVol) (libvirtxml.StorageVolumeBackingStore, error) {
	baseVolumeDef, err := newDefVolumeFromLibvirt(virtConn, baseVolume)
	if err != nil {
		return libvirtxml.StorageVolumeBackingStore{}, fmt.Errorf("could not get volume: %s", err)
	}
	baseVolPath, err := virtConn.StorageVolGetPath(*baseVolume)
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

func newDefVolumeFromLibvirt(virtConn *libvirt.Libvirt, volume *libvirt.StorageVol) (libvirtxml.StorageVolume, error) {
	name := volume.Name
	volumeDefXML, err := virtConn.StorageVolGetXMLDesc(*volume, 0)
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

func timeFromEpoch(str string) time.Time {
	var s, ns int

	ts := strings.Split(str, ".")
	if len(ts) == 2 {
		ns, _ = strconv.Atoi(ts[1])
	}
	s, _ = strconv.Atoi(ts[0])

	return time.Unix(int64(s), int64(ns))
}

func uploadVolume(poolName string, client *libvirtClient, volumeDef libvirtxml.StorageVolume, img image) (string, error) {
	pool, err := client.virt.StoragePoolLookupByName(poolName)
	if err != nil {
		return "", fmt.Errorf("can't find storage pool %q", poolName)
	}

	//client.poolMutexKV.Lock(poolName)
	//defer client.poolMutexKV.Unlock(poolName)

	// Refresh the pool of the volume so that libvirt knows it is
	// not longer in use.
	err = waitForSuccess("Error refreshing pool for volume", func() error {
		return client.virt.StoragePoolRefresh(pool, 0)
	})
	if err != nil {
		return "", fmt.Errorf("timeout when calling waitForSuccess: %v", err)
	}

	volumeDefXML, err := xml.Marshal(volumeDef)
	if err != nil {
		return "", fmt.Errorf("Error serializing libvirt volume: %s", err)
	}
	// create the volume
	volume, err := client.virt.StorageVolCreateXML(pool, string(volumeDefXML), 0)
	if err != nil {
		return "", fmt.Errorf("Error creating libvirt volume for device %s: %s", volumeDef.Name, err)
	}

	// upload ISO file
	err = img.importImage(newCopier(client.virt, &volume, volumeDef.Capacity.Value), volumeDef)
	if err != nil {
		return "", fmt.Errorf("Error while uploading volume %s: %s", img.string(), err)
	}

	glog.Infof("Volume ID: %s", volume.Key)
	return volume.Key, nil
}

func newCopier(virConn *libvirt.Libvirt, volume *libvirt.StorageVol, size uint64) func(src io.Reader) error {
	copier := func(src io.Reader) error {
		r, w := io.Pipe()
		defer w.Close()

		go func() error {
			buffer := make([]byte, 4*1024*1024)
			bytesCopied, err := io.CopyBuffer(w, src, buffer)

			// if we get unexpected EOF this mean that connection was closed suddently from server side
			// the problem is not on the plugin but on server hosting currupted images
			if err == io.ErrUnexpectedEOF {
				return fmt.Errorf("error: transfer was unexpectedly closed from the server while downloading. Please try again later or check the server hosting sources")
			}
			if err != nil {
				return fmt.Errorf("error while copying source to volume %s", err)
			}

			if uint64(bytesCopied) != size {
				return fmt.Errorf("error during volume Upload. BytesCopied: %d != %d volume.size", bytesCopied, size)
			}

			return w.Close()

		}()

		if err := virConn.StorageVolUpload(*volume, r, 0, size, 0); err != nil {
			return fmt.Errorf("error while uploading volume %s", err)
		}

		return nil
	}
	return copier
}
