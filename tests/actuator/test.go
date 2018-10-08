package main

import (
	"fmt"
	libvirtutils "github.com/openshift/cluster-api-provider-libvirt/cloud/libvirt/actuators/machine/utils"
	actuator "github.com/openshift/cluster-api-provider-libvirt/cmd/libvirt-actuator/utils"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"os"
	"os/exec"
	"path"
	"strings"
)

// Test cases

const defaultLogLevel = "debug"
const defaultlibvirtdURI = "qemu:///system"

var rootCmd = &cobra.Command{
	Use:   "actuator-test",
	Short: "Test the actuator without going through the cluster API",
	RunE: func(cmd *cobra.Command, args []string) error {
		logLevel := cmd.Flag("log-level").Value.String()
		libvirtdURI := cmd.Flag("libvirtd-uri").Value.String()
		manifestsDir := cmd.Flag("manifests-dir").Value.String()
		log.SetOutput(os.Stdout)
		if lvl, err := log.ParseLevel(logLevel); err != nil {
			log.Panic(err)
		} else {
			log.SetLevel(lvl)
		}

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
			log.Errorf("failed creating actuator infra assumtions %v", err)
			//return err
		}
		defer destroyActuatorInfraAssumtions(client)

		for _, test := range testCases {
			// create machine to manually test
			cluster, machine, err := actuator.ReadClusterResources(
				path.Join(manifestsDir, "cluster.yaml"),
				path.Join(manifestsDir, test.machineFile),
			)
			name := machine.Name
			if err != nil {
				log.Errorf("failed reading cluster resources %v", err)
				return err
			}

			actuator := actuator.CreateActuator(machine, log.WithField("example", "create-machine"))
			err = actuator.Create(cluster, machine)
			if err != nil {
				return err
			}
			log.Info("machine creation was successful!")

			// validations
			// the actuator reports the machine exists
			exists, err := actuator.Exists(cluster, machine)
			if err != nil {
				log.Errorf("error checking if machine %s exists", name)
				return err
			}
			if exists != true {
				log.Errorf("machine %s expected but not found", name)
				return err
			}
			log.Infof("verified the actuator reporrs the machine %s exists", name)

			// there's a volume with the domain name
			exists, err = libvirtutils.DomainExists(name, client)
			if err != nil {
				log.Errorf("error checking if domain %s exists", name)
				return err
			}
			if exists != true {
				log.Errorf("domain %s expected but not found", name)
				return err
			}
			log.Infof("verified that domain %s exists", name)

			// verify is reachable, TODO(alberto) find a better way to do this
			out, _ := exec.Command("ping", test.expectedReachableIP, "-c 5", "-i 5").Output()
			if !strings.Contains(string(out), "bytes from") {
				log.Errorf("domain %s not reachable for ip %s, %s", name, test.expectedReachableIP, string(out))
				return err
			}
			log.Infof("verified domain %s is reachable for ip %s", name, test.expectedReachableIP)
			// TODO(alberto) ssh and verify ignition config

			err = actuator.Delete(cluster, machine)
			if err != nil {
				return err
			}
			log.Info("machine deletion was successful!")
		}
		return nil

	},
}

func init() {
	rootCmd.PersistentFlags().StringP("log-level", "l", defaultLogLevel, "Log level (debug,info,warn,error,fatal)")
	rootCmd.PersistentFlags().StringP("libvirtd-uri", "u", defaultlibvirtdURI, "Libvirtd url (qemu+tcp://ip/system)")
	rootCmd.PersistentFlags().StringP("manifests-dir", "m", ".", "./examples")
}

func createActuatorInfraAssumtions(client *libvirtutils.Client) error {
	// TODO(alberto) Create poolName: default

	// Create base volume from image (libvirt_base_volume)
	if err := libvirtutils.CreateVolume("baseVolume", "default", "", "file:///root/coreos_production_qemu_image.img", "qcow2", client); err != nil {
		log.Errorf("failed creating base volume %v", err)
		return err
	}

	// Create ign volume
	content := `{"ignition":{"version":"2.2.0"},"networkd":{},"passwd":{"users":[{"name":"core","passwordHash": "$6$Jez3bVF7jG$ncmvBeJiYbzFKZSQKzTwg9gJ2qoF4N.JYyt8iv4qCThCdJmOtxnZz3l1W3btoh9.bXE8DcdXr6iXuV7ES4kww0"}]},"storage":{},"systemd":{}}`
	if err := libvirtutils.CreateIgntion("default", "worker.ign", content, client); err != nil {
		log.Errorf("failed creating ignition %v", err)
		return err
	}

	// Create networkInterfaceName
	if err := libvirtutils.CreateNetwork("actuatorTestNetwork", "tt.test", "tt0", "nat", []string{"192.168.124.0/24"}, false, client); err != nil {
		log.Errorf("failed creating network %v", err)
		return err
	}
	return nil
}

func destroyActuatorInfraAssumtions(client *libvirtutils.Client) {
	// Delete base volume
	if err := libvirtutils.DeleteVolume("baseVolume", client); err != nil {
		log.Errorf("failed deleting base volume %v", err)
	}

	// Delete ign volume
	if err := libvirtutils.DeleteVolume("worker.ign", client); err != nil {
		log.Errorf("failed creating ignition %v", err)
	}

	// Delete networkInterfaceName
	if err := libvirtutils.DeleteNetwork("actuatorTestNetwork", client); err != nil {
		log.Errorf("failed creating network %v", err)
	}
}

func main() {
	err := rootCmd.Execute()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error occurred: %v\n", err)
		os.Exit(1)
	}
}
