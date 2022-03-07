package client

import (
	"context"
	"encoding/xml"
	"fmt"
	"net"

	libvirt "github.com/digitalocean/go-libvirt"
	liburi "github.com/dmacvicar/terraform-provider-libvirt/libvirt/uri"
	"github.com/golang/glog"
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
	CreateDomain(context.Context, CreateDomainInput) error

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
	GetDHCPLeasesByNetwork(networkName string) ([]libvirt.NetworkDhcpLease, error)

	// LookupDomainHostnameByDHCPLease looks up a domain hostname based on its DHCP lease
	LookupDomainHostnameByDHCPLease(domIPAddress string, networkName string) (string, error)

	GetConn() *libvirt.Libvirt

	ListAllInterfaceAddresses(dom *libvirt.Domain, source libvirt.DomainInterfaceAddressesSource) ([]libvirt.DomainInterface, error)
}

type libvirtClient struct {
	uri string

	virt *libvirt.Libvirt

	connection net.Conn

	// storage pool that holds all volumes
	pool *libvirt.StoragePool
	// cache pool's name so we don't have to call failable GetName() method on pool all the time.
	poolName string
}

var _ Client = &libvirtClient{}

// NewClient returns libvirt client for the specified URI
func NewClient(URI string, poolName string) (Client, error) {
	glog.Infof("[INFO] libvirt NewClient to URI: %v poolName: %v\n", URI, poolName)
	u, err := liburi.Parse(URI)
	if err != nil {
		return nil, err
	}

	conn, err := u.DialTransport()
	if err != nil {
		return nil, err
	}

	virt := libvirt.New(conn)
	if err := virt.ConnectToURI(libvirt.ConnectURI(u.RemoteName())); err != nil {
		return nil, fmt.Errorf("failed to connect to: %s", u.RemoteName())
	}

	v, err := virt.ConnectGetLibVersion()
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve libvirt version: %w", err)
	}
	glog.Infof("[INFO] libvirt client libvirt version: %v\n", v)

	pool, err := virt.StoragePoolLookupByName(poolName)
	if err != nil {
		return nil, fmt.Errorf("can't find storage pool %q: %v", poolName, err)
	}

	return &libvirtClient{
		uri:        URI,
		virt:       virt,
		connection: conn,
		pool:       &pool,
		poolName:   poolName,
	}, nil
}

// Close closes the client's libvirt connection.
func (client *libvirtClient) Close() error {
	glog.Infof("Closing libvirt connection: %p", client.connection)
	err := client.connection.Close()
	if err != nil {
		glog.Infof("Error closing libvirt connection: %v", err)
	}
	return err
}

func (client *libvirtClient) GetConn() *libvirt.Libvirt {
	return client.virt
}

func (client *libvirtClient) ListAllInterfaceAddresses(dom *libvirt.Domain, source libvirt.DomainInterfaceAddressesSource) ([]libvirt.DomainInterface, error) {
	return client.virt.DomainInterfaceAddresses(*dom, uint32(source), 0)
}

