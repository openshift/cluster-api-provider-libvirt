package machines

import (
	"fmt"

	libvirt "github.com/libvirt/libvirt-go"
	clusterv1alpha1 "sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1"
)

type libvirtClient struct {
	conn *libvirt.Connect
}

func NewLibvirtClient(uri string) (*libvirtClient, error) {
	conn, err := libvirt.NewConnect(uri)
	if err != nil {
		return nil, err
	}

	return &libvirtClient{
		conn: conn,
	}, nil
}

func (client *libvirtClient) GetRunningInstances(machine *clusterv1alpha1.Machine) ([]interface{}, error) {
	domain, err := client.getRunningDomain(machine.Name)
	if err != nil {
		return nil, err
	}

	var instances []interface{} = make([]interface{}, 1)
	instances[0] = domain

	return instances, nil
}

func (client *libvirtClient) GetPublicDNSName(machine *clusterv1alpha1.Machine) (string, error) {
	return client.GetPrivateIP(machine)
}

func (client *libvirtClient) GetPrivateIP(machine *clusterv1alpha1.Machine) (string, error) {
	domain, err := client.getRunningDomain(machine.Name)
	if err != nil {
		return "", err
	}

	domainInterfaces, err := domain.ListAllInterfaceAddresses(libvirt.DOMAIN_INTERFACE_ADDRESSES_SRC_LEASE)
	if err != nil {
		return "", err
	}

	if len(domainInterfaces) == 0 {
		return "", fmt.Errorf("no domain interface for machine instance found")
	}

	domainInterface := domainInterfaces[0]
	if len(domainInterface.Addrs) == 0 || domainInterface.Addrs[0].Addr == "" {
		return "", fmt.Errorf("no address for machine instances domain interface found")
	}

	return domainInterface.Addrs[0].Addr, nil
}

func (client *libvirtClient) getRunningDomain(name string) (*libvirt.Domain, error) {
	domain, err := client.conn.LookupDomainByName(name)
	if err != nil {
		return nil, fmt.Errorf("error retrieving libvirt domain: %s", err)
	}

	state, _, err := domain.GetState()
	if err != nil {
		return nil, fmt.Errorf("couldn't get info about domain: %s", err)
	}

	if state != libvirt.DOMAIN_RUNNING {
		return nil, fmt.Errorf("no running machine instance found")
	}

	return domain, nil
}
