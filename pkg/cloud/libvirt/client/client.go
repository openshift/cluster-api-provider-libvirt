package client

import (
	"encoding/xml"
	"fmt"
	"runtime"

	"github.com/golang/glog"
	libvirt "github.com/libvirt/libvirt-go"
	libvirtxml "github.com/libvirt/libvirt-go-xml"
	providerconfigv1 "github.com/openshift/cluster-api-provider-libvirt/pkg/apis/libvirtproviderconfig/v1beta1"

	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/client-go/kubernetes"
)

//go:generate mockgen -source=./client.go -destination=./mock/client_generated.go -package=mock

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

	// CloudInitVolumeName of cloud init volume to be added to domain definition
	CloudInitVolumeName string

	// IgnitionVolumeName of ignition volume to be added to domain definition
	IgnitionVolumeName string

	// NetworkInterfaceName as name of network interface
	NetworkInterfaceName string

	// NetworkInterfaceAddress as address of network interface
	NetworkInterfaceAddress string

	// HostName as network interface hostname
	HostName string

	// AddressRange as IP subnet address range
	ReservedLeases *Leases

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

	// BaseVolumeName as name of the base volume
	BaseVolumeName string

	// Source as location of base volume
	Source string

	// VolumeFormat as volume format
	VolumeFormat string

	// VolumeSize contains the size of the volume
	VolumeSize *resource.Quantity
}

// LibvirtClientBuilderFuncType is function type for building aws client
type LibvirtClientBuilderFuncType func(URI string, poolName string) (Client, error)

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

	// VolumeExists checks if volume exists
	VolumeExists(name string) (bool, error)

	// DeleteVolume deletes a domain based on its name
	DeleteVolume(name string) error

	// GetDHCPLeasesByNetwork get all network DHCP leases by network name
	GetDHCPLeasesByNetwork(networkName string) ([]libvirt.NetworkDHCPLease, error)

	// LookupDomainHostnameByDHCPLease looks up a domain hostname based on its DHCP lease
	LookupDomainHostnameByDHCPLease(domIPAddress string, networkName string) (string, error)
}

type libvirtClient struct {
	connection *libvirt.Connect

	// storage pool that holds all volumes
	pool *libvirt.StoragePool
	// cache pool's name so we don't have to call failable GetName() method on pool all the time.
	poolName string
}

var _ Client = &libvirtClient{}

// NewClient returns libvirt client for the specified URI
func NewClient(URI string, poolName string) (Client, error) {
	connection, err := libvirt.NewConnect(URI)
	if err != nil {
		return nil, err
	}

	glog.Infof("Created libvirt connection: %p", connection)

	pool, err := connection.LookupStoragePoolByName(poolName)
	if err != nil {
		return nil, fmt.Errorf("can't find storage pool %q: %v", poolName, err)
	}

	return &libvirtClient{
		connection: connection,
		pool:       pool,
		poolName:   poolName,
	}, nil
}

// Close closes the client's libvirt connection.
func (client *libvirtClient) Close() error {
	glog.Infof("Closing libvirt connection: %p", client.connection)

	_, err := client.connection.Close()
	if err != nil {
		glog.Infof("Error closing libvirt connection: %v", err)
	}

	return err
}

