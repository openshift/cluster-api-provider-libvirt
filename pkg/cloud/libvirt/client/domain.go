package client

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"net"
	"os"
	"strings"
	"time"

	"math/rand"

	"github.com/davecgh/go-spew/spew"
	"github.com/golang/glog"
	libvirt "github.com/libvirt/libvirt-go"
	libvirtxml "github.com/libvirt/libvirt-go-xml"
	"github.com/openshift/cluster-api-provider-libvirt/lib/cidr"
)

// ErrLibVirtConIsNil is returned when the libvirt connection is nil.
var ErrLibVirtConIsNil = errors.New("the libvirt connection was nil")

// ErrDomainNotFound is returned when a domain is not found
var ErrDomainNotFound = errors.New("Domain not found")

func init() {
	rand.Seed(time.Now().UTC().UnixNano())
}

type pendingMapping struct {
	mac      string
	hostname string
	network  *libvirt.Network
}

func newDomainDef(virConn *libvirt.Connect) libvirtxml.Domain {
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
		CPU:     &libvirtxml.DomainCPU{},
		Devices: newDevicesDef(virConn),
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

func newDevicesDef(virConn *libvirt.Connect) *libvirtxml.DomainDeviceList {
	var serialPort uint

	domainList := libvirtxml.DomainDeviceList{
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
			{
				Source: &libvirtxml.DomainChardevSource{
					Pty: &libvirtxml.DomainChardevSourcePty{},
				},
				Target: &libvirtxml.DomainConsoleTarget{
					Type: "virtio",
					Port: &serialPort,
				},
			},
		},
	}

	arch, err := getHostArchitecture(virConn)
	if err != nil {
		glog.Errorf("Error retrieving host architecture: %s", err)
	}
	// Both "s390" and "s390x" are linux kernel architectures for Linux on IBM z Systems, and they are for 31-bit and 64-bit respectively.
	// Graphics/Spice isn't supported on s390/s390x platform.
	// Same case for PowerPC systems as well
	if !strings.HasPrefix(arch, "s390") && !strings.HasPrefix(arch, "ppc64") {
		domainList.Graphics = []libvirtxml.DomainGraphic{
			{
				Spice: &libvirtxml.DomainGraphicSpice{
					AutoPort: "yes",
				},
			},
		}
	}

	return &domainList
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
	glog.Infof("Capabilities of host \n %+v", caps)
	return caps, nil
}

func getGuestForArchType(caps libvirtxml.Caps, arch string, virttype string) (libvirtxml.CapsGuest, error) {
	for _, guest := range caps.Guests {
		glog.Infof("Checking for %s/%s against %s/%s", arch, virttype, guest.Arch.Name, guest.OSType)
		if guest.Arch.Name == arch && guest.OSType == virttype {
			glog.Infof("Found %d machines in guest for %s/%s", len(guest.Arch.Machines), arch, virttype)
			return guest, nil
		}
	}
	return libvirtxml.CapsGuest{}, fmt.Errorf("Could not find any guests for architecure type %s/%s", virttype, arch)
}

func getCanonicalMachineName(caps libvirtxml.Caps, arch string, virttype string, targetmachine string) (string, error) {
	glog.Info("Get machine name")
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
	return "", fmt.Errorf("Cannot find machine type %s for %s/%s in %v", targetmachine, virttype, arch, caps)
}

