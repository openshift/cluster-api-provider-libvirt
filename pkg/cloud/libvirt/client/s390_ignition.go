package client

import (
	"fmt"
	"github.com/golang/glog"
	libvirt "github.com/libvirt/libvirt-go"
	libvirtxml "github.com/libvirt/libvirt-go-xml"
	providerconfigv1 "github.com/openshift/cluster-api-provider-libvirt/pkg/apis/libvirtproviderconfig/v1beta1"
	"io"
	"io/ioutil"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"os"
	"os/exec"
	"path/filepath"
)

var execCommand = exec.Command

func setIgnitionForS390X(domainDef *libvirtxml.Domain, client *libvirtClient, ignition *providerconfigv1.Ignition, kubeClient kubernetes.Interface, machineNamespace, volumeName string) error {
	glog.Infof("Creating ignition file for s390x")
	ignitionDef := newIgnitionDef()

	if ignition.UserDataSecret == "" {
		return fmt.Errorf("ignition.userDataSecret not set")
	}

	secret, err := kubeClient.CoreV1().Secrets(machineNamespace).Get(ignition.UserDataSecret, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("can not retrieve user data secret '%v/%v' when constructing cloud init volume: %v", machineNamespace, ignition.UserDataSecret, err)
	}
	userDataSecret, ok := secret.Data["userData"]
	if !ok {
		return fmt.Errorf("can not retrieve user data secret '%v/%v' when constructing cloud init volume: key 'userData' not found in the secret", machineNamespace, ignition.UserDataSecret)
	}

	ignitionDef.Name = volumeName
	ignitionDef.PoolName = client.poolName
	ignitionDef.Content = string(userDataSecret)

	glog.Infof("Ignition: %+v", ignitionDef)

	tmpDir, err := ioutil.TempDir("", "config-drive")
	if err != nil {
		return fmt.Errorf("Failed to create config-drive directory: %v", err)
	}
	defer func() {
		if err = os.RemoveAll(tmpDir); err != nil {
			glog.Errorf("Error while removing config-drive directory: %v", err)
		}
	}()

	ignitionVolumeName, err := ignitionDef.createAndUploadIso(tmpDir, client)
	if err != nil {
		return fmt.Errorf("Error create and upload iso file: %s", err)
	}

	glog.Infof("Calling newDiskForConfigDrive for coreos_ignition on s390x ")
	disk, err := newDiskForConfigDrive(client.connection, ignitionVolumeName)
	if err != nil {
		return err
	}

	domainDef.Devices.Disks = append(domainDef.Devices.Disks, disk)

	return nil
}

func (ign *defIgnition) createAndUploadIso(tmpDir string, client *libvirtClient) (string, error) {
	ignFile, err := ign.createFile()
	if err != nil {
		return "", err
	}
	defer func() {
		// Remove the tmp ignition file
		if err = os.Remove(ignFile); err != nil {
			glog.Infof("Error while removing tmp Ignition file: %s", err)
		}
	}()

	isoVolumeFile, err := createIgnitionISO(tmpDir, ignFile)
	if err != nil {
		return "", fmt.Errorf("Error generate iso file: %s", err)
	}

	img, err := newImage(isoVolumeFile)
	if err != nil {
		return "", err
	}

	size, err := img.size()
	if err != nil {
		return "", err
	}

	volumeDef := newDefVolume(ign.Name)
	volumeDef.Capacity.Unit = "B"
	volumeDef.Capacity.Value = size
	volumeDef.Target.Format.Type = "raw"

	return uploadVolume(ign.PoolName, client, volumeDef, img)
}

func newDiskForConfigDrive(virConn *libvirt.Connect, volumeKey string) (libvirtxml.DomainDisk, error) {
	disk := libvirtxml.DomainDisk{
		Device: "cdrom",
		Target: &libvirtxml.DomainDiskTarget{
			// s390 platform doesn't support IDE controller, it shoule be virtio controller
			Dev: "vdb",
			Bus: "scsi",
		},
		Driver: &libvirtxml.DomainDiskDriver{
			Name: "qemu",
			Type: "raw",
		},
	}
	diskVolume, err := virConn.LookupStorageVolByKey(volumeKey)
	if err != nil {
		return disk, fmt.Errorf("Can't retrieve volume %s: %v", volumeKey, err)
	}
	diskVolumeFile, err := diskVolume.GetPath()
	if err != nil {
		return disk, fmt.Errorf("Error retrieving volume file: %s", err)
	}

	disk.Source = &libvirtxml.DomainDiskSource{
		File: &libvirtxml.DomainDiskSourceFile{
			File: diskVolumeFile,
		},
	}

	return disk, nil
}

// createIgnitionISO create config drive iso with ignition-config file
func createIgnitionISO(tmpDir string, ignPath string) (string, error) {
	glog.Infof("The ignPath %s", ignPath)
	//get the ignition contentt
	userData, err := os.Open(ignPath)
	if err != nil {
		return "", fmt.Errorf("Error get the ignition content : %s", err)
	}
	defer userData.Close()
	newDestinationPath, err := os.Create(filepath.Join(tmpDir, "user_data"))
	if _, err := io.Copy(newDestinationPath, userData); err != nil {
		return "", fmt.Errorf("Error copy the ignitio content to newDestinationPath : %s", err)
	}
	configDrivePath := filepath.Join(tmpDir, "config.iso")
	cmd := exec.Command(
		"mkisofs",
		"-output",
		configDrivePath,
		"-volid",
		"config-2",
		"-root",
		"openstack/latest",
		"-joliet",
		"-rock",
		newDestinationPath.Name())
	glog.Infof("Executing command: %+v", cmd)

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("Error while starting the creation of ignition's ISO image: %s", err)
	}
	glog.Infof("ISO created at %s", configDrivePath)
	return configDrivePath, nil
}
