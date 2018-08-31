/*
Copyright 2018 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

// Tests individual Libvirt actuator actions

import (
	"fmt"
	"io/ioutil"
	"os"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	clusterv1 "sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1"

	"github.com/ghodss/yaml"
	machineactuator "github.com/openshift/cluster-api-provider-libvirt/cloud/libvirt/actuators/machine"

	"sigs.k8s.io/cluster-api/pkg/client/clientset_generated/clientset/fake"
)

func usage() {
	fmt.Printf("Usage: %s\n\n", os.Args[0])
}

var rootCmd = &cobra.Command{
	Use:   "libvirt-actuator-test",
	Short: "Test for Cluster API Libvirt actuator",
}

func init() {
	rootCmd.PersistentFlags().StringP("machine", "m", "", "Machine manifest")
	rootCmd.PersistentFlags().StringP("cluster", "c", "", "Cluster manifest")

	rootCmd.AddCommand(&cobra.Command{
		Use:   "create",
		Short: "Create machine instance for specified cluster",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := checkFlags(cmd); err != nil {
				return err
			}
			cluster, machine, err := readClusterResources(
				cmd.Flag("cluster").Value.String(),
				cmd.Flag("machine").Value.String(),
			)
			if err != nil {
				return err
			}

			actuator := createActuator(machine, log.WithField("example", "create-machine"))
			err = actuator.Create(cluster, machine)
			if err != nil {
				return err
			}
			fmt.Printf("Machine creation was successful!")
			return nil
		},
	})

	rootCmd.AddCommand(&cobra.Command{
		Use:   "delete INSTANCE-ID",
		Short: "Delete machine instance",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := checkFlags(cmd); err != nil {
				return err
			}
			cluster, machine, err := readClusterResources(
				cmd.Flag("cluster").Value.String(),
				cmd.Flag("machine").Value.String(),
			)
			if err != nil {
				return err
			}

			actuator := createActuator(machine, log.WithField("example", "create-machine"))
			err = actuator.Delete(cluster, machine)
			if err != nil {
				return err
			}
			fmt.Printf("Machine delete operation was successful.\n")
			return nil
		},
	})

	rootCmd.AddCommand(&cobra.Command{
		Use:   "exists",
		Short: "Determine if underlying machine instance exists",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := checkFlags(cmd); err != nil {
				return err
			}
			cluster, machine, err := readClusterResources(
				cmd.Flag("cluster").Value.String(),
				cmd.Flag("machine").Value.String())
			if err != nil {
				return err
			}

			actuator := createActuator(machine, log.WithField("example", "create-machine"))
			exists, err := actuator.Exists(cluster, machine)
			if err != nil {
				return err
			}
			if exists {
				fmt.Printf("Underlying machine's instance exists.\n")
			} else {
				fmt.Printf("Underlying machine's instance not found.\n")
			}
			return nil
		},
	})
}

func readClusterResources(clusterLoc, machineLoc string) (*clusterv1.Cluster, *clusterv1.Machine, error) {
	machine := &clusterv1.Machine{}
	{
		bytes, err := ioutil.ReadFile(machineLoc)
		if err != nil {
			return nil, nil, fmt.Errorf("machine manifest %q: %v", machineLoc, err)
		}

		if err = yaml.Unmarshal(bytes, &machine); err != nil {
			return nil, nil, fmt.Errorf("machine manifest %q: %v", machineLoc, err)
		}
	}

	cluster := &clusterv1.Cluster{}
	{
		bytes, err := ioutil.ReadFile(clusterLoc)
		if err != nil {
			return nil, nil, fmt.Errorf("cluster manifest %q: %v", clusterLoc, err)
		}

		if err = yaml.Unmarshal(bytes, &cluster); err != nil {
			return nil, nil, fmt.Errorf("cluster manifest %q: %v", clusterLoc, err)
		}
	}

	return cluster, machine, nil
}

func createActuator(machine *clusterv1.Machine, logger *log.Entry) *machineactuator.Actuator {
	fakeClient := fake.NewSimpleClientset(machine)
	params := machineactuator.ActuatorParams{fakeClient}
	actuator, _ := machineactuator.NewActuator(params)
	return actuator
}

func checkFlags(cmd *cobra.Command) error {
	if cmd.Flag("cluster").Value.String() == "" {
		return fmt.Errorf("--%v/-%v flag is required", cmd.Flag("cluster").Name, cmd.Flag("cluster").Shorthand)
	}
	if cmd.Flag("machine").Value.String() == "" {
		return fmt.Errorf("--%v/-%v flag is required", cmd.Flag("machine").Name, cmd.Flag("machine").Shorthand)
	}
	return nil
}

func main() {
	log.SetOutput(os.Stdout)
	log.SetLevel(log.DebugLevel)

	err := rootCmd.Execute()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error occurred: %v\n", err)
		os.Exit(1)
	}
}
