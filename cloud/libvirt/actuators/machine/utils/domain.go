package machine

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"log"
	"net"
	"os"
	"strings"

	"math/rand"

	"github.com/davecgh/go-spew/spew"
	libvirt "github.com/libvirt/libvirt-go"
	"github.com/libvirt/libvirt-go-xml"
	providerconfigv1 "github.com/openshift/cluster-api-provider-libvirt/cloud/libvirt/providerconfig/v1alpha1"
	clusterv1 "sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1"
)

const (
	baseVolumePath = "/var/lib/libvirt/images/"
)

var LibVirtConIsNil string = "the libvirt connection was nil"

// Client libvirt
type Client struct {
	libvirt *libvirt.Connect
}

type pendingMapping struct {
	mac      string
	hostname string
	network  *libvirt.Network
}

func newDomainDef() libvirtxml.Domain {
	domainDef := libvirtxml.Domain{
		OS: &libvirtxml.DomainOS{
			Type: &libvirtxml.DomainOSType{
				Type: "hvm",
			},
		},
		Memory: &libvirtxml.DomainMemory{
			Unit:  "MiB",
			Value: 512,
		},
		VCPU: &libvirtxml.DomainVCPU{
			Placement: "static",
			Value:     1,
		},
		CPU: &libvirtxml.DomainCPU{},
		Devices: &libvirtxml.DomainDeviceList{
			Graphics: []libvirtxml.DomainGraphic{
				{
					Spice: &libvirtxml.DomainGraphicSpice{
						AutoPort: "yes",
					},
				},
			},
			Channels: []libvirtxml.DomainChannel{
				{
					Target: &libvirtxml.DomainChannelTarget{
						VirtIO: &libvirtxml.DomainChannelTargetVirtIO{
							Name: "org.qemu.guest_agent.0",
						},
					},
				},
			},
			RNGs: []libvirtxml.DomainRNG{
				{
					Model: "virtio",
					Backend: &libvirtxml.DomainRNGBackend{
						Random: &libvirtxml.DomainRNGBackendRandom{},
					},
				},
			},
		},
		Features: &libvirtxml.DomainFeatureList{
			PAE:  &libvirtxml.DomainFeature{},
			ACPI: &libvirtxml.DomainFeature{},
			APIC: &libvirtxml.DomainFeatureAPIC{},
		},
	}

	if v := os.Getenv("TERRAFORM_LIBVIRT_TEST_DOMAIN_TYPE"); v != "" {
		domainDef.Type = v
	} else {
		domainDef.Type = "kvm"
	}

	return domainDef
}

func getHostArchitecture(virConn *libvirt.Connect) (string, error) {
	type HostCapabilities struct {
		XMLName xml.Name `xml:"capabilities"`
		Host    struct {
			XMLName xml.Name `xml:"host"`
			CPU     struct {
				XMLName xml.Name `xml:"cpu"`
				Arch    string   `xml:"arch"`
			}
		}
	}

	info, err := virConn.GetCapabilities()
	if err != nil {
		return "", err
	}

	capabilities := HostCapabilities{}
	xml.Unmarshal([]byte(info), &capabilities)

	return capabilities.Host.CPU.Arch, nil
}

func getHostCapabilities(virConn *libvirt.Connect) (libvirtxml.Caps, error) {
	// We should perhaps think of storing this on the connect object
	// on first call to avoid the back and forth
	caps := libvirtxml.Caps{}
	capsXML, err := virConn.GetCapabilities()
	if err != nil {
		return caps, err
	}
	xml.Unmarshal([]byte(capsXML), &caps)
	log.Printf("[TRACE] Capabilities of host \n %+v", caps)
	return caps, nil
}

func getGuestForArchType(caps libvirtxml.Caps, arch string, virttype string) (libvirtxml.CapsGuest, error) {
	for _, guest := range caps.Guests {
		log.Printf("[TRACE] Checking for %s/%s against %s/%s\n", arch, virttype, guest.Arch.Name, guest.OSType)
		if guest.Arch.Name == arch && guest.OSType == virttype {
			log.Printf("[DEBUG] Found %d machines in guest for %s/%s", len(guest.Arch.Machines), arch, virttype)
			return guest, nil
		}
	}
	return libvirtxml.CapsGuest{}, fmt.Errorf("[DEBUG] Could not find any guests for architecure type %s/%s", virttype, arch)
}

