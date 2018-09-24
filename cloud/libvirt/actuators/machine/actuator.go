// Copyright © 2018 The Kubernetes Authors.
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package machine

import (
	"fmt"

	"github.com/golang/glog"
	libvirtutils "github.com/openshift/cluster-api-provider-libvirt/cloud/libvirt/actuators/machine/utils"
	clusterv1 "sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1"
	clusterclient "sigs.k8s.io/cluster-api/pkg/client/clientset_generated/clientset"
)

// Actuator is responsible for performing machine reconciliation
type Actuator struct {
	clusterClient clusterclient.Interface
	cidrOffset    int
}

// ActuatorParams holds parameter information for Actuator
type ActuatorParams struct {
	ClusterClient clusterclient.Interface
}

// NewActuator creates a new Actuator
func NewActuator(params ActuatorParams) (*Actuator, error) {
	return &Actuator{
		clusterClient: params.ClusterClient,
		cidrOffset:    50,
	}, nil
}

// Create creates a machine and is invoked by the Machine Controller
func (a *Actuator) Create(cluster *clusterv1.Cluster, machine *clusterv1.Machine) error {
	glog.Infof("Creating machine %q for cluster %q.", machine.Name, cluster.Name)
	// TODO: hack to increase IPs. Build proper logic in setNetworkInterfaces method
	a.cidrOffset++
	if err := libvirtutils.CreateVolumeAndMachine(machine, a.cidrOffset); err != nil {
		glog.Errorf("Could not create libvirt machine: %v", err)
		return fmt.Errorf("error creating machine %v", err)
	}
	return nil
}

// Delete deletes a machine and is invoked by the Machine Controller
func (a *Actuator) Delete(cluster *clusterv1.Cluster, machine *clusterv1.Machine) error {
	glog.Infof("Deleting machine %q for cluster %q.", machine.Name, cluster.Name)
	return libvirtutils.DeleteVolumeAndDomain(machine)
}

// Update updates a machine and is invoked by the Machine Controller
func (a *Actuator) Update(cluster *clusterv1.Cluster, machine *clusterv1.Machine) error {
	glog.Infof("Updating machine %v for cluster %v.", machine.Name, cluster.Name)
	return fmt.Errorf("TODO: Not yet implemented")
}

// Exists test for the existance of a machine and is invoked by the Machine Controller
func (a *Actuator) Exists(cluster *clusterv1.Cluster, machine *clusterv1.Machine) (bool, error) {
	glog.Infof("Checking if machine %v for cluster %v exists.", machine.Name, cluster.Name)
	return libvirtutils.DomainExists(machine)
}
