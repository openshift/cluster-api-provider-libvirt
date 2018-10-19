package utils

import (
	"bytes"
	"fmt"

	providerconfigv1 "github.com/openshift/cluster-api-provider-libvirt/pkg/cloud/libvirt/providerconfig/v1alpha1"
	"k8s.io/apimachinery/pkg/runtime"
	clusterv1alpha1 "sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1"
)

func TestingMachineProviderConfig(uri, clusterID string) (clusterv1alpha1.ProviderConfig, error) {
	machinePc := &providerconfigv1.LibvirtMachineProviderConfig{
		DomainMemory: 2048,
		DomainVcpu:   1,
		CloudInit: &providerconfigv1.CloudInit{
			ISOImagePath: "/var/lib/libvirt/images/cloud-init.iso",
		},
		Volume: &providerconfigv1.Volume{
			PoolName:     "default",
			BaseVolumeID: "/var/lib/libvirt/images/fedora_base",
		},
		NetworkInterfaceName:    "default",
		NetworkInterfaceAddress: "192.168.124.12/24",
		Autostart:               false,
		URI:                     uri,
	}

	var buf bytes.Buffer
	if err := providerconfigv1.Encoder.Encode(machinePc, &buf); err != nil {
		return clusterv1alpha1.ProviderConfig{}, fmt.Errorf("LibvirtMachineProviderConfig encoding failed: %v", err)
	}

	return clusterv1alpha1.ProviderConfig{
		Value: &runtime.RawExtension{Raw: buf.Bytes()},
	}, nil
}

func MasterMachineProviderConfig(masterUserDataSecret, libvirturi string) (clusterv1alpha1.ProviderConfig, error) {
	machinePc := &providerconfigv1.LibvirtMachineProviderConfig{
		DomainMemory: 2048,
		DomainVcpu:   2,
		CloudInit: &providerconfigv1.CloudInit{
			ISOImagePath:   "/var/lib/libvirt/images/cloud-init.iso",
			UserDataSecret: masterUserDataSecret,
		},
		Volume: &providerconfigv1.Volume{
			PoolName:     "default",
			BaseVolumeID: "/var/lib/libvirt/images/fedora_base",
		},
		NetworkInterfaceName:    "default",
		NetworkInterfaceAddress: "192.168.122.0/24",
		Autostart:               false,
		URI:                     libvirturi,
	}

	var buf bytes.Buffer
	if err := providerconfigv1.Encoder.Encode(machinePc, &buf); err != nil {
		return clusterv1alpha1.ProviderConfig{}, fmt.Errorf("LibvirtMachineProviderConfig encoding failed: %v", err)
	}

	return clusterv1alpha1.ProviderConfig{
		Value: &runtime.RawExtension{Raw: buf.Bytes()},
	}, nil
}

func WorkerMachineProviderConfig(workerUserDataSecret, libvirturi string) (clusterv1alpha1.ProviderConfig, error) {
	return MasterMachineProviderConfig(workerUserDataSecret, libvirturi)
}
