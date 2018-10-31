package main

import (
	"fmt"
	"os"
	"os/exec"
	"path"
	"strings"

	"github.com/golang/glog"
	libvirtxml "github.com/libvirt/libvirt-go-xml"
	actuator "github.com/openshift/cluster-api-provider-libvirt/cmd/libvirt-actuator/utils"
	providerconfigv1 "github.com/openshift/cluster-api-provider-libvirt/pkg/apis/libvirtproviderconfig/v1alpha1"
	libvirtutils "github.com/openshift/cluster-api-provider-libvirt/pkg/cloud/libvirt/actuators/machine/utils"
	"github.com/spf13/cobra"
	apiv1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	kubernetesfake "k8s.io/client-go/kubernetes/fake"
)

// Test cases

const defaultlibvirtdURI = "qemu:///system"

var rootCmd = &cobra.Command{
	Use:   "actuator-test",
	Short: "Test the actuator without going through the cluster API",
	RunE: func(cmd *cobra.Command, args []string) error {
		libvirtdURI := cmd.Flag("libvirtd-uri").Value.String()
		manifestsDir := cmd.Flag("manifests-dir").Value.String()

		testCases := []struct {
			machineFile         string
			expectedReachableIP string
		}{
			{
				machineFile:         "machine-with-full-paths.yaml",
				expectedReachableIP: "192.168.124.51", // currently the actuator starts with an offset of 50
			},
		}
		//build libvirtd client
		client, err := libvirtutils.BuildClient(libvirtdURI)
		if err != nil {
			return fmt.Errorf("Failed to build libvirt client: %s", err)
		}

		if err := createActuatorInfraAssumtions(client); err != nil {
			glog.Errorf("failed creating actuator infra assumtions %v", err)
			//return err
		}
		defer destroyActuatorInfraAssumtions(client)

		for _, test := range testCases {
			// create machine to manually test
			cluster, machine, _, err := actuator.ReadClusterResources(
				path.Join(manifestsDir, "cluster.yaml"),
				path.Join(manifestsDir, test.machineFile),
				"",
			)
			name := machine.Name
			if err != nil {
				glog.Errorf("failed reading cluster resources %v", err)
				return err
			}

			actuator := actuator.CreateActuator(machine, nil)
			err = actuator.Create(cluster, machine)
			if err != nil {
				return err
			}
			glog.Info("machine creation was successful!")

			// validations
			// the actuator reports the machine exists
			exists, err := actuator.Exists(cluster, machine)
			if err != nil {
				glog.Errorf("error checking if machine %s exists", name)
				return err
			}
			if exists != true {
				glog.Errorf("machine %s expected but not found", name)
				return err
			}
			glog.Infof("verified the actuator reporrs the machine %s exists", name)

			// there's a volume with the domain name
			exists, err = libvirtutils.DomainExists(name, client)
			if err != nil {
				glog.Errorf("error checking if domain %s exists", name)
				return err
			}
			if exists != true {
				glog.Errorf("domain %s expected but not found", name)
				return err
			}
			glog.Infof("verified that domain %s exists", name)

			// verify is reachable, TODO(alberto) find a better way to do this
			out, _ := exec.Command("ping", test.expectedReachableIP, "-c 5", "-i 5").Output()
			if !strings.Contains(string(out), "bytes from") {
				glog.Errorf("domain %s not reachable for ip %s, %s", name, test.expectedReachableIP, string(out))
				return err
			}
			glog.Infof("verified domain %s is reachable for ip %s", name, test.expectedReachableIP)
			// TODO(alberto) ssh and verify ignition config

			err = actuator.Delete(cluster, machine)
			if err != nil {
				return err
			}
			glog.Info("machine deletion was successful!")
		}
		return nil

	},
}

func init() {
	rootCmd.PersistentFlags().StringP("libvirtd-uri", "u", defaultlibvirtdURI, "Libvirtd url (qemu+tcp://ip/system)")
	rootCmd.PersistentFlags().StringP("manifests-dir", "m", ".", "./examples")
}

func createActuatorInfraAssumtions(client *libvirtutils.Client) error {
	// TODO(alberto) Create poolName: default

	// Create base volume from image (libvirt_base_volume)
	if err := libvirtutils.CreateVolume("baseVolume", "default", "", "file:///root/coreos_production_qemu_image.img", "qcow2", client); err != nil {
		glog.Errorf("failed creating base volume %v", err)
		return err
	}

	// Create ign volume
	content := `{"ignition":{"version":"2.2.0"},"networkd":{},"passwd":{"users":[{"name":"core","passwordHash": "$6$Jez3bVF7jG$ncmvBeJiYbzFKZSQKzTwg9gJ2qoF4N.JYyt8iv4qCThCdJmOtxnZz3l1W3btoh9.bXE8DcdXr6iXuV7ES4kww0"}]},"storage":{},"systemd":{}}`

	userData := &apiv1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "user-data-secret-ignition",
			Namespace: "machineNS",
		},
		Data: map[string][]byte{
			"userData": []byte(content),
		},
	}

	objList := []runtime.Object{}
	if userData != nil {
		objList = append(objList, userData)
	}
	fakeKubeClient := kubernetesfake.NewSimpleClientset(objList...)

	if err := libvirtutils.SetIgnition(&libvirtxml.Domain{}, client, &providerconfigv1.Ignition{UserDataSecret: "user-data-secret-ignition"}, fakeKubeClient, "machineNS", "worker.ign", "default"); err != nil {
		glog.Errorf("failed creating ignition %v", err)
		return err
	}

	// Create networkInterfaceName
	if err := libvirtutils.CreateNetwork("actuatorTestNetwork", "tt.test", "tt0", "nat", []string{"192.168.124.0/24"}, false, client); err != nil {
		glog.Errorf("failed creating network %v", err)
		return err
	}
	return nil
}

func destroyActuatorInfraAssumtions(client *libvirtutils.Client) {
	// Delete base volume
	if err := libvirtutils.DeleteVolume("baseVolume", client); err != nil {
		glog.Errorf("failed deleting base volume %v", err)
	}

	// Delete ign volume
	if err := libvirtutils.DeleteVolume("worker.ign", client); err != nil {
		glog.Errorf("failed creating ignition %v", err)
	}

	// Delete networkInterfaceName
	if err := libvirtutils.DeleteNetwork("actuatorTestNetwork", client); err != nil {
		glog.Errorf("failed creating network %v", err)
	}
}

func main() {
	err := rootCmd.Execute()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error occurred: %v\n", err)
		os.Exit(1)
	}
}
