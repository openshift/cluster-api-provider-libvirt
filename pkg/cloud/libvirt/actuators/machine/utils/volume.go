package utils

import (
	"encoding/xml"
	"errors"
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

// ErrVolumeNotFound is returned when a domain is not found
var ErrVolumeNotFound = errors.New("Domain not found")

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

func timeFromEpoch(str string) time.Time {
	var s, ns int

	ts := strings.Split(str, ".")
	if len(ts) == 2 {
		ns, _ = strconv.Atoi(ts[1])
	}
	s, _ = strconv.Atoi(ts[0])

	return time.Unix(int64(s), int64(ns))
}
