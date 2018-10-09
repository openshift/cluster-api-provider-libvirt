package utils

import (
	"encoding/base64"
	"fmt"

	"k8s.io/client-go/kubernetes"

	libvirtxml "github.com/libvirt/libvirt-go-xml"
	providerconfigv1 "github.com/openshift/cluster-api-provider-libvirt/cloud/libvirt/providerconfig/v1alpha1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Max firmware config string is set to 1024.
// Round it to 1020 for some extra waste space.
const maxFirmwareCfgChunkSize = 1020

func breakUserDataIntoChunks(userData string, chunkMaxSize int) []string {
	userDataLen := len(userData)
	chunks := userDataLen / chunkMaxSize

	b := make([]string, chunks)
	for i := 0; i < chunks; i++ {
		b[i] = userData[i*chunkMaxSize : (i+1)*chunkMaxSize]
	}

	if userDataLen%chunkMaxSize > 0 {
		b = append(b, userData[chunks*chunkMaxSize:userDataLen])
	}
	return b
}

func setCloudInit(domainDef *libvirtxml.Domain, cloudInit *providerconfigv1.CloudInit, kubeClient kubernetes.Interface, machineNamespace string) error {
	if cloudInit.ISOImagePath == "" {
		return fmt.Errorf("error setting cloud-init, ISO image path is empty")
	}

	domainDef.Devices.Disks = append(domainDef.Devices.Disks, libvirtxml.DomainDisk{
		Device: "cdrom",
		Driver: &libvirtxml.DomainDiskDriver{
			Name: "qemu",
			Type: "raw",
		},
		Source: &libvirtxml.DomainDiskSource{
			File: &libvirtxml.DomainDiskSourceFile{
				File: cloudInit.ISOImagePath,
			},
		},
		Target: &libvirtxml.DomainDiskTarget{
			Dev: "hdd",
			Bus: "ide",
		},
	})

	if cloudInit.UserDataSecret != "" {
		userDataSecret, err := kubeClient.CoreV1().Secrets(machineNamespace).Get(cloudInit.UserDataSecret, metav1.GetOptions{})
		if err != nil {
			return fmt.Errorf("can not retrieve user data secret '%v/%v' when constructing cloud init volume: %v", machineNamespace, cloudInit.UserDataSecret, err)
		}

		userDataEnc := base64.StdEncoding.EncodeToString(userDataSecret.Data["userData"])
		// Only first 1024 characters of the string in the fw_cfg option are used, the rest is ignored.
		// Also see https://github.com/coreos/bugs/issues/2083#issuecomment-380973018
		// Thus, the secret needs to be broken into chunks of size of at most 1024 chars
		chunks := breakUserDataIntoChunks(userDataEnc, maxFirmwareCfgChunkSize)
		domainDef.QEMUCommandline = &libvirtxml.DomainQEMUCommandline{}
		for idx, chunk := range chunks {
			domainDef.QEMUCommandline.Args = append(domainDef.QEMUCommandline.Args, libvirtxml.DomainQEMUCommandlineArg{
				// https://github.com/qemu/qemu/blob/master/docs/specs/fw_cfg.txt
				Value: "-fw_cfg",
			})
			domainDef.QEMUCommandline.Args = append(domainDef.QEMUCommandline.Args, libvirtxml.DomainQEMUCommandlineArg{
				Value: fmt.Sprintf("name=opt/actuator.libvirt.io.k8s.sigs/config%0*d,string=%s", len(chunks), idx, chunk),
			})
		}
	}

	return nil
}