func getCanonicalMachineName(caps libvirtxml.Caps, arch string, virttype string, targetmachine string) (string, error) {
	log.Printf("[INFO] getCanonicalMachineName")
	guest, err := getGuestForArchType(caps, arch, virttype)
	if err != nil {
		return "", err
	}

	for _, machine := range guest.Arch.Machines {
		if machine.Name == targetmachine {
			if machine.Canonical != "" {
				return machine.Canonical, nil
			}
			return machine.Name, nil
		}
	}
	return "", fmt.Errorf("[WARN] Cannot find machine type %s for %s/%s in %v", targetmachine, virttype, arch, caps)
}

func newDomainDefForConnection(virConn *libvirt.Connect) (libvirtxml.Domain, error) {
	d := newDomainDef()

	arch, err := getHostArchitecture(virConn)
	if err != nil {
		return d, err
	}
	d.OS.Type.Arch = arch

	caps, err := getHostCapabilities(virConn)
	if err != nil {
		return d, err
	}
	guest, err := getGuestForArchType(caps, d.OS.Type.Arch, d.OS.Type.Type)
	if err != nil {
		return d, err
	}

	d.Devices.Emulator = guest.Arch.Emulator

	if len(guest.Arch.Machines) > 0 {
		d.OS.Type.Machine = guest.Arch.Machines[0].Name
	}

	canonicalmachine, err := getCanonicalMachineName(caps, d.OS.Type.Arch, d.OS.Type.Type, d.OS.Type.Machine)
	if err != nil {
		return d, err
	}
	d.OS.Type.Machine = canonicalmachine
	return d, nil
}

func setCoreOSIgnition(domainDef *libvirtxml.Domain, ignKey string) error {
	domainDef.QEMUCommandline = &libvirtxml.DomainQEMUCommandline{
		Args: []libvirtxml.DomainQEMUCommandlineArg{
			{
				Value: "-fw_cfg",
			},
			{
				Value: fmt.Sprintf("name=opt/com.coreos/config,file=%s", ignKey),
			},
		},
	}
	return nil
}

// note, source is not initialized
func newDefDisk(i int) libvirtxml.DomainDisk {
	return libvirtxml.DomainDisk{
		Device: "disk",
		Target: &libvirtxml.DomainDiskTarget{
			Bus: "virtio",
			Dev: fmt.Sprintf("vd%s", diskLetterForIndex(i)),
		},
		Driver: &libvirtxml.DomainDiskDriver{
			Name: "qemu",
			Type: "qcow2",
		},
	}
}

var diskLetters = []rune("abcdefghijklmnopqrstuvwxyz")

const oui = "05abcd"

// diskLetterForIndex return diskLetters for index
func diskLetterForIndex(i int) string {

	q := i / len(diskLetters)
	r := i % len(diskLetters)
	letter := diskLetters[r]

	if q == 0 {
		return fmt.Sprintf("%c", letter)
	}

	return fmt.Sprintf("%s%c", diskLetterForIndex(q-1), letter)
}
func randomWWN(strlen int) string {
	const chars = "abcdef0123456789"
	result := make([]byte, strlen)
	for i := 0; i < strlen; i++ {
		result[i] = chars[rand.Intn(len(chars))]
	}
	return oui + string(result)
}

func setDisks(domainDef *libvirtxml.Domain, virConn *libvirt.Connect, volumeKey string) error {
	disk := newDefDisk(0)
	log.Printf("[INFO] LookupStorageVolByKey")
	diskVolume, err := virConn.LookupStorageVolByKey(volumeKey)
	if err != nil {
		return fmt.Errorf("Can't retrieve volume %s", volumeKey)
	}
	log.Printf("[INFO] diskVolume")
	diskVolumeFile, err := diskVolume.GetPath()
	if err != nil {
		return fmt.Errorf("Error retrieving volume file: %s", err)
	}

	log.Printf("[INFO] DomainDiskSource")
	disk.Source = &libvirtxml.DomainDiskSource{
		File: &libvirtxml.DomainDiskSourceFile{
			File: diskVolumeFile,
		},
	}

	domainDef.Devices.Disks = append(domainDef.Devices.Disks, disk)

	return nil
}

