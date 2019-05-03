package utils

import (
	"fmt"
	"io/ioutil"

	apiv1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/ghodss/yaml"
	"github.com/golang/glog"
	"github.com/openshift/cluster-api-provider-libvirt/pkg/apis/libvirtproviderconfig/v1beta1"
	machineactuator "github.com/openshift/cluster-api-provider-libvirt/pkg/cloud/libvirt/actuators/machine"
	libvirtclient "github.com/openshift/cluster-api-provider-libvirt/pkg/cloud/libvirt/client"
	"github.com/openshift/cluster-api-provider-libvirt/test"
	clusterv1 "github.com/openshift/cluster-api/pkg/apis/cluster/v1alpha1"
	machinev1 "github.com/openshift/cluster-api/pkg/apis/machine/v1beta1"
	kubernetesfake "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/tools/record"
)

func ReadClusterResources(clusterLoc, machineLoc, userDataLoc string) (*clusterv1.Cluster, *machinev1.Machine, *apiv1.Secret, error) {
	machine := &machinev1.Machine{}
	{
		bytes, err := ioutil.ReadFile(machineLoc)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("failed to read machine manifest %q: %v", machineLoc, err)
		}

		if err = yaml.Unmarshal(bytes, &machine); err != nil {
			return nil, nil, nil, fmt.Errorf("failed to unmarshal machine manifest %q: %v", machineLoc, err)
		}
	}

	cluster := &clusterv1.Cluster{}
	{
		bytes, err := ioutil.ReadFile(clusterLoc)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("failed to read cluster manifest %q: %v", clusterLoc, err)
		}

		if err = yaml.Unmarshal(bytes, &cluster); err != nil {
			return nil, nil, nil, fmt.Errorf("failed to unmarshal cluster manifest %q: %v", clusterLoc, err)
		}
	}

	var userDataSecret *apiv1.Secret
	if userDataLoc != "" {
		userDataSecret = &apiv1.Secret{}
		bytes, err := ioutil.ReadFile(userDataLoc)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("failed to read user data manifest %q: %v", userDataLoc, err)
		}

		if err = yaml.Unmarshal(bytes, &userDataSecret); err != nil {
			return nil, nil, nil, fmt.Errorf("failed to unmarshal user data manifest %q: %v", userDataLoc, err)
		}
	}

	return cluster, machine, userDataSecret, nil
}

func CreateActuator(machine *machinev1.Machine, userData *apiv1.Secret) *machineactuator.Actuator {
	objList := []runtime.Object{}
	if userData != nil {
		objList = append(objList, userData)
	}
	fakeKubeClient := kubernetesfake.NewSimpleClientset(objList...)

	codec, err := v1beta1.NewCodec()
	if err != nil {
		glog.Fatal(err)
	}

	params := machineactuator.ActuatorParams{
		ClusterClient: test.NewSimpleClientset(machine),
		KubeClient:    fakeKubeClient,
		ClientBuilder: libvirtclient.NewClient,
		Codec:         codec,
		EventRecorder: &record.FakeRecorder{},
	}
	actuator, _ := machineactuator.NewActuator(params)
	return actuator
}
