package utils

import (
	"bytes"
	"fmt"

	providerconfigv1 "github.com/openshift/cluster-api-provider-libvirt/cloud/libvirt/providerconfig/v1alpha1"
	"k8s.io/apimachinery/pkg/runtime"
	clusterv1alpha1 "sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1"
)

func TestingMachineProviderConfig(uri, clusterID string) (clusterv1alpha1.ProviderConfig, error) {
	machinePc := &providerconfigv1.LibvirtMachineProviderConfig{
		DomainMemory: 2048,
		DomainVcpu:   1,
		IgnKey:       "/var/lib/libvirt/images/worker.ign",
		Volume: &providerconfigv1.Volume{
			PoolName:     "default",
			BaseVolumeID: "/var/lib/libvirt/images/coreos_base",
		},
		NetworkInterfaceName:    "tectonic",
		NetworkInterfaceAddress: "192.168.124.12",
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
