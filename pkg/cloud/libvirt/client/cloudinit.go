package client

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"text/template"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/golang/glog"

	libvirtxml "github.com/libvirt/libvirt-go-xml"
	providerconfigv1 "github.com/openshift/cluster-api-provider-libvirt/pkg/apis/libvirtproviderconfig/v1beta1"
)

func setCloudInit(ctx context.Context, domainDef *libvirtxml.Domain, client *libvirtClient, cloudInit *providerconfigv1.CloudInit, kubeClient kubernetes.Interface, machineNamespace, volumeName, domainName string) error {

	// At least user data or ssh access needs to be set to create the cloud init
	if cloudInit.UserDataSecret == "" && !cloudInit.SSHAccess {
		return nil
	}

	// default to bash noop
	userDataSecret := []byte(":")
	if cloudInit.UserDataSecret != "" {
		secret, err := kubeClient.CoreV1().Secrets(machineNamespace).Get(ctx, cloudInit.UserDataSecret, metav1.GetOptions{})
		if err != nil {
			return fmt.Errorf("can not retrieve user data secret '%v/%v' when constructing cloud init volume: %v", machineNamespace, cloudInit.UserDataSecret, err)
		}
		ok := false
		userDataSecret, ok = secret.Data["userData"]
		if !ok {
			return fmt.Errorf("can not retrieve user data secret '%v/%v' when constructing cloud init volume: key 'data' not found in the secret", machineNamespace, cloudInit.UserDataSecret)
		}
	}

	userData, err := renderCloudInitStr(userDataSecret, cloudInit.SSHAccess)
	if err != nil {
		return fmt.Errorf("can not render cloud init user-data: %v", err)
	}

	metaData, err := renderMetaDataStr(domainName)
	if err != nil {
		return fmt.Errorf("can not render cloud init meta-data: %v", err)
	}

	cloudInitISOName := volumeName

	cloudInitDef := newCloudInitDef()
	cloudInitDef.UserData = string(userData)
	cloudInitDef.MetaData = string(metaData)
	cloudInitDef.Name = cloudInitISOName
	cloudInitDef.PoolName = client.poolName

	glog.Infof("cloudInitDef: %+v", cloudInitDef)

	iso, err := cloudInitDef.createISO()
	if err != nil {
		return fmt.Errorf("unable to create ISO %v: %v", cloudInitISOName, err)
	}

	key, err := cloudInitDef.uploadIso(client, iso)
	if err != nil {
		return fmt.Errorf("unable to upload ISO: %v", err)
	}
	glog.Infof("key: %+v", key)

	domainDef.Devices.Disks = append(domainDef.Devices.Disks, libvirtxml.DomainDisk{
		Device: "cdrom",
		Driver: &libvirtxml.DomainDiskDriver{
			Name: "qemu",
			Type: "raw",
		},
		Source: &libvirtxml.DomainDiskSource{
			File: &libvirtxml.DomainDiskSourceFile{
				File: key,
			},
		},
		Target: &libvirtxml.DomainDiskTarget{
			Dev: "hdd",
			Bus: "ide",
		},
	})

	return nil
}

const userDataFileName string = "user-data"
const metaDataFileName string = "meta-data"
const networkConfigFileName string = "network-config"

type defCloudInit struct {
	Name          string
	PoolName      string
	MetaData      string `yaml:"meta_data"`
	UserData      string `yaml:"user_data"`
	NetworkConfig string `yaml:"network_config"`
}

func newCloudInitDef() defCloudInit {
	return defCloudInit{}
}

// Create the ISO holding all the cloud-init data
// Returns a string with the full path to the ISO file
func (ci *defCloudInit) createISO() (string, error) {
	glog.Info("Creating new ISO")
	tmpDir, err := ci.createFiles()
	if err != nil {
		return "", err
	}

	isoDestination := filepath.Join(tmpDir, ci.Name)
	cmd := exec.Command(
		"mkisofs",
		"-output",
		isoDestination,
		"-volid",
		"cidata",
		"-joliet",
		"-rock",
		filepath.Join(tmpDir, userDataFileName),
		filepath.Join(tmpDir, metaDataFileName),
		filepath.Join(tmpDir, networkConfigFileName))

	glog.Infof("About to execute cmd: %+v", cmd)
	if err = cmd.Run(); err != nil {
		return "", fmt.Errorf("error while starting the creation of CloudInit's ISO image: %s", err)
	}
	glog.Infof("ISO created at %s", isoDestination)

	return isoDestination, nil
}

// write user-data,  meta-data network-config in tmp files and dedicated directory
// Returns a string containing the name of the temporary directory and an error
// object
func (ci *defCloudInit) createFiles() (string, error) {
	glog.Info("Creating ISO contents")
	tmpDir, err := ioutil.TempDir("", "cloudinit")
	if err != nil {
		return "", fmt.Errorf("Cannot create tmp directory for cloudinit ISO generation: %s",
			err)
	}
	// user-data
	if err = ioutil.WriteFile(filepath.Join(tmpDir, userDataFileName), []byte(ci.UserData), os.ModePerm); err != nil {
		return "", fmt.Errorf("Error while writing user-data to file: %s", err)
	}
	// meta-data
	if err = ioutil.WriteFile(filepath.Join(tmpDir, metaDataFileName), []byte(ci.MetaData), os.ModePerm); err != nil {
		return "", fmt.Errorf("Error while writing meta-data to file: %s", err)
	}
	// network-config
	if err = ioutil.WriteFile(filepath.Join(tmpDir, networkConfigFileName), []byte(ci.NetworkConfig), os.ModePerm); err != nil {
		return "", fmt.Errorf("Error while writing network-config to file: %s", err)
	}

	glog.Info("ISO contents created")

	return tmpDir, nil
}