// CreateDomain creates domain based on CreateDomainInput
func (client *libvirtClient) CreateDomain(ctx context.Context, input CreateDomainInput) error {
	if input.DomainName == "" {
		return fmt.Errorf("Failed to create domain, name is empty")
	}
	glog.Info("Create resource libvirt_domain")

	// Get default values from Host
	domainDef, err := newDomainDefForConnection(client.virt)
	if err != nil {
		return fmt.Errorf("Failed to newDomainDefForConnection: %s", err)
	}

	arch, err := getHostArchitecture(client.virt)
	if err != nil {
		return fmt.Errorf("Error retrieving host architecture: %s", err)
	}

	// Get values from machineProviderConfig
	if err := domainDefInit(&domainDef, &input, arch); err != nil {
		return fmt.Errorf("Failed to init domain definition from machineProviderConfig: %v", err)
	}

	glog.Info("Create volume")
	diskVolume, err := client.getVolume(input.VolumeName)
	if err != nil {
		return fmt.Errorf("can't retrieve volume %s for pool %s: %v", input.VolumeName, client.poolName, err)
	}
	if err := setDisks(client.virt, &domainDef, &diskVolume); err != nil {
		return fmt.Errorf("Failed to setDisks: %s", err)
	}

	glog.Info("Create ignition configuration")

	if input.Ignition != nil {
		if err := setIgnition(ctx, &domainDef, client, input.Ignition, input.KubeClient, input.MachineNamespace, input.IgnitionVolumeName, arch); err != nil {
			return err
		}
	} else if input.IgnKey != "" {
		ignVolume, err := client.getVolume(input.IgnKey)
		if err != nil {
			return fmt.Errorf("error getting ignition volume: %v", err)
		}
		ignVolumePath, err := client.virt.StorageVolGetPath(ignVolume)
		if err != nil {
			return fmt.Errorf("error getting ignition volume path: %v", err)
		}

		if err := setCoreOSIgnition(&domainDef, ignVolumePath, arch); err != nil {
			return err
		}
	} else if input.CloudInit != nil {
		if err := setCloudInit(ctx, &domainDef, client, input.CloudInit, input.KubeClient, input.MachineNamespace, input.CloudInitVolumeName, input.DomainName); err != nil {
			return err
		}
	} else {
		return fmt.Errorf("machine does not has a IgnKey nor CloudInit value")
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
		client.virt,
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

	glog.Infof("Creating libvirt domain at %s", client.uri)

	data, err := xmlMarshallIndented(domainDef)
	if err != nil {
		return fmt.Errorf("error serializing libvirt domain: %v", err)
	}

	glog.Infof("Creating libvirt domain with XML:\n%s", data)
	domain, err := client.virt.DomainDefineXML(data)
	if err != nil {
		return fmt.Errorf("error defining libvirt domain: %v", err)
	}

	autostart := int32(0)
	if input.Autostart {
		autostart = 1
	}
	if err := client.virt.DomainSetAutostart(domain, autostart); err != nil {
		return fmt.Errorf("error setting Autostart: %v", err)
	}

	err = client.virt.DomainCreate(domain)
	if err != nil {
		return fmt.Errorf("error creating libvirt domain: %v", err)
	}

	glog.Infof("Domain ID: %s", domain.UUID)
	return nil
}

// LookupDomainByName looks up a domain by name and returns a pointer to it.
// Note: The caller is responsible for freeing the returned domain.
func (client *libvirtClient) LookupDomainByName(name string) (*libvirt.Domain, error) {
	glog.Infof("Lookup domain by name: %q", name)
	if client.connection == nil {
		return nil, ErrLibVirtConIsNil
	}

	domain, err := client.virt.DomainLookupByName(name)
	if err != nil {
		return nil, err
	}

	return &domain, nil
}

// DomainExists checks if domain exists
func (client *libvirtClient) DomainExists(name string) (bool, error) {
	glog.Infof("Check if %q domain exists", name)
	if client.connection == nil {
		return false, ErrLibVirtConIsNil
	}

	_, err := client.virt.DomainLookupByName(name)
	if err != nil {
		if libvirt.IsNotFound(err) {
			return false, nil
		}
		return false, err
	}

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

	domain, err := client.virt.DomainLookupByName(name)
	if err != nil {
		return fmt.Errorf("Error retrieving libvirt domain: %s", err)
	}

	state, _, err := client.virt.DomainGetState(domain, 0)
	if err != nil {
		return fmt.Errorf("Couldn't get info about domain: %s", err)
	}

	if libvirt.DomainState(state) == libvirt.DomainRunning || libvirt.DomainState(state) == libvirt.DomainPaused {
		if err := client.virt.DomainDestroy(domain); err != nil {
			return fmt.Errorf("Couldn't destroy libvirt domain: %s", err)
		}
	}

	if err := client.virt.DomainUndefineFlags(domain, libvirt.DomainUndefineNvram); err != nil {
		if e := err.(libvirt.Error); e.Code == uint32(libvirt.ErrNoSupport) || e.Code == uint32(libvirt.ErrInvalidArg) {
			glog.Info("libvirt does not support undefine flags: will try again without flags")
			if err := client.virt.DomainUndefine(domain); err != nil {
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

	v, err := client.getVolume(input.VolumeName)
	if err == nil {
		return fmt.Errorf("storage volume '%s' already exists", input.VolumeName)
	}
	volume = &v

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
		_, baseVolumeCapacity, baseVolumeSize, err := client.virt.StorageVolGetInfo(baseVolume)
		if err != nil {
			return fmt.Errorf("Can't retrieve volume info %s", input.BaseVolumeName)
		}

		if baseVolumeCapacity > baseVolumeSize {
			volumeDef.Capacity.Value = uint64(baseVolumeCapacity)
		} else {
			volumeDef.Capacity.Value = baseVolumeSize
		}

		backingStoreDef, err := newDefBackingStoreFromLibvirt(client.virt, &baseVolume)
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
			_, err := client.virt.StoragePoolLookupByName(client.poolName)
			return err
		})
		if err != nil {
			return fmt.Errorf("can't find storage pool '%s'", client.poolName)
		}

		pool, err := client.virt.StoragePoolLookupByName(client.poolName)
		if err != nil {
			return fmt.Errorf("can't find storage pool '%s'", volume.Pool)
		}

		v, err := client.virt.StorageVolCreateXML(pool, string(volumeDefXML), 0)
		if err != nil {
			return fmt.Errorf("Error creating libvirt volume: %s", err)
		}
		volume = &v
	}

	// we use the key as the id
	if input.Source != "" {
		err = img.importImage(newCopier(client.virt, volume, volumeDef.Capacity.Value), volumeDef)
		if err != nil {
			return fmt.Errorf("Error while uploading source %s: %s", img.string(), err)
		}
	}

	glog.Infof("Volume ID: %s", volume.Key)
	return nil
}

// VolumeExists checks if a volume exists
func (client *libvirtClient) VolumeExists(name string) (bool, error) {
	glog.Infof("Check if %q volume exists", name)
	if client.connection == nil {
		return false, ErrLibVirtConIsNil
	}

	_, err := client.getVolume(name)
	if err != nil {
		return false, nil
	}
	return true, nil
}

func (client *libvirtClient) getVolume(volumeName string) (libvirt.StorageVol, error) {
	// Check whether the storage volume exists. Its name needs to be
	// unique.
	volume, err := client.virt.StorageVolLookupByName(*client.pool, volumeName)
	if err != nil {
		// Let's try by ID in case of older Installer
		volume, err = client.virt.StorageVolLookupByKey(volumeName)
		if err != nil {
			return libvirt.StorageVol{}, fmt.Errorf("can't retrieve volume %q: %v", volumeName, err)
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

	// Refresh the pool of the volume so that libvirt knows it is
	// not longer in use.
	_, err = client.virt.StoragePoolLookupByVolume(volume)
	if err != nil {
		return fmt.Errorf("Error retrieving pool for volume: %s", err)
	}

	// TODO: add locking support
	//client.poolMutexKV.Lock(client.poolName)
	//defer client.poolMutexKV.Unlock(client.poolName)

	waitForSuccess("Error refreshing pool for volume", func() error {
		_, err := client.virt.StoragePoolLookupByVolume(volume)
		return err
	})

	err = client.virt.StorageVolDelete(volume, libvirt.StorageVolDeleteNormal)
	if err != nil {
		return fmt.Errorf("Can't delete volume %s: %s", name, err)
	}

	return nil
}

// This may also be implementable with https://libvirt.org/html/libvirt-libvirt-domain.html#virDomainInterfaceAddresses
// GetDHCPLeasesByNetwork returns all network DHCP leases by network name
func (client *libvirtClient) GetDHCPLeasesByNetwork(networkName string) ([]libvirt.NetworkDhcpLease, error) {
	network, err := client.virt.NetworkLookupByName(networkName)
	if err != nil {
		glog.Errorf("Failed to fetch network %s from the libvirt", networkName)
		return nil, err
	}
	leases, _, err := client.virt.NetworkGetDhcpLeases(network, []string{}, 1, 0)
	return leases, err
}

// LookupDomainHostnameByDHCPLease looks up a domain hostname based on its DHCP lease
func (client *libvirtClient) LookupDomainHostnameByDHCPLease(domIPAddress string, networkName string) (string, error) {
	dchpLeases, err := client.GetDHCPLeasesByNetwork(networkName)
	if err != nil {
		glog.Errorf("Failed to fetch dhcp leases for the network %s", networkName)
		return "", err
	}

	for _, lease := range dchpLeases {
		if lease.Ipaddr == domIPAddress {
			return lease.Hostname[0], nil
		}
	}
	return "", fmt.Errorf("Failed to find hostname for the DHCP lease with IP %s", domIPAddress)
}
