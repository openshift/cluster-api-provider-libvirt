package client

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"io/ioutil"
	"os"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/golang/glog"
	libvirt "github.com/libvirt/libvirt-go"
	libvirtxml "github.com/libvirt/libvirt-go-xml"
	providerconfigv1 "github.com/openshift/cluster-api-provider-libvirt/pkg/apis/libvirtproviderconfig/v1beta1"
)

func setIgnition(domainDef *libvirtxml.Domain, client *libvirtClient, ignition *providerconfigv1.Ignition, kubeClient kubernetes.Interface, machineNamespace, volumeName, poolName string) error {
	glog.Infof("creating ignition file")
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
	ignitionDef.PoolName = poolName
	ignitionDef.Content = string(userDataSecret)

	glog.Infof("ignition: %+v", ignitionDef)

	ignitionVolumeName, err := ignitionDef.createAndUpload(client)
	if err != nil {
		return err
	}

	domainDef.QEMUCommandline = &libvirtxml.DomainQEMUCommandline{
		Args: []libvirtxml.DomainQEMUCommandlineArg{
			{
				// https://github.com/qemu/qemu/blob/master/docs/specs/fw_cfg.txt
				Value: "-fw_cfg",
			},
			{
				Value: fmt.Sprintf("name=opt/com.coreos/config,file=%s", ignitionVolumeName),
			},
		},
	}
	return nil
}

type defIgnition struct {
	Name     string
	PoolName string
	Content  string
}

// Creates a new cloudinit with the defaults
// the provider uses
func newIgnitionDef() defIgnition {
	return defIgnition{}
}

// Create a ISO file based on the contents of the CloudInit instance and
// uploads it to the libVirt pool
// Returns a string holding terraform's internal ID of this resource
func (ign *defIgnition) createAndUpload(client *libvirtClient) (string, error) {
	pool, err := client.connection.LookupStoragePoolByName(ign.PoolName)
	if err != nil {
		return "", fmt.Errorf("can't find storage pool %q", ign.PoolName)
	}
	defer pool.Free()

	//client.poolMutexKV.Lock(ign.PoolName)
	//defer client.poolMutexKV.Unlock(ign.PoolName)

	// Refresh the pool of the volume so that libvirt knows it is
	// not longer in use.
	err = waitForSuccess("Error refreshing pool for volume", func() error {
		return pool.Refresh(0)
	})
	if err != nil {
		return "", fmt.Errorf("timeout when calling waitForSuccess: %v", err)
	}

	volumeDef := newDefVolume()
	volumeDef.Name = ign.Name

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

	img, err := newImage(ignFile)
	if err != nil {
		return "", err
	}

	size, err := img.size()
	if err != nil {
		return "", err
	}

	volumeDef.Capacity.Unit = "B"
	volumeDef.Capacity.Value = size
	volumeDef.Target.Format.Type = "raw"

	volumeDefXML, err := xml.Marshal(volumeDef)
	if err != nil {
		return "", fmt.Errorf("Error serializing libvirt volume: %s", err)
	}

	// create the volume
	volume, err := pool.StorageVolCreateXML(string(volumeDefXML), 0)
	if err != nil {
		return "", fmt.Errorf("Error creating libvirt volume for Ignition %s: %s", ign.Name, err)
	}
	defer volume.Free()

	// upload ignition file
	err = img.importImage(newCopier(client.connection, volume, volumeDef.Capacity.Value), volumeDef)
	if err != nil {
		return "", fmt.Errorf("Error while uploading ignition file %s: %s", img.string(), err)
	}

	key, err := volume.GetKey()
	if err != nil {
		return "", fmt.Errorf("Error retrieving volume key: %s", err)
	}
	glog.Infof("Ignition ID: %s", key)
	return key, nil
}

// Dumps the Ignition object to a temporary ignition file
func (ign *defIgnition) createFile() (string, error) {
	glog.Info("Creating Ignition temporary file")
	tempFile, err := ioutil.TempFile("", ign.Name)
	if err != nil {
		return "", fmt.Errorf("Cannot create tmp file for Ignition: %s",
			err)
	}
	defer tempFile.Close()

	var file bool
	file = true
	if _, err := os.Stat(ign.Content); err != nil {
		var js map[string]interface{}
		if errConf := json.Unmarshal([]byte(ign.Content), &js); errConf != nil {
			return "", fmt.Errorf("coreos_ignition 'content' is neither a file "+
				"nor a valid json object %s", ign.Content)
		}
		file = false
	}

	if !file {
		if _, err := tempFile.WriteString(ign.Content); err != nil {
			return "", fmt.Errorf("Cannot write Ignition object to temporary " +
				"ignition file")
		}
	} else if file {
		ignFile, err := os.Open(ign.Content)
		if err != nil {
			return "", fmt.Errorf("Error opening supplied Ignition file %s", ign.Content)
		}
		defer ignFile.Close()
		_, err = io.Copy(tempFile, ignFile)
		if err != nil {
			return "", fmt.Errorf("Error copying supplied Igition file to temporary file: %s", ign.Content)
		}
	}
	return tempFile.Name(), nil
}

func newCopier(virConn *libvirt.Connect, volume *libvirt.StorageVol, size uint64) func(src io.Reader) error {
	copier := func(src io.Reader) error {
		var bytesCopied int64

		stream, err := virConn.NewStream(0)
		if err != nil {
			return err
		}

		defer func() {
			if uint64(bytesCopied) != size {
				stream.Abort()
			} else {
				stream.Finish()
			}
			stream.Free()
		}()

		volume.Upload(stream, 0, size, 0)

		sio := newStreamIO(*stream)

		bytesCopied, err = io.Copy(sio, src)
		if err != nil {
			return err
		}
		glog.Infof("%d bytes uploaded\n", bytesCopied)
		return nil
	}
	return copier
}
