package utils

import (
	"encoding/xml"
	"fmt"
	"log"
	"math/rand"
	"net"

	libvirt "github.com/libvirt/libvirt-go"
	libvirtxml "github.com/libvirt/libvirt-go-xml"
)

const (
	netModeIsolated = "none"
	netModeNat      = "nat"
	netModeRoute    = "route"
	netModeBridge   = "bridge"
)

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
	xmlDesc, err := getHostXMLDesc(ip, mac, name)
	if err != nil {
		return fmt.Errorf("error getting host xml desc: %v", err)
	}
	log.Printf("Adding host with XML:\n%s", xmlDesc)
	return n.Update(libvirt.NETWORK_UPDATE_COMMAND_ADD_LAST, libvirt.NETWORK_SECTION_IP_DHCP_HOST, -1, xmlDesc, libvirt.NETWORK_UPDATE_AFFECT_CURRENT)
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
func updateHost(n *libvirt.Network, ip, mac, name string) error {
	xmlDesc, err := getHostXMLDesc(ip, mac, name)
	if err != nil {
		return fmt.Errorf("error getting host xml desc: %v", err)
	}
	log.Printf("Updating host with XML:\n%s", xmlDesc)
	return n.Update(libvirt.NETWORK_UPDATE_COMMAND_MODIFY, libvirt.NETWORK_SECTION_IP_DHCP_HOST, -1, xmlDesc, libvirt.NETWORK_UPDATE_AFFECT_CURRENT)
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

// Creates a network definition from a XML
func newDefNetworkFromXML(s string) (libvirtxml.Network, error) {
	var networkDef libvirtxml.Network
	err := xml.Unmarshal([]byte(s), &networkDef)
	if err != nil {
		return libvirtxml.Network{}, err
	}
	return networkDef, nil
}

// Creates a network definition with the defaults the provider uses
func newNetworkDef() (libvirtxml.Network, error) {
	const defNetworkXML = `
		<network>
		  <name>default</name>
		  <forward mode='nat'>
		    <nat>
		      <port start='1024' end='65535'/>
		    </nat>
		  </forward>
		</network>`
	if d, err := newDefNetworkFromXML(defNetworkXML); err != nil {
		return libvirtxml.Network{}, fmt.Errorf("unexpected error while parsing default network definition: %v", err)
	} else {
		return d, nil
	}
}

func CreateNetwork(name, domain, bridge, mode string, addresses []string, autostart bool, client *Client) error {
	// see https://libvirt.org/formatnetwork.html
	networkDef, err := newNetworkDef()
	if err != nil {
		return fmt.Errorf("error creating network definition %v", err)
	}
	networkDef.Name = name
	networkDef.Domain = &libvirtxml.NetworkDomain{
		Name: domain,
	}

	networkDef.Bridge = &libvirtxml.NetworkBridge{
		Name: bridge,
		STP:  "on",
	}

	// check the network mode
	networkDef.Forward = &libvirtxml.NetworkForward{
		Mode: mode,
	}
	if networkDef.Forward.Mode == netModeIsolated || networkDef.Forward.Mode == netModeNat || networkDef.Forward.Mode == netModeRoute {

		if networkDef.Forward.Mode == netModeIsolated {
			// there is no forwarding when using an isolated network
			networkDef.Forward = nil
		} else if networkDef.Forward.Mode == netModeRoute {
			// there is no NAT when using a routed network
			networkDef.Forward.NAT = nil
		}

		// some network modes require a DHCP/DNS server
		// set the addresses for DHCP
		if len(addresses) > 0 {
			ipsPtrsLst := []libvirtxml.NetworkIP{}
			for _, address := range addresses {
				_, ipNet, err := net.ParseCIDR(address)
				if err != nil {
					return fmt.Errorf("error parsing addresses definition %q: %v", address, err)
				}
				ones, bits := ipNet.Mask.Size()
				family := "ipv4"
				if bits == (net.IPv6len * 8) {
					family = "ipv6"
				}
				ipsRange := 2 ^ bits - 2 ^ ones
				if ipsRange < 4 {
					return fmt.Errorf("Netmask seems to be too strict: only %d IPs available (%s)", ipsRange-3, family)
				}

				// we should calculate the range served by DHCP. For example, for
				// 192.168.121.0/24 we will serve 192.168.121.2 - 192.168.121.254
				start, end := networkRange(ipNet)

				// skip the .0, (for the network),
				start[len(start)-1]++

				// assign the .1 to the host interface
				dni := libvirtxml.NetworkIP{
					Address: start.String(),
					Prefix:  uint(ones),
					Family:  family,
				}

				start[len(start)-1]++ // then skip the .1
				end[len(end)-1]--     // and skip the .255 (for broadcast)

				dni.DHCP = &libvirtxml.NetworkDHCP{
					Ranges: []libvirtxml.NetworkDHCPRange{
						{
							Start: start.String(),
							End:   end.String(),
						},
					},
				}
				ipsPtrsLst = append(ipsPtrsLst, dni)
			}
			networkDef.IPs = ipsPtrsLst
		}

	} else if networkDef.Forward.Mode == netModeBridge {
		if bridge == "" {
			return fmt.Errorf("'bridge' must be provided when using the bridged network mode")
		}
		// Bridges cannot forward
		networkDef.Forward = nil
	} else {
		return fmt.Errorf("unsupported network mode '%s'", networkDef.Forward.Mode)
	}

	// once we have the network defined, connect to libvirt and create it from the XML serialization
	connectURI, err := client.connection.GetURI()
	if err != nil {
		return fmt.Errorf("Error retrieving libvirt connection URI: %s", err)
	}
	log.Printf("[INFO] Creating libvirt network at %s", connectURI)

	data, err := xmlMarshallIndented(networkDef)
	if err != nil {
		return fmt.Errorf("Error serializing libvirt network: %s", err)
	}

	log.Printf("[DEBUG] Creating libvirt network at %s: %s", connectURI, data)
	network, err := client.connection.NetworkDefineXML(data)
	if err != nil {
		return fmt.Errorf("Error defining libvirt network: %s - %s", err, data)
	}
	err = network.Create()
	if err != nil {
		return fmt.Errorf("Error crearing libvirt network: %s", err)
	}
	defer network.Free()

	id, err := network.GetUUIDString()
	if err != nil {
		return fmt.Errorf("Error retrieving libvirt network id: %s", err)
	}
	log.Printf("[INFO] Created network %s [%s]", networkDef.Name, id)

	if autostart {
		err = network.SetAutostart(autostart)
		if err != nil {
			return fmt.Errorf("Error setting autostart for network: %s", err)
		}
	}
	return nil
}

// networkRange calculates the first and last IP addresses in an IPNet
func networkRange(network *net.IPNet) (net.IP, net.IP) {
	netIP := network.IP.To4()
	lastIP := net.IPv4(0, 0, 0, 0).To4()
	if netIP == nil {
		netIP = network.IP.To16()
		lastIP = net.IPv6zero.To16()
	}
	firstIP := netIP.Mask(network.Mask)
	for i := 0; i < len(lastIP); i++ {
		lastIP[i] = netIP[i] | ^network.Mask[i]
	}
	return firstIP, lastIP
}

func DeleteNetwork(name string, client *Client) error {
	network, err := client.connection.LookupNetworkByName(name)
	if err != nil {
		return fmt.Errorf("When destroying libvirt network: error retrieving %s", err)
	}
	defer network.Free()

	active, err := network.IsActive()
	if err != nil {
		return fmt.Errorf("Couldn't determine if network is active: %s", err)
	}
	if !active {
		// we have to restart an inactive network, otherwise it won't be
		// possible to remove it.
		if err := network.Create(); err != nil {
			return fmt.Errorf("Cannot restart an inactive network %s", err)
		}
	}

	if err := network.Destroy(); err != nil {
		return fmt.Errorf("When destroying libvirt network: %s", err)
	}

	if err := network.Undefine(); err != nil {
		return fmt.Errorf("Couldn't undefine libvirt network: %s", err)
	}

	if err != nil {
		return fmt.Errorf("Error waiting for network to reach NOT-EXISTS state: %s", err)
	}

	log.Printf("deleted network %s", name)
	return nil
}
