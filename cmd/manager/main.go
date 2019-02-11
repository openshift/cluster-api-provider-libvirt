package main

import (
	"github.com/openshift/cluster-api-provider-libvirt/pkg/apis"
	"github.com/openshift/cluster-api-provider-libvirt/pkg/apis/libvirtproviderconfig/v1beta1"
	machineactuator "github.com/openshift/cluster-api-provider-libvirt/pkg/cloud/libvirt/actuators/machine"
	libvirtclient "github.com/openshift/cluster-api-provider-libvirt/pkg/cloud/libvirt/client"
	"github.com/openshift/cluster-api-provider-libvirt/pkg/controller"
	machineapis "github.com/openshift/cluster-api/pkg/apis"
	"github.com/openshift/cluster-api/pkg/client/clientset_generated/clientset"
	"k8s.io/client-go/kubernetes"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/runtime/signals"

	"flag"

	"github.com/golang/glog"
	"k8s.io/klog"
)

func main() {
	klogFlags := flag.NewFlagSet("klog", flag.ExitOnError)
	klog.InitFlags(klogFlags)
	flag.Parse()
	flag.VisitAll(func(f1 *flag.Flag) {
		f2 := klogFlags.Lookup(f1.Name)
		if f2 != nil {
			value := f1.Value.String()
			f2.Value.Set(value)
		}
	})

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

	if err := machineapis.AddToScheme(mgr.GetScheme()); err != nil {
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

	codec, err := v1beta1.NewCodec()
	if err != nil {
		glog.Fatal(err)
	}

	params := machineactuator.ActuatorParams{
		ClusterClient: client,
		KubeClient:    kubeClient,
		ClientBuilder: libvirtclient.NewClient,
		Codec:         codec,
		EventRecorder: m.GetRecorder("libvirt-controller"),
	}

	machineactuator.MachineActuator, err = machineactuator.NewActuator(params)
	if err != nil {
		glog.Fatalf("Could not create Libvirt machine actuator: %v", err)
	}
}
