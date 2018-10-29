package main

import (
	"github.com/openshift/cluster-api-provider-libvirt/pkg/apis"
	"github.com/openshift/cluster-api-provider-libvirt/pkg/apis/libvirtproviderconfig/v1alpha1"
	machineactuator "github.com/openshift/cluster-api-provider-libvirt/pkg/cloud/libvirt/actuators/machine"
	"github.com/openshift/cluster-api-provider-libvirt/pkg/controller"
	"k8s.io/client-go/kubernetes"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	clusterapis "sigs.k8s.io/cluster-api/pkg/apis"
	"sigs.k8s.io/cluster-api/pkg/client/clientset_generated/clientset"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/runtime/signals"

	"flag"

	"github.com/golang/glog"
)

func main() {
	// the following line exists to make glog happy, for more information, see: https://github.com/kubernetes/kubernetes/issues/17162
	//flag.CommandLine.Parse([]string{})
	flag.Parse()

	// Get a config to talk to the apiserver
	cfg, err := config.GetConfig()
	if err != nil {
		glog.Fatal(err)
	}

	// Create a new Cmd to provide shared dependencies and start components
	mgr, err := manager.New(cfg, manager.Options{})
	if err != nil {
		glog.Fatal(err)
	}

	glog.Infof("Registering Components.")

	// Setup Scheme for all resources
	if err := apis.AddToScheme(mgr.GetScheme()); err != nil {
		glog.Fatal(err)
	}

	if err := clusterapis.AddToScheme(mgr.GetScheme()); err != nil {
		glog.Fatal(err)
	}

	initActuator(mgr)
	// Setup all Controllers
	if err := controller.AddToManager(mgr); err != nil {
		glog.Fatal(err)
	}

	glog.Infof("Starting the Cmd.")

	// Start the Cmd
	glog.Fatal(mgr.Start(signals.SetupSignalHandler()))
}

func initActuator(m manager.Manager) {
	config := m.GetConfig()
	client, err := clientset.NewForConfig(config)
	if err != nil {
		glog.Fatalf("Could not create client for talking to the apiserver: %v", err)
	}

	kubeClient, err := kubernetes.NewForConfig(config)
	if err != nil {
		glog.Fatalf("Could not create kubernetes client to talk to the apiserver: %v", err)
	}

	codec, err := v1alpha1.NewCodec()
	if err != nil {
		glog.Fatal(err)
	}

	params := machineactuator.ActuatorParams{
		ClusterClient: client,
		KubeClient:    kubeClient,
		Codec:         codec,
	}

	machineactuator.MachineActuator, err = machineactuator.NewActuator(params)
	if err != nil {
		glog.Fatalf("Could not create Libvirt machine actuator: %v", err)
	}
}
