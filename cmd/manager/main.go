package main

import (
	"flag"
	"time"

	"github.com/golang/glog"
	"github.com/openshift/cluster-api-provider-libvirt/pkg/apis"
	"github.com/openshift/cluster-api-provider-libvirt/pkg/apis/libvirtproviderconfig/v1beta1"
	machineactuator "github.com/openshift/cluster-api-provider-libvirt/pkg/cloud/libvirt/actuators/machine"
	libvirtclient "github.com/openshift/cluster-api-provider-libvirt/pkg/cloud/libvirt/client"
	"github.com/openshift/cluster-api-provider-libvirt/pkg/controller"
	machineapis "github.com/openshift/machine-api-operator/pkg/apis/machine/v1beta1"
	clientset "github.com/openshift/machine-api-operator/pkg/generated/clientset/versioned"
	"github.com/openshift/machine-api-operator/pkg/metrics"

	"k8s.io/client-go/kubernetes"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	"k8s.io/klog"

	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/manager/signals"
)

func main() {
	watchNamespace := flag.String("namespace", "", "Namespace that the controller watches to reconcile machine-api objects. If unspecified, the controller watches for machine-api objects across all namespaces.")
	healthAddr := flag.String(
		"health-addr",
		":9440",
		"The address for health checking.",
	)

	metricsAddress := flag.String(
		"metrics-bind-address",
		metrics.DefaultMachineMetricsAddress,
		"Address for hosting metrics",
	)

	klogFlags := flag.NewFlagSet("klog", flag.ExitOnError)
	klog.InitFlags(klogFlags)

	leaderElectResourceNamespace := flag.String(
		"leader-elect-resource-namespace",
		"",
		"The namespace of resource object that is used for locking during leader election. If unspecified and running in cluster, defaults to the service account namespace for the controller. Required for leader-election outside of a cluster.",
	)

	leaderElect := flag.Bool(
		"leader-elect",
		false,
		"Start a leader election client and gain leadership before executing the main loop. Enable this when running replicated components for high availability.",
	)

	leaderElectLeaseDuration := flag.Duration(
		"leader-elect-lease-duration",
		15*time.Second,
		"The duration that non-leader candidates will wait after observing a leadership renewal until attempting to acquire leadership of a led but unrenewed leader slot. This is effectively the maximum duration that a leader can be stopped before it is replaced by another candidate. This is only applicable if leader election is enabled.",
	)

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

	opts := manager.Options{
		LeaderElection:          *leaderElect,
		LeaderElectionNamespace: *leaderElectResourceNamespace,
		LeaderElectionID:        "cluster-api-provider-libvirt-leader",
		LeaseDuration:           leaderElectLeaseDuration,
		HealthProbeBindAddress:  *healthAddr,
		MetricsBindAddress:      *metricsAddress,
	}

	if *watchNamespace != "" {
		opts.Namespace = *watchNamespace
		klog.Infof("Watching machine-api objects only in namespace %q for reconciliation.", opts.Namespace)
	}

	// Create a new Cmd to provide shared dependencies and start components
	mgr, err := manager.New(cfg, opts)
	if err != nil {
		glog.Fatal(err)
	}

	glog.Info("Registering Components")

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

	if err := mgr.AddReadyzCheck("ping", healthz.Ping); err != nil {
		klog.Fatal(err)
	}

	if err := mgr.AddHealthzCheck("ping", healthz.Ping); err != nil {
		klog.Fatal(err)
	}

	glog.Info("Starting the Cmd")

	// Start the Cmd
	err = mgr.Start(signals.SetupSignalHandler())
	if err != nil {
		glog.Fatal(err)
	}
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
		EventRecorder: m.GetEventRecorderFor("libvirt-controller"),
	}

	machineactuator.MachineActuator, err = machineactuator.NewActuator(params)
	if err != nil {
		glog.Fatalf("Could not create Libvirt machine actuator: %v", err)
	}
}
