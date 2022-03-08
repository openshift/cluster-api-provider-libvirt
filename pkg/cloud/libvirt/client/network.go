package client

import (
	"encoding/xml"
	"fmt"
	"math/rand"
	"sync"

	"github.com/golang/glog"

	libvirt "github.com/digitalocean/go-libvirt"
	libvirtxml "github.com/libvirt/libvirt-go-xml"
)

const (
	netModeIsolated = "none"
	netModeNat      = "nat"
	netModeRoute    = "route"
	netModeBridge   = "bridge"
	workerIPCidr    = 51
)

// Leases contains list of DHCP leases
type Leases struct {
	Items map[string]string
	sync.Mutex
}

// Network interface used to expose a libvirt.Network
type Network interface {
	GetXMLDesc(flags libvirt.NetworkXMLFlags) (string, error)
}

func newDefNetworkfromLibvirt(virtConn *libvirt.Libvirt, network libvirt.Network) (libvirtxml.Network, error) {
	networkXMLDesc, err := virtConn.NetworkGetXMLDesc(network, 0)
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
func updateOrAddHost(virtConn *libvirt.Libvirt, n *libvirt.Network, ip, mac, name string) error {
	err := updateHost(virtConn, n, ip, mac, name)
	if virErr, ok := err.(libvirt.Error); ok && virErr.Code == uint32(libvirt.ErrOperationInvalid) {
		return addHost(virtConn, n, ip, mac, name)
	}
	return err
}

// Adds a new static host to the network
func addHost(virtConn *libvirt.Libvirt, n *libvirt.Network, ip, mac, name string) error {
	xmlDesc, err := getHostXMLDesc(ip, mac, name)
	if err != nil {
		return fmt.Errorf("error getting host xml desc: %v", err)
	}
	glog.Infof("Adding host with XML:\n%s", xmlDesc)
	return virtConn.NetworkUpdate(*n, uint32(libvirt.NetworkUpdateCommandAddLast), uint32(libvirt.NetworkSectionIPDhcpHost), -1, xmlDesc, libvirt.NetworkUpdateAffectCurrent)
}

func getHostXMLDesc(ip, mac, name string) (string, error) {
	networkDHCPHost := libvirtxml.NetworkDHCPHost{
		IP:   ip,
		MAC:  mac,
		Name: name,
	}
	tmp := struct {
		XMLName xml.Name `xml:"host"`
		libvirtxml.NetworkDHCPHost
	}{xml.Name{}, networkDHCPHost}
	xml, err := xmlMarshallIndented(tmp)
	if err != nil {
		return "", fmt.Errorf("could not marshall: %v", err)
	}
	return xml, nil
}

// Update a static host from the network
func updateHost(virtConn *libvirt.Libvirt, n *libvirt.Network, ip, mac, name string) error {
	xmlDesc, err := getHostXMLDesc(ip, mac, name)
	if err != nil {
		return fmt.Errorf("error getting host xml desc: %v", err)
	}
	glog.Infof("Updating host with XML:\n%s", xmlDesc)
	return virtConn.NetworkUpdate(*n, uint32(libvirt.NetworkUpdateCommandModify), uint32(libvirt.NetworkSectionIPDhcpHost), -1, xmlDesc, libvirt.NetworkUpdateAffectCurrent)
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

// FillReservedLeases will fill Leases structure with existing DHCP leases
func FillReservedLeases(leases *Leases, libvirtLeases []libvirt.NetworkDhcpLease) {
	leases.Lock()
	for _, libvirtLease := range libvirtLeases {
		leases.Items[libvirtLease.Ipaddr] = ""
	}
	leases.Unlock()
}