// randomMACAddress returns a randomized MAC address
func randomMACAddress() (string, error) {
	buf := make([]byte, 6)
	_, err := rand.Read(buf)
	if err != nil {
		return "", err
	}

	// set local bit and unicast
	buf[0] = (buf[0] | 2) & 0xfe
	// Set the local bit
	buf[0] |= 2

	// avoid libvirt-reserved addresses
	if buf[0] == 0xfe {
		buf[0] = 0xee
	}

	return fmt.Sprintf("%02x:%02x:%02x:%02x:%02x:%02x",
		buf[0], buf[1], buf[2], buf[3], buf[4], buf[5]), nil
}

// Network interface used to expose a libvirt.Network
type Network interface {
	GetXMLDesc(flags libvirt.NetworkXMLFlags) (string, error)
}

func newDefNetworkfromLibvirt(network Network) (libvirtxml.Network, error) {
	networkXMLDesc, err := network.GetXMLDesc(0)
	if err != nil {
		return libvirtxml.Network{}, fmt.Errorf("Error retrieving libvirt domain XML description: %s", err)
	}
	networkDef := libvirtxml.Network{}
	err = xml.Unmarshal([]byte(networkXMLDesc), &networkDef)
	if err != nil {
		return libvirtxml.Network{}, fmt.Errorf("Error reading libvirt network XML description: %s", err)
	}
	return networkDef, nil
}

// HasDHCP checks if the network has a DHCP server managed by libvirt
func HasDHCP(net libvirtxml.Network) bool {
	if net.Forward != nil {
		if net.Forward.Mode == "nat" || net.Forward.Mode == "route" || net.Forward.Mode == "" {
			return true
		}
	}
	return false
}

// Tries to update first, if that fails, it will add it
func updateOrAddHost(n *libvirt.Network, ip, mac, name string) error {
	err := updateHost(n, ip, mac, name)
	if virErr, ok := err.(libvirt.Error); ok && virErr.Code == libvirt.ERR_OPERATION_INVALID && virErr.Domain == libvirt.FROM_NETWORK {
		return addHost(n, ip, mac, name)
	}
	return err
}

// Adds a new static host to the network
func addHost(n *libvirt.Network, ip, mac, name string) error {
	xmlDesc := getHostXMLDesc(ip, mac, name)
	log.Printf("Adding host with XML:\n%s", xmlDesc)
	return n.Update(libvirt.NETWORK_UPDATE_COMMAND_ADD_LAST, libvirt.NETWORK_SECTION_IP_DHCP_HOST, -1, xmlDesc, libvirt.NETWORK_UPDATE_AFFECT_CURRENT)
}

func getHostXMLDesc(ip, mac, name string) string {
	dd := libvirtxml.NetworkDHCPHost{
		IP:   ip,
		MAC:  mac,
		Name: name,
	}
	tmp := struct {
		XMLName xml.Name `xml:"host"`
		libvirtxml.NetworkDHCPHost
	}{xml.Name{}, dd}
	xml, err := xmlMarshallIndented(tmp)
	if err != nil {
		panic("could not marshall host")
	}
	return xml
}

// return an indented XML
func xmlMarshallIndented(b interface{}) (string, error) {
	buf := new(bytes.Buffer)
	enc := xml.NewEncoder(buf)
	enc.Indent("  ", "    ")
	if err := enc.Encode(b); err != nil {
		return "", fmt.Errorf("could not marshall this:\n%s", spew.Sdump(b))
	}
	return buf.String(), nil
}

