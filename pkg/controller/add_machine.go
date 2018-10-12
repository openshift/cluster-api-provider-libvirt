/*
Copyright 2018 The Kubernetes authors.
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

package controller

import (
	"github.com/golang/glog"
	machineactuator "github.com/openshift/cluster-api-provider-libvirt/pkg/cloud/libvirt/actuators/machine"
	log "github.com/sirupsen/logrus"
	"k8s.io/client-go/kubernetes"
	"os"
	"sigs.k8s.io/cluster-api/pkg/client/clientset_generated/clientset"
	"sigs.k8s.io/cluster-api/pkg/controller/machine"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

func init() {
	// AddToManagerFuncs is a list of functions to create controllers and add them to a manager.
	AddToManagerFuncs = append(AddToManagerFuncs, func(m manager.Manager) error {
		config := m.GetConfig()

		client, err := clientset.NewForConfig(config)
		if err != nil {
			glog.Fatalf("Could not create client for talking to the apiserver: %v", err)
		}

		kubeClient, err := kubernetes.NewForConfig(config)
		if err != nil {
			glog.Fatalf("Could not create kubernetes client to talk to the apiserver: %v", err)
		}

		log.SetOutput(os.Stdout)
		if lvl, err := log.ParseLevel("debug"); err != nil {
			log.Panic(err)
		} else {
			log.SetLevel(lvl)
		}

		params := machineactuator.ActuatorParams{
			ClusterClient: client,
			KubeClient:    kubeClient,
		}

		actuator, err := machineactuator.NewActuator(params)
		if err != nil {
			glog.Fatalf("Could not create Libvirt machine actuator: %v", err)
		}
		return machine.AddWithActuator(m, actuator)
	})
}
