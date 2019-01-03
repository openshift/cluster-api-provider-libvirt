package machine

import (
	"context"
	"fmt"
	"testing"

	"github.com/golang/mock/gomock"
	libvirt "github.com/libvirt/libvirt-go"
	providerconfigv1 "github.com/openshift/cluster-api-provider-libvirt/pkg/apis/libvirtproviderconfig/v1alpha1"
	libvirtclient "github.com/openshift/cluster-api-provider-libvirt/pkg/cloud/libvirt/client"
	mocklibvirt "github.com/openshift/cluster-api-provider-libvirt/pkg/cloud/libvirt/client/mock"
	fakeclusterclientset "github.com/openshift/cluster-api-provider-libvirt/test"
	kubernetesfake "k8s.io/client-go/kubernetes/fake"
	clusterv1 "sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1"

	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
)

func init() {
	// Add types to scheme
	clusterv1.AddToScheme(scheme.Scheme)
}

const (
	noError            = ""
	libvirtClientError = "error creating libvirt client"
)

func TestMachineEvents(t *testing.T) {
	codec, err := providerconfigv1.NewCodec()
	if err != nil {
		t.Fatalf("unable to build codec: %v", err)
	}

	machine, err := stubMachine()
	if err != nil {
		t.Fatal(err)
	}

	cluster := stubCluster()

	machineInvalidProviderConfig := machine.DeepCopy()
	machineInvalidProviderConfig.Spec.ProviderSpec.Value = nil
	machineInvalidProviderConfig.Spec.ProviderSpec.ValueFrom = nil

	cases := []struct {
		name               string
		machine            *clusterv1.Machine
		error              string
		operation          func(actuator *Actuator, cluster *clusterv1.Cluster, machine *clusterv1.Machine)
		event              string
		createVolumeErr    error
		deleteVolumeErr    error
		createDomainErr    error
		deleteDomainErr    error
		lookupDomainOutput *libvirt.Domain
		lookupDomainErr    error
		domainExistsErr    error
		domainExists       bool
	}{
		{
			name:    "Create machine event failed (invalid configuration)",
			machine: machineInvalidProviderConfig,
			operation: func(actuator *Actuator, cluster *clusterv1.Cluster, machine *clusterv1.Machine) {
				actuator.Create(context.TODO(), cluster, machine)
			},
			event: "Warning FailedCreate InvalidConfiguration",
		},
		{
			name:    "Create machine event failed (error creating libvirt client)",
			machine: machine,
			error:   libvirtClientError,
			operation: func(actuator *Actuator, cluster *clusterv1.Cluster, machine *clusterv1.Machine) {
				actuator.Create(context.TODO(), cluster, machine)
			},
			event: "Warning FailedCreate CreateError",
		},
		{
			name:            "Create machine event failed (error creating volume)",
			machine:         machine,
			createVolumeErr: fmt.Errorf("error"),
			operation: func(actuator *Actuator, cluster *clusterv1.Cluster, machine *clusterv1.Machine) {
				actuator.Create(context.TODO(), cluster, machine)
			},
			event: "Warning FailedCreate CreateError",
		},
		{
			name:            "Create machine event failed (error creating domain)",
			machine:         machine,
			createDomainErr: fmt.Errorf("error"),
			operation: func(actuator *Actuator, cluster *clusterv1.Cluster, machine *clusterv1.Machine) {
				actuator.Create(context.TODO(), cluster, machine)
			},
			event: "Warning FailedCreate CreateError",
		},
		{
			name:            "Create machine event failed (error looking up domain)",
			machine:         machine,
			lookupDomainErr: fmt.Errorf("error"),
			operation: func(actuator *Actuator, cluster *clusterv1.Cluster, machine *clusterv1.Machine) {
				actuator.Create(context.TODO(), cluster, machine)
			},
			event: "Warning FailedCreate CreateError",
		},
		{
			name:    "Create machine event succeed",
			machine: machine,
			operation: func(actuator *Actuator, cluster *clusterv1.Cluster, machine *clusterv1.Machine) {
				actuator.Create(context.TODO(), cluster, machine)
			},
			event: "Normal Created Created Machine libvirt-actuator-testing-machine",
		},
		{
			name:    "Delete machine event failed (invalid configuration)",
			machine: machineInvalidProviderConfig,
			operation: func(actuator *Actuator, cluster *clusterv1.Cluster, machine *clusterv1.Machine) {
				actuator.Delete(context.TODO(), cluster, machine)
			},
			event: "Warning FailedDelete InvalidConfiguration",
		},
		{
			name:    "Delete machine event failed (error creating libvirt client)",
			machine: machine,
			error:   libvirtClientError,
			operation: func(actuator *Actuator, cluster *clusterv1.Cluster, machine *clusterv1.Machine) {
				actuator.Delete(context.TODO(), cluster, machine)
			},
			event: "Warning FailedDelete DeleteError",
		},
		{
			name:            "Delete machine event failed (error getting domain)",
			machine:         machine,
			domainExistsErr: fmt.Errorf("error"),
			operation: func(actuator *Actuator, cluster *clusterv1.Cluster, machine *clusterv1.Machine) {
				actuator.Delete(context.TODO(), cluster, machine)
			},
			event: "Warning FailedDelete DeleteError",
		},
		{
			name:            "Delete machine event failed (error deleting domain)",
			machine:         machine,
			domainExists:    true,
			deleteDomainErr: fmt.Errorf("error"),
			operation: func(actuator *Actuator, cluster *clusterv1.Cluster, machine *clusterv1.Machine) {
				actuator.Delete(context.TODO(), cluster, machine)
			},
			event: "Warning FailedDelete DeleteError",
		},
		{
			name:            "Delete machine event failed (error deleting volume)",
			machine:         machine,
			domainExists:    true,
			deleteVolumeErr: fmt.Errorf("error"),
			operation: func(actuator *Actuator, cluster *clusterv1.Cluster, machine *clusterv1.Machine) {
				actuator.Delete(context.TODO(), cluster, machine)
			},
			event: "Warning FailedDelete DeleteError",
		},
		{
			name:         "Delete machine event succeeds",
			machine:      machine,
			domainExists: true,
			operation: func(actuator *Actuator, cluster *clusterv1.Cluster, machine *clusterv1.Machine) {
				actuator.Delete(context.TODO(), cluster, machine)
			},
			event: "Normal Deleted Deleted Machine libvirt-actuator-testing-machine",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {

			mockCtrl := gomock.NewController(t)
			mockLibvirtClient := mocklibvirt.NewMockClient(mockCtrl)

			eventsChannel := make(chan string, 1)

			params := ActuatorParams{
				ClusterClient: fakeclusterclientset.NewSimpleClientset(tc.machine),
				KubeClient:    kubernetesfake.NewSimpleClientset(),
				ClientBuilder: func(uri string) (libvirtclient.Client, error) {
					if tc.error == libvirtClientError {
						return nil, fmt.Errorf(libvirtClientError)
					}
					return mockLibvirtClient, nil
				},
				Codec: codec,
				// use fake recorder and store an event into one item long buffer for subsequent check
				EventRecorder: &record.FakeRecorder{
					Events: eventsChannel,
				},
			}

			mockLibvirtClient.EXPECT().Close()
			mockLibvirtClient.EXPECT().CreateVolume(gomock.Any()).Return(tc.createVolumeErr).AnyTimes()
			mockLibvirtClient.EXPECT().DeleteVolume(gomock.Any()).Return(tc.deleteVolumeErr).AnyTimes()
			mockLibvirtClient.EXPECT().CreateDomain(gomock.Any()).Return(tc.createDomainErr).AnyTimes()
			mockLibvirtClient.EXPECT().DeleteDomain(gomock.Any()).Return(tc.deleteDomainErr).AnyTimes()
			mockLibvirtClient.EXPECT().LookupDomainByName(gomock.Any()).Return(tc.lookupDomainOutput, tc.lookupDomainErr).AnyTimes()
			mockLibvirtClient.EXPECT().DomainExists(gomock.Any()).Return(tc.domainExists, tc.domainExistsErr).AnyTimes()

			actuator, err := NewActuator(params)
			if err != nil {
				t.Fatalf("Could not create AWS machine actuator: %v", err)
			}

			tc.operation(actuator, cluster, tc.machine)
			select {
			case event := <-eventsChannel:
				if event != tc.event {
					t.Errorf("Expected %q event, got %q", tc.event, event)
				}
			default:
				t.Errorf("Expected %q event, got none", tc.event)
			}
		})
	}
}