// Update a static host from the network
func updateHost(n *libvirt.Network, ip, mac, name string) error {
	xmlDesc := getHostXMLDesc(ip, mac, name)
	log.Printf("Updating host with XML:\n%s", xmlDesc)
	return n.Update(libvirt.NETWORK_UPDATE_COMMAND_MODIFY, libvirt.NETWORK_SECTION_IP_DHCP_HOST, -1, xmlDesc, libvirt.NETWORK_UPDATE_AFFECT_CURRENT)
}

func setNetworkInterfaces(domainDef *libvirtxml.Domain,
	virConn *libvirt.Connect, partialNetIfaces map[string]*pendingMapping,
	waitForLeases *[]*libvirtxml.DomainInterface,
	networkInterfaceHostname string, networkInterfaceName string, networkInterfaceAddress string, counter int) error {

	// TODO: support more than 1 interface
	for i := 0; i < 1; i++ {
		netIface := libvirtxml.DomainInterface{
			Model: &libvirtxml.DomainInterfaceModel{
				Type: "virtio",
			},
		}

		// calculate the MAC address
		var err error
		mac, err := randomMACAddress()
		if err != nil {
			return fmt.Errorf("Error generating mac address: %s", err)
		}
		netIface.MAC = &libvirtxml.DomainInterfaceMAC{
			Address: mac,
		}

		if networkInterfaceName != "" {
			// when using a "network_id" we are referring to a "network resource"
			// we have defined somewhere else...
			network, err := virConn.LookupNetworkByName(networkInterfaceName)
			if err != nil {
				return fmt.Errorf("Can't retrieve network name %s", networkInterfaceName)
			}
			defer network.Free()

			networkName, err := network.GetName()
			if err != nil {
				return fmt.Errorf("Error retrieving network name: %s", err)
			}
			networkDef, err := newDefNetworkfromLibvirt(network)

			if HasDHCP(networkDef) {
				hostname := domainDef.Name
				if networkInterfaceHostname != "" {
					hostname = networkInterfaceHostname
				}
				if networkInterfaceAddress != "" {
					// some IP(s) provided
					ip := net.ParseIP(networkInterfaceAddress)
					ip = ip.To4()
					// check ip != nil
					ip[3] = byte(int(ip[3]) + counter)
					log.Printf("[INFO] Increasing IP %s", ip.String())
					if ip == nil {
						return fmt.Errorf("Could not parse addresses '%s'", networkInterfaceAddress)
					}

					log.Printf("[INFO] Adding IP/MAC/host=%s/%s/%s to %s", ip.String(), mac, hostname, networkName)
					if err := updateOrAddHost(network, ip.String(), mac, hostname); err != nil {
						return err
					}
				} else {
					// no IPs provided: if the hostname has been provided, wait until we get an IP
					wait := false
					for _, iface := range *waitForLeases {
						if iface == &netIface {
							wait = true
							break
						}
					}
					if !wait {
						return fmt.Errorf("Cannot map '%s': we are not waiting for DHCP lease and no IP has been provided", hostname)
					}
					// the resource specifies a hostname but not an IP, so we must wait until we
					// have a valid lease and then read the IP we have been assigned, so we can
					// do the mapping
					log.Printf("[DEBUG] Do not have an IP for '%s' yet: will wait until DHCP provides one...", hostname)
					partialNetIfaces[strings.ToUpper(mac)] = &pendingMapping{
						mac:      strings.ToUpper(mac),
						hostname: hostname,
						network:  network,
					}
				}
			}
			netIface.Source = &libvirtxml.DomainInterfaceSource{
				Network: &libvirtxml.DomainInterfaceSourceNetwork{
					Network: networkName,
				},
			}
		}
		domainDef.Devices.Interfaces = append(domainDef.Devices.Interfaces, netIface)
	}

	return nil
}

// Config struct for the libvirt-provider
type Config struct {
	URI string
}

// Client libvirt, generate libvirt client given URI
func buildClient(uri string) (*Client, error) {
	libvirtClient, err := libvirt.NewConnect(uri)
	if err != nil {
		return nil, err
	}
	log.Println("[INFO] Created libvirt client")

	client := &Client{
		libvirt: libvirtClient,
	}

	return client, nil
}

