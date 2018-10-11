package machines

import (
	"fmt"
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/prometheus/common/log"
	"k8s.io/apimachinery/pkg/util/uuid"

	"github.com/openshift/cluster-api-actuator-pkg/pkg/e2e/framework"
	"github.com/openshift/cluster-api-actuator-pkg/pkg/manifests"
	"github.com/openshift/cluster-api-provider-libvirt/test/utils"

	apiv1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clusterv1alpha1 "sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1"
)

const CloudProvider = "libvirt"

func TestCart(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Machine Suite")
}

var _ = framework.SigKubeDescribe("Machines", func() {
	f := framework.NewFramework()
	var err error
	var testNamespace *apiv1.Namespace

	machinesToDelete := framework.InitMachinesToDelete()

	BeforeEach(func() {
		f.BeforeEach()

		testNamespace = &apiv1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "namespace-" + string(uuid.NewUUID()),
			},
		}

		By(fmt.Sprintf("Creating %q namespace", testNamespace.Name))
		_, err = f.KubeClient.CoreV1().Namespaces().Create(testNamespace)
		Expect(err).NotTo(HaveOccurred())

		f.DeployClusterAPIStack(testNamespace.Name, CloudProvider)
	})

	AfterEach(func() {
		// Make sure all machine(set)s are deleted before deleting its namespace
		machinesToDelete.Delete()

		if testNamespace != nil {
			f.DestroyClusterAPIStack(testNamespace.Name, CloudProvider)
			log.Infof(testNamespace.Name+": %#v", testNamespace)
			By(fmt.Sprintf("Destroying %q namespace", testNamespace.Name))
			f.KubeClient.CoreV1().Namespaces().Delete(testNamespace.Name, &metav1.DeleteOptions{})
			// Ignore namespaces that are not deleted so other specs can be run.
			// Every spec runs in its own namespaces so it's enough to make sure
			// namespaces does not inluence each other
		}
		// it's assumed the cluster API is completely destroyed
	})

	// Any of the tests run assumes the cluster-api stack is already deployed.
	// So all the machine, resp. machineset related tests must be run on top
	// of the same cluster-api stack. Once the machine, resp. machineset objects
	// are defined through CRD, we can relax the restriction.
	Context("libvirt actuator", func() {
		It("can create domain", func() {
			clusterID := framework.ClusterID
			if clusterID == "" {
				clusterID = "cluster-" + string(uuid.NewUUID())
			}

			cluster := &clusterv1alpha1.Cluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      clusterID,
					Namespace: testNamespace.Name,
				},
				Spec: clusterv1alpha1.ClusterSpec{
					ClusterNetwork: clusterv1alpha1.ClusterNetworkingConfig{
						Services: clusterv1alpha1.NetworkRanges{
							CIDRBlocks: []string{"10.0.0.1/24"},
						},
						Pods: clusterv1alpha1.NetworkRanges{
							CIDRBlocks: []string{"10.0.0.1/24"},
						},
						ServiceDomain: "example.com",
					},
				},
			}

			f.CreateClusterAndWait(cluster)

			// Create/delete a single machine, test instance is provisioned/terminated
			uri := "qemu:///system"
			testMachineProviderConfig, err := utils.TestingMachineProviderConfig(uri, cluster.Name)
			Expect(err).NotTo(HaveOccurred())
			testMachine := manifests.TestingMachine(cluster.Name, cluster.Namespace, testMachineProviderConfig)
			libvirtClient, err := NewLibvirtClient(uri)
			Expect(err).NotTo(HaveOccurred())

			f.CreateMachineAndWait(testMachine, libvirtClient)
			machinesToDelete.AddMachine(testMachine, f, libvirtClient)

			// TODO: Run some tests here!!! E.g. check for properly set security groups, iam role, tags

			f.DeleteMachineAndWait(testMachine, libvirtClient)
		})
	})
})