func (ci *defCloudInit) uploadIso(client *libvirtClient, iso string) (string, error) {
	volumeDef := newDefVolume(ci.Name)

	// an existing image was given, this mean we can't choose size
	img, err := newImage(iso)
	if err != nil {
		return "", err
	}

	defer removeTmpIsoDirectory(iso)

	size, err := img.size()
	if err != nil {
		return "", err
	}

	volumeDef.Capacity.Unit = "B"
	volumeDef.Capacity.Value = size
	volumeDef.Target.Format.Type = "raw"

	return uploadVolume(ci.PoolName, client, volumeDef, img)
}

func removeTmpIsoDirectory(iso string) {
	err := os.RemoveAll(filepath.Dir(iso))
	if err != nil {
		glog.Infof("Error while removing tmp directory holding the ISO file: %s", err)
	}
}

type cloudInitParams struct {
	UserDataScript string
	SSHAccess      bool
}

func renderCloudInitStr(userDataScript []byte, sshAccess bool) (string, error) {
	// The bash script is rendered into cloud init file so it needs to be
	// base64 encoded to avoid interpretation.
	userDataEnc := base64.StdEncoding.EncodeToString(userDataScript)

	params := cloudInitParams{
		UserDataScript: userDataEnc,
		SSHAccess:      sshAccess,
	}
	t, err := template.New("cloudinit").Parse(defaultCloudInitStr)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	err = t.Execute(&buf, params)
	if err != nil {
		return "", err
	}

	return buf.String(), nil
}

const defaultCloudInitStr = `
#cloud-config

# Hostname management
preserve_hostname: False
hostname: whatever
fqdn: whatever.example.local

runcmd:
  # Set the hostname to its IP address so every kubernetes node has unique name
  - hostnamectl set-hostname $(ip route get 1 | cut -d' ' -f7)
  # Run the user data script
  - echo '{{ .UserDataScript }}' | base64 -d | bash
  # Remove cloud-init when finished with it
  - [ yum, -y, remove, cloud-init ]

# Configure where output will go
output:
  all: ">> /var/log/cloud-init.log"

{{ if .SSHAccess }}
# configure interaction with ssh server
ssh_svcname: ssh
ssh_deletekeys: True
ssh_genkeytypes: ['rsa', 'ecdsa']

# Install public ssh key to the first user-defined user configured
# in cloud.cfg in the template (which is fedpra for Fedora cloud images)
ssh_authorized_keys:
  - ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAACAQCkvgGhhYwEjWjD+ACW8s+DIanHqYJIC7RbgBRrvAqJQuWE87jfTtREHuW+o0qU1eIPPJzebu58VPgy3SscnrN2fKuMT2PAkevmjj4ARQmdsR/BBrmzdibe/Wnd8WEMNX82L+YrkuHoVkgafFkreSZgf/j8glGNl7IQe5gi2XDG1e+BQ+e94dxAExeRlldhQsbFvQJ+qLmDhHE4zdf/d/CqY6PwoIHlrOVLux7/pBV5SGg5eKlGCPi80oEf23LbwHYjkUXzEreBqUrWSwsdp6jIQ9zzADRQJ0+C47K6uwxy1RIe3q6t7f1eJwjmOaYYS2Sc+U1cpPHrWY3OzZJkbIZ3Fva8qVdbqhMW2ASqJ7oGpdwiRp7FTvoKlEktcc6JUK19sZ6dft79PF9nRy8nfz4obKowCZn7aqVBOW41DhaoC5oB9pfBgSPnObGnpkXITWrx/oUQ1zwrPIH150X3XuDdYXfrmDk/k+cQS7hjG328pfJs8oBhqUmyikUxjnXvDX/LQzacwDF3XKCy6Xq98bemFp8lnAG7c3tW8tYpn3Non6M3XaS2W/ece9JRZKOOCaqC52U7sg6nL/Yv11Sg9WSfJtINzNN1cKxZsIaPvorPflwqNlLWH3dPCb4KQry/54HCBvsKm1+s/yud31zk9C/CI5bFV959bLq+6ra6hAMBTw== Libvirt guest key
{{ end }}
`

type metaDataParams struct {
	InstanceID string
}

func renderMetaDataStr(instanceID string) (string, error) {
	params := metaDataParams{
		InstanceID: instanceID,
	}

	t, err := template.New("metadata").Parse(defaultMetaDataStr)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	err = t.Execute(&buf, params)
	if err != nil {
		return "", err
	}

	return buf.String(), nil
}

const defaultMetaDataStr = `
instance-id: {{ .InstanceID }}; local-hostname: {{ .InstanceID }}
`