// MachineConfigProviderFromClusterAPIMachineSpec gets the machine provider config MachineSetSpec from the
// specified cluster-api MachineSpec.
func MachineProviderConfigFromClusterAPIMachineSpec(ms *clusterv1.MachineSpec) (*providerconfigv1.LibvirtMachineProviderConfig, error) {
	if ms.ProviderConfig.Value == nil {
		return nil, fmt.Errorf("no Value in ProviderConfig")
	}
	obj, gvk, err := providerconfigv1.Codecs.UniversalDecoder(providerconfigv1.SchemeGroupVersion).Decode([]byte(ms.ProviderConfig.Value.Raw), nil, nil)
	if err != nil {
		return nil, err
	}
	spec, ok := obj.(*providerconfigv1.LibvirtMachineProviderConfig)
	if !ok {
		return nil, fmt.Errorf("unexpected object when parsing machine provider config: %#v", gvk)
	}
	return spec, nil
}

func domainDefInit(domainDef *libvirtxml.Domain, name string, machineConfig providerconfigv1.LibvirtMachineProviderConfig) error {
	if name != "" {
		domainDef.Name = name
	} else {
		return fmt.Errorf("machine does not have an name set")
	}

	if machineConfig.DomainMemory != 0 {
		domainDef.Memory = &libvirtxml.DomainMemory{
			Value: uint(machineConfig.DomainMemory),
			Unit:  "MiB",
		}
	} else {
		return fmt.Errorf("machine does not have an DomainMemory set")
	}

	if machineConfig.DomainVcpu != 0 {
		domainDef.VCPU = &libvirtxml.DomainVCPU{
			Value: machineConfig.DomainVcpu,
		}
	} else {
		return fmt.Errorf("machine does not have an DomainVcpu set")
	}

	domainDef.Devices.Emulator = "/usr/bin/kvm-spice"
	//setConsoles(d, &domainDef)
	//setCmdlineArgs(d, &domainDef)
	//setFirmware(d, &domainDef)
	//setBootDevices(d, &domainDef)

	return nil
}

func createDomain(name string, machineProviderConfig *providerconfigv1.LibvirtMachineProviderConfig, counter int) error {
	if name == "" {
		return fmt.Errorf("Failed to create domain, name is empty")
	}
	client, err := buildClient(machineProviderConfig.Uri)
	if err != nil {
		return fmt.Errorf("Failed to build libvirt client: %s", err)
	}
	log.Printf("[DEBUG] Create resource libvirt_domain")
	virConn := client.libvirt

	// Get default values from Host
	domainDef, err := newDomainDefForConnection(virConn)
	if err != nil {
		return fmt.Errorf("Failed to newDomainDefForConnection: %s", err)
	}

	// Get values from machineProviderConfig
	if err := domainDefInit(&domainDef, name, *machineProviderConfig); err != nil {
		return fmt.Errorf("Failed to init domain definition from machineProviderConfig: %v", err)
	}

	log.Printf("[INFO] setCoreOSIgnition")
	if machineProviderConfig.IgnKey != "" {
		if err := setCoreOSIgnition(&domainDef, machineProviderConfig.IgnKey); err != nil {
			return err
		}
	} else {
		return fmt.Errorf("machine does not has a IgnKey value")
	}

	log.Printf("[INFO] setDisks")
	var volumeName string
	if machineProviderConfig.Volume.VolumeName != "" {
		volumeName = machineProviderConfig.Volume.VolumeName
	} else {
		volumeName = name
	}
	VolumeKey := fmt.Sprintf(baseVolumePath+"%s", volumeName)
	if err := setDisks(&domainDef, virConn, VolumeKey); err != nil {
		return fmt.Errorf("Failed to setDisks: %s", err)
	}

	log.Printf("[INFO] setNetworkInterfaces")
	var waitForLeases []*libvirtxml.DomainInterface
	var hostName string
	if machineProviderConfig.NetworkInterfaceHostname != "" {
		hostName = machineProviderConfig.NetworkInterfaceHostname
	} else {
		hostName = name
	}
	// TODO: support more than 1 interface
	partialNetIfaces := make(map[string]*pendingMapping, 1)
	if err := setNetworkInterfaces(&domainDef, virConn, partialNetIfaces, &waitForLeases,
		hostName, machineProviderConfig.NetworkInterfaceName,
		machineProviderConfig.NetworkInterfaceAddress, counter); err != nil {
		return err
	}

	// TODO: support setFilesystems
	//if err := setFilesystems(d, &domainDef); err != nil {
	//	return err
	//}

	connectURI, err := virConn.GetURI()
	if err != nil {
		return fmt.Errorf("error retrieving libvirt connection URI: %v", err)
	}
	log.Printf("[INFO] Creating libvirt domain at %s", connectURI)

	data, err := xmlMarshallIndented(domainDef)
	if err != nil {
		return fmt.Errorf("error serializing libvirt domain: %v", err)
	}

	log.Printf("[DEBUG] Creating libvirt domain with XML:\n%s", data)
	domain, err := virConn.DomainDefineXML(data)
	if err != nil {
		return fmt.Errorf("error defining libvirt domain: %v", err)
	}

	if err := domain.SetAutostart(machineProviderConfig.Autostart); err != nil {
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

	log.Printf("[INFO] Domain ID: %s", id)
	return nil
}

func deleteDomain(name string, machineProviderConfig *providerconfigv1.LibvirtMachineProviderConfig) error {
	log.Printf("[DEBUG] Delete a domain")

	client, err := buildClient(machineProviderConfig.Uri)
	if err != nil {
		return fmt.Errorf("Failed to build libvirt client: %s", err)
	}

	virConn := client.libvirt
	if virConn == nil {
		return fmt.Errorf(LibVirtConIsNil)
	}

	log.Printf("[DEBUG] Deleting domain %s", name)

	domain, err := virConn.LookupDomainByName(name)
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
			log.Printf("libvirt does not support undefine flags: will try again without flags")
			if err := domain.Undefine(); err != nil {
				return fmt.Errorf("Couldn't undefine libvirt domain: %s", err)
			}
		} else {
			return fmt.Errorf("Couldn't undefine libvirt domain with flags: %s", err)
		}
	}

	return nil
}