// CreateDomain creates domain based on CreateDomainInput
func (client *libvirtClient) CreateDomain(input CreateDomainInput) error {
	if input.DomainName == "" {
		return fmt.Errorf("Failed to create domain, name is empty")
	}
	glog.Info("Create resource libvirt_domain")

	// Get default values from Host
	domainDef, err := newDomainDefForConnection(client.connection)
	if err != nil {
		return fmt.Errorf("Failed to newDomainDefForConnection: %s", err)
	}

	// Get values from machineProviderConfig
	if err := domainDefInit(&domainDef, input.DomainName, input.DomainMemory, input.DomainVcpu); err != nil {
		return fmt.Errorf("Failed to init domain definition from machineProviderConfig: %v", err)
	}

	glog.Info("Create volume")
	diskVolume, err := client.getVolume(input.VolumeName)
	if err != nil {
		return fmt.Errorf("can't retrieve volume %s for pool %s: %v", input.VolumeName, client.poolName, err)
	}
	if err := setDisks(&domainDef, diskVolume); err != nil {
		return fmt.Errorf("Failed to setDisks: %s", err)
	}

	glog.Info("Create ignition configuration")

	// Both "s390" and "s390x" are linux kernel architectures for Linux on IBM z Systems, and they are for 31-bit and 64-bit respectively.
	if runtime.GOARCH == "s390x" || runtime.GOARCH == "s390" {
		if input.Ignition != nil {
			if err := setIgnitionForS390X(&domainDef, client, input.Ignition, input.KubeClient, input.MachineNamespace, input.IgnitionVolumeName); err != nil {
				return err
			}
		}
	} else {
		if input.Ignition != nil {
			if err := setIgnition(&domainDef, client, input.Ignition, input.KubeClient, input.MachineNamespace, input.IgnitionVolumeName); err != nil {
				return err
			}
		} else if input.IgnKey != "" {
			ignVolume, err := client.getVolume(input.IgnKey)
			if err != nil {
				return fmt.Errorf("error getting ignition volume: %v", err)
			}
			ignVolumePath, err := ignVolume.GetPath()
			if err != nil {
				return fmt.Errorf("error getting ignition volume path: %v", err)
			}

			if err := setCoreOSIgnition(&domainDef, ignVolumePath); err != nil {
				return err
			}
		} else if input.CloudInit != nil {
			if err := setCloudInit(&domainDef, client, input.CloudInit, input.KubeClient, input.MachineNamespace, input.CloudInitVolumeName, input.DomainName); err != nil {
				return err
			}
		} else {
			return fmt.Errorf("machine does not has a IgnKey nor CloudInit value")
		}
	}

	glog.Info("Set up network interface")
	var waitForLeases []*libvirtxml.DomainInterface
	hostName := input.HostName
	if hostName == "" {
		hostName = input.DomainName
	}
	// TODO: support more than 1 interface
	partialNetIfaces := make(map[string]*pendingMapping, 1)
	if err := setNetworkInterfaces(
		&domainDef,
		client.connection,
		partialNetIfaces,
		&waitForLeases,
		hostName,
		input.NetworkInterfaceName,
		input.NetworkInterfaceAddress,
		input.ReservedLeases,
	); err != nil {
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
func (client *libvirtClient) LookupDomainByName(name string) (*libvirt.Domain, error) {
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
func (client *libvirtClient) DomainExists(name string) (bool, error) {
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
func (client *libvirtClient) DeleteDomain(name string) error {
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
			glog.Info("libvirt does not support undefine flags: will try again without flags")
			if err := domain.Undefine(); err != nil {
				return fmt.Errorf("couldn't undefine libvirt domain: %v", err)
			}
		} else {
			return fmt.Errorf("couldn't undefine libvirt domain with flags: %v", err)
		}
	}

	return nil
}

// CreateVolume creates volume based on CreateVolumeInput
func (client *libvirtClient) CreateVolume(input CreateVolumeInput) error {
	var volume *libvirt.StorageVol
	glog.Infof("Create a libvirt volume with name %s for pool %s from the base volume %s", input.VolumeName, client.poolName, input.BaseVolumeName)

	// TODO: lock pool
	//client.poolMutexKV.Lock(client.poolName)
	//defer client.poolMutexKV.Unlock(client.poolName)

	volume, err := client.getVolume(input.VolumeName)
	if err == nil {
		return fmt.Errorf("storage volume '%s' already exists", input.VolumeName)
	}

	volumeDef := newDefVolume(input.VolumeName)
	volumeDef.Target.Format.Type = input.VolumeFormat
	var img image
	// an source image was given, this mean we can't choose size
	if input.Source != "" {
		if input.BaseVolumeName != "" {
			return fmt.Errorf("'base_volume_name' can't be specified when also 'source' is given")
		}

		if img, err = newImage(input.Source); err != nil {
			return err
		}

		// update the image in the description, even if the file has not changed
		size, err := img.size()
		if err != nil {
			return err
		}
		glog.Infof("Image %s image is: %d bytes", img, size)
		volumeDef.Capacity.Unit = "B"
		volumeDef.Capacity.Value = size
	} else if input.BaseVolumeName != "" {
		volume = nil

		baseVolume, err := client.getVolume(input.BaseVolumeName)

		if err != nil {
			return fmt.Errorf("Can't retrieve volume %s", input.BaseVolumeName)
		}
		var baseVolumeInfo *libvirt.StorageVolInfo
		baseVolumeInfo, err = baseVolume.GetInfo()
		if err != nil {
			return fmt.Errorf("Can't retrieve volume info %s", input.BaseVolumeName)
		}

		var volumeSize uint64
		if input.VolumeSize != nil {
			size, _ := input.VolumeSize.AsInt64()
			volumeSize = uint64(size)
		} else {
			volumeSize = uint64(defaultSize)
		}

		if baseVolumeInfo.Capacity > volumeSize {
			volumeDef.Capacity.Value = baseVolumeInfo.Capacity
		} else {
			volumeDef.Capacity.Value = volumeSize
		}

		backingStoreDef, err := newDefBackingStoreFromLibvirt(baseVolume)
		if err != nil {
			return fmt.Errorf("Could not retrieve backing store %s", input.BaseVolumeName)
		}
		volumeDef.BackingStore = &backingStoreDef
	}

	if volume == nil {
		volumeDefXML, err := xml.Marshal(volumeDef)
		if err != nil {
			return fmt.Errorf("Error serializing libvirt volume: %s", err)
		}

		// create the volume
		// Refresh the pool of the volume so that libvirt knows it is
		// not longer in use.
		err = waitForSuccess("error refreshing pool for volume", func() error {
			return client.pool.Refresh(0)
		})
		if err != nil {
			return fmt.Errorf("can't find storage pool '%s'", client.poolName)
		}

		v, err := client.pool.StorageVolCreateXML(string(volumeDefXML), 0)
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
		err = img.importImage(newCopier(client.connection, volume, volumeDef.Capacity.Value), volumeDef)
		if err != nil {
			return fmt.Errorf("Error while uploading source %s: %s", img.string(), err)
		}
	}

	glog.Infof("Volume ID: %s", key)
	return nil
}

// VolumeExists checks if a volume exists
func (client *libvirtClient) VolumeExists(name string) (bool, error) {
	glog.Infof("Check if %q volume exists", name)
	if client.connection == nil {
		return false, ErrLibVirtConIsNil
	}

	volume, err := client.getVolume(name)
	if err != nil {
		return false, nil
	}
	volume.Free()
	return true, nil
}

func (client *libvirtClient) getVolume(volumeName string) (*libvirt.StorageVol, error) {
	// Check whether the storage volume exists. Its name needs to be
	// unique.
	volume, err := client.pool.LookupStorageVolByName(volumeName)
	if err != nil {
		// Let's try by ID in case of older Installer
		volume, err = client.connection.LookupStorageVolByKey(volumeName)
		if err != nil {
			return nil, fmt.Errorf("can't retrieve volume %q: %v", volumeName, err)
		}
	}
	return volume, nil
}

// DeleteVolume deletes a domain based on its name
func (client *libvirtClient) DeleteVolume(name string) error {
	exists, err := client.VolumeExists(name)
	if err != nil {
		return err
	}
	if !exists {
		glog.Infof("Volume %s does not exists", name)
		return ErrVolumeNotFound
	}
	glog.Infof("Deleting volume %s", name)

	volume, err := client.getVolume(name)
	if err != nil {
		return fmt.Errorf("Can't retrieve volume %s", name)
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
	//client.poolMutexKV.Lock(client.poolName)
	//defer client.poolMutexKV.Unlock(client.poolName)

	waitForSuccess("Error refreshing pool for volume", func() error {
		return volPool.Refresh(0)
	})

	err = volume.Delete(0)
	if err != nil {
		return fmt.Errorf("Can't delete volume %s: %s", name, err)
	}

	return nil
}

// GetDHCPLeasesByNetwork returns all network DHCP leases by network name
func (client *libvirtClient) GetDHCPLeasesByNetwork(networkName string) ([]libvirt.NetworkDHCPLease, error) {
	network, err := client.connection.LookupNetworkByName(networkName)
	if err != nil {
		glog.Errorf("Failed to fetch network %s from the libvirt", networkName)
		return nil, err
	}

	return network.GetDHCPLeases()
}

// LookupDomainHostnameByDHCPLease looks up a domain hostname based on its DHCP lease
func (client *libvirtClient) LookupDomainHostnameByDHCPLease(domIPAddress string, networkName string) (string, error) {
	dchpLeases, err := client.GetDHCPLeasesByNetwork(networkName)
	if err != nil {
		glog.Errorf("Failed to fetch dhcp leases for the network %s", networkName)
		return "", err
	}

	for _, lease := range dchpLeases {
		if lease.IPaddr == domIPAddress {
			return lease.Hostname, nil
		}
	}
	return "", fmt.Errorf("Failed to find hostname for the DHCP lease with IP %s", domIPAddress)
}
