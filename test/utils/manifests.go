package utils

import (
	"fmt"

	providerconfigv1 "github.com/openshift/cluster-api-provider-libvirt/pkg/apis/libvirtproviderconfig/v1beta1"
	machinev1 "github.com/openshift/machine-api-operator/pkg/apis/machine/v1beta1"
)

func TestingMachineProviderSpec(uri, clusterID string) (machinev1.ProviderSpec, error) {
	machinePc := &providerconfigv1.LibvirtMachineProviderConfig{
		DomainMemory: 2048,
		DomainVcpu:   1,
		CloudInit: &providerconfigv1.CloudInit{
			SSHAccess: true,
		},
		Volume: &providerconfigv1.Volume{
			PoolName:     "default",
			BaseVolumeID: "fedora_base",
		},
		NetworkInterfaceName:    "default",
		NetworkInterfaceAddress: "192.168.124.12/24",
		Autostart:               false,
		URI:                     uri,
	}

	codec, err := providerconfigv1.NewCodec()
	if err != nil {
		return machinev1.ProviderSpec{}, fmt.Errorf("failed creating codec: %v", err)
	}
	config, err := codec.EncodeToProviderSpec(machinePc)
	if err != nil {
		return machinev1.ProviderSpec{}, fmt.Errorf("codec.EncodeToProviderSpec failed: %v", err)
	}
	return *config, nil
}

func MasterMachineProviderSpec(masterUserDataSecret, libvirturi string) (machinev1.ProviderSpec, error) {
	machinePc := &providerconfigv1.LibvirtMachineProviderConfig{
		DomainMemory: 2048,
		DomainVcpu:   2,
		CloudInit: &providerconfigv1.CloudInit{
			SSHAccess:      true,
			UserDataSecret: masterUserDataSecret,
		},
		Volume: &providerconfigv1.Volume{
			PoolName:     "default",
			BaseVolumeID: "fedora_base",
		},
		NetworkInterfaceName:    "default",
		NetworkInterfaceAddress: "192.168.122.0/24",
		Autostart:               false,
		URI:                     libvirturi,
	}

	codec, err := providerconfigv1.NewCodec()
	if err != nil {
		return machinev1.ProviderSpec{}, fmt.Errorf("failed creating codec: %v", err)
	}
	config, err := codec.EncodeToProviderSpec(machinePc)
	if err != nil {
		return machinev1.ProviderSpec{}, fmt.Errorf("codec.EncodeToProviderSpec failed: %v", err)
	}
	return *config, nil
}

func WorkerMachineProviderSpec(workerUserDataSecret, libvirturi string) (machinev1.ProviderSpec, error) {
	return MasterMachineProviderSpec(workerUserDataSecret, libvirturi)
}