func newDomainDefForConnection(virConn *libvirt.Connect) (libvirtxml.Domain, error) {
	d := newDomainDef(virConn)

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

func setCoreOSIgnition(domainDef *libvirtxml.Domain, ignKey string, arch string) error {
	if ignKey == "" {
		return fmt.Errorf("error setting coreos ignition, ignKey is empty")
	}
	if strings.HasPrefix(arch, "s390") || strings.HasPrefix(arch, "ppc64") {
		// System Z and PowerPC do not support the Firmware Configuration
		// device. After a discussion about the best way to support a similar
		// method for qemu in https://github.com/coreos/ignition/issues/928,
		// decided on creating a virtio-blk device with a serial of ignition
		// which contains the ignition config and have ignition support for
		// reading from the device which landed in https://github.com/coreos/ignition/pull/936
		igndisk := libvirtxml.DomainDisk{
			Device: "disk",
			Source: &libvirtxml.DomainDiskSource{
				File: &libvirtxml.DomainDiskSourceFile{
					File: ignKey,
				},
			},
			Target: &libvirtxml.DomainDiskTarget{
				Dev: "vdb",
				Bus: "virtio",
			},
			Driver: &libvirtxml.DomainDiskDriver{
				Name: "qemu",
				Type: "raw",
			},
			ReadOnly: &libvirtxml.DomainDiskReadOnly{},
			Serial:   "ignition",
		}
		domainDef.Devices.Disks = append(domainDef.Devices.Disks, igndisk)
	} else {
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

func setDisks(domainDef *libvirtxml.Domain, diskVolume *libvirt.StorageVol) error {
	disk := newDefDisk(0)
	glog.Info("Getting disk volume")
	diskVolumeFile, err := diskVolume.GetPath()
	if err != nil {
		return fmt.Errorf("Error retrieving volume file: %s", err)
	}

	glog.Info("Constructing domain disk source")
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

func setNetworkInterfaces(
	domainDef *libvirtxml.Domain,
	virConn *libvirt.Connect,
	partialNetIfaces map[string]*pendingMapping,
	waitForLeases *[]*libvirtxml.DomainInterface,
	networkInterfaceHostname string,
	networkInterfaceName string,
	networkInterfaceAddress string,
	reservedLeases *Leases,
) error {

	// TODO: support more than 1 interface
	for i := 0; i < 1; i++ {
		netIface := libvirtxml.DomainInterface{
			Model: &libvirtxml.DomainInterfaceModel{
				Type: "virtio",
			},
		}

		// calculate the MAC address
		// TODO: possible MAC collision
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
			if err != nil {
				return fmt.Errorf("Error retrieving network definition: %v", err)
			}

			if HasDHCP(networkDef) {
				hostname := domainDef.Name
				if networkInterfaceHostname != "" {
					hostname = networkInterfaceHostname
				}
				glog.Infof("Networkaddress: %v", networkInterfaceAddress)
				if networkInterfaceAddress != "" {
					_, networkCIDR, err := net.ParseCIDR(networkInterfaceAddress)
					if err != nil {
						return fmt.Errorf("failed to parse libvirt network ipRange: %v", err)
					}

					// generate new IP's until we will have the IP that not leased to another machine
					var ip net.IP
					baseWokerIPCidr := workerIPCidr
					for {
						ip, err = cidr.GenerateIP(networkCIDR, baseWokerIPCidr)
						if err != nil {
							return fmt.Errorf("failed to generate ip: %v", err)
						}
						if _, ok := reservedLeases.Items[ip.String()]; !ok {
							break
						}
						baseWokerIPCidr++
					}

					// add generated IP to reserved leases map
					reservedLeases.Lock()
					reservedLeases.Items[ip.String()] = ""
					reservedLeases.Unlock()

					glog.Infof("Adding IP/MAC/host=%s/%s/%s to %s", ip.String(), mac, hostname, networkName)
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
					glog.Infof("Do not have an IP for '%s' yet: will wait until DHCP provides one...", hostname)
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

// UEFI ROM image and NVRAM settings
func setFirmware(input *CreateDomainInput, domainDef *libvirtxml.Domain) {
	if input.Firmware != "" {
		domainDef.OS.Loader = &libvirtxml.DomainLoader{
			Path:     input.Firmware,
			Readonly: "yes",
			Type:     "pflash",
			Secure:   "no",
		}
		if input.Nvram != nil {
			domainDef.OS.NVRam = &libvirtxml.DomainNVRam{
				NVRam:    input.Nvram.NvramFile,
				Template: input.Nvram.NvramTemplate,
			}
		}
	}
}

func domainDefInit(domainDef *libvirtxml.Domain, input *CreateDomainInput) error {
	if input.DomainName != "" {
		domainDef.Name = input.DomainName
	} else {
		return fmt.Errorf("machine does not have an name set")
	}

	if input.DomainMemory != 0 {
		domainDef.Memory = &libvirtxml.DomainMemory{
			Value: uint(input.DomainMemory),
			Unit:  "MiB",
		}
	} else {
		return fmt.Errorf("machine does not have an DomainMemory set")
	}

	if input.DomainVcpu != 0 {
		domainDef.VCPU = &libvirtxml.DomainVCPU{
			Value: input.DomainVcpu,
		}
	} else {
		return fmt.Errorf("machine does not have an DomainVcpu set")
	}

	domainDef.CPU.Mode = "host-passthrough"
	setFirmware(input, domainDef)

	//setConsoles(d, &domainDef)
	//setCmdlineArgs(d, &domainDef)
	//setBootDevices(d, &domainDef)

	return nil
}
