package utils

import (
	"fmt"
	"github.com/ghodss/yaml"
	machineactuator "github.com/openshift/cluster-api-provider-libvirt/cloud/libvirt/actuators/machine"
	log "github.com/sirupsen/logrus"
	"io/ioutil"
	clusterv1 "sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1"
	"sigs.k8s.io/cluster-api/pkg/client/clientset_generated/clientset/fake"
)

func ReadClusterResources(clusterLoc, machineLoc string) (*clusterv1.Cluster, *clusterv1.Machine, error) {
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

func CreateActuator(machine *clusterv1.Machine, logger *log.Entry) *machineactuator.Actuator {
	fakeClient := fake.NewSimpleClientset(machine)
	params := machineactuator.ActuatorParams{
		ClusterClient: fakeClient,
	}
	actuator, _ := machineactuator.NewActuator(params)
	return actuator
}
