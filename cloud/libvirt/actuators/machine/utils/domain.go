package libvirt

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"log"
	"net"
	"os"
	"strings"
	"time"

	"math/rand"

	"github.com/davecgh/go-spew/spew"
	libvirt "github.com/libvirt/libvirt-go"
	"github.com/libvirt/libvirt-go-xml"
	"github.com/openshift/cluster-api-provider-libvirt/lib/cidr"
)

// LibVirtConIsNil contains a nil connection error message
var LibVirtConIsNil = "the libvirt connection was nil"

func init() {
	rand.Seed(time.Now().UTC().UnixNano())
}

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
	var serialPort uint
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
			Consoles: []libvirtxml.DomainConsole{
				libvirtxml.DomainConsole{
					Source: &libvirtxml.DomainChardevSource{
						Pty: &libvirtxml.DomainChardevSourcePty{},
					},
					Target: &libvirtxml.DomainConsoleTarget{
						Type: "virtio",
						Port: &serialPort,
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
	if ignKey == "" {
		return fmt.Errorf("error setting coreos ignition, ignKey is empty")
	}
	domainDef.QEMUCommandline = &libvirtxml.DomainQEMUCommandline{
		Args: []libvirtxml.DomainQEMUCommandlineArg{
			{
				// https://github.com/qemu/qemu/blob/master/docs/specs/fw_cfg.txt
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

func setDisks(domainDef *libvirtxml.Domain, diskVolume libvirt.StorageVol) error {
	disk := newDefDisk(0)
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

func setNetworkInterfaces(domainDef *libvirtxml.Domain,
	virConn *libvirt.Connect, partialNetIfaces map[string]*pendingMapping,
	waitForLeases *[]*libvirtxml.DomainInterface,
	networkInterfaceHostname string, networkInterfaceName string, networkInterfaceAddress string, offset int) error {

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
				log.Printf("Networkaddress %v", networkInterfaceAddress)
				if networkInterfaceAddress != "" {
					_, networkCIDR, err := net.ParseCIDR(networkInterfaceAddress)
					if err != nil {
						return fmt.Errorf("failed to parse libvirt network ipRange: %v", err)
					}
					var ip net.IP
					if ip, err = cidr.GenerateIP(networkCIDR, offset); err != nil {
						return fmt.Errorf("failed to generate ip: %v", err)
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
func BuildClient(URI string) (*Client, error) {
	libvirtClient, err := libvirt.NewConnect(URI)
	if err != nil {
		return nil, err
	}
	log.Println("[INFO] Created libvirt client")

	client := &Client{
		libvirt: libvirtClient,
	}

	return client, nil
}

func domainDefInit(domainDef *libvirtxml.Domain, name string, memory, vcpu int) error {
	if name != "" {
		domainDef.Name = name
	} else {
		return fmt.Errorf("machine does not have an name set")
	}

	if memory != 0 {
		domainDef.Memory = &libvirtxml.DomainMemory{
			Value: uint(memory),
			Unit:  "MiB",
		}
	} else {
		return fmt.Errorf("machine does not have an DomainMemory set")
	}

	if vcpu != 0 {
		domainDef.VCPU = &libvirtxml.DomainVCPU{
			Value: vcpu,
		}
	} else {
		return fmt.Errorf("machine does not have an DomainVcpu set")
	}

	//setConsoles(d, &domainDef)
	//setCmdlineArgs(d, &domainDef)
	//setFirmware(d, &domainDef)
	//setBootDevices(d, &domainDef)

	return nil
}

func CreateDomain(name, ignKey, poolName string, volumeName, hostName, networkInterfaceName, networkInterfaceAddress string, autostart bool, memory, vcpu, offset int, client *Client) error {
	if name == "" {
		return fmt.Errorf("Failed to create domain, name is empty")
	}
	log.Printf("[DEBUG] Create resource libvirt_domain")
	virConn := client.libvirt

	// Get default values from Host
	domainDef, err := newDomainDefForConnection(virConn)
	if err != nil {
		return fmt.Errorf("Failed to newDomainDefForConnection: %s", err)
	}

	// Get values from machineProviderConfig
	if err := domainDefInit(&domainDef, name, memory, vcpu); err != nil {
		return fmt.Errorf("Failed to init domain definition from machineProviderConfig: %v", err)
	}

	log.Printf("[INFO] setCoreOSIgnition")
	if ignKey != "" {
		ignVolume, err := getVolumeFromPool(ignKey, poolName, virConn)
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
	} else {
		return fmt.Errorf("machine does not has a IgnKey value")
	}

	log.Printf("[INFO] setDisks")
	diskVolume, err := getVolumeFromPool(volumeName, poolName, virConn)
	if err != nil {
		return fmt.Errorf("can't retrieve volume %s for pool %s: %v", volumeName, poolName, err)
	}
	log.Printf("[INFO] diskVolume")
	if err := setDisks(&domainDef, *diskVolume); err != nil {
		return fmt.Errorf("Failed to setDisks: %s", err)
	}

	log.Printf("[INFO] setNetworkInterfaces")
	var waitForLeases []*libvirtxml.DomainInterface
	if hostName == "" {
		hostName = name
	}
	// TODO: support more than 1 interface
	partialNetIfaces := make(map[string]*pendingMapping, 1)
	if err := setNetworkInterfaces(&domainDef, virConn, partialNetIfaces, &waitForLeases,
		hostName, networkInterfaceName,
		networkInterfaceAddress, offset); err != nil {
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

	if err := domain.SetAutostart(autostart); err != nil {
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

func DeleteDomain(name string, client *Client) error {
	log.Printf("[DEBUG] Delete a domain")
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

// DomainExists verify a domain exists for given machine
func DomainExists(name string, client *Client) (bool, error) {
	log.Printf("[DEBUG] Check if a domain exists")
	virConn := client.libvirt
	if virConn == nil {
		return false, fmt.Errorf(LibVirtConIsNil)
	}

	domain, err := virConn.LookupDomainByName(name)
	if err != nil {
		if err.(libvirt.Error).Code == libvirt.ERR_NO_DOMAIN {
			return false, nil
		}
		return false, err
	}
	defer domain.Free()

	return true, nil
}