func CreateVolumeAndMachine(machine *clusterv1.Machine, counter int) error {
	machineProviderConfig, err := MachineProviderConfigFromClusterAPIMachineSpec(&machine.Spec)
	if err != nil {
		return fmt.Errorf("error getting machineProviderConfig from spec: %v", err)
	}

	if err := createVolume(machine.Name, machineProviderConfig); err != nil {
		return fmt.Errorf("error creating volume: %v", err)
	}

	if err = createDomain(machine.Name, machineProviderConfig, counter); err != nil {
		return fmt.Errorf("error creating domain: %v", err)
	}
	return nil
}

func DeleteVolumeAndDomain(machine *clusterv1.Machine) error {
	machineProviderConfig, err := MachineProviderConfigFromClusterAPIMachineSpec(&machine.Spec)
	if err != nil {
		return fmt.Errorf("error getting machineProviderConfig from spec: %v", err)
	}

	if err := deleteDomain(machine.Name, machineProviderConfig); err != nil {
		return fmt.Errorf("error deleting domain: %v", err)
	}

	if err := deleteVolume(machine.Name, machineProviderConfig.Uri); err != nil {
		return fmt.Errorf("error deleting domain: %v", err)
	}
	return nil
}

func DomainExists(machine *clusterv1.Machine) (bool, error) {
	log.Printf("[DEBUG] Check if a domain exists")

	machineProviderConfig, err := MachineProviderConfigFromClusterAPIMachineSpec(&machine.Spec)
	client, err := buildClient(machineProviderConfig.Uri)
	if err != nil {
		return false, fmt.Errorf("Failed to build libvirt client: %s", err)
	}

	virConn := client.libvirt
	if virConn == nil {
		return false, fmt.Errorf(LibVirtConIsNil)
	}

	domain, err := virConn.LookupDomainByName(machine.Name)
	if err != nil {
		if err.(libvirt.Error).Code == libvirt.ERR_NO_DOMAIN {
			return false, nil
		}
		return false, err
	}
	defer domain.Free()

	return true, nil
}
