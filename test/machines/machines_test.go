package machines

import (
	"fmt"
	"io/ioutil"
	"testing"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/uuid"
	"k8s.io/apimachinery/pkg/util/wait"

	"github.com/openshift/cluster-api-actuator-pkg/pkg/e2e/framework"
	"github.com/openshift/cluster-api-actuator-pkg/pkg/manifests"
	"github.com/openshift/cluster-api-provider-libvirt/test/utils"

	log "github.com/sirupsen/logrus"
	apiv1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clusterv1alpha1 "sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1"
)

const (
	poolTimeout                       = 20 * time.Second
	pollInterval                      = 1 * time.Second
	poolClusterAPIDeploymentTimeout   = 10 * time.Minute
	timeoutPoolMachineRunningInterval = 10 * time.Minute
)

func TestCart(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Machine Suite")
}

func BuildPKSecret(secretName, namespace, pkLoc string) (*apiv1.Secret, error) {
	pkBytes, err := ioutil.ReadFile(pkLoc)
	if err != nil {
		return nil, fmt.Errorf("unable to read %v: %v", pkLoc, err)
	}

	return &apiv1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: namespace,
		},
		Data: map[string][]byte{
			"privatekey": pkBytes,
		},
	}, nil
}

func createSecretAndWait(f *framework.Framework, secret *apiv1.Secret) {
	_, err := f.KubeClient.CoreV1().Secrets(secret.Namespace).Create(secret)
	Expect(err).NotTo(HaveOccurred())

	err = wait.Poll(framework.PollInterval, framework.PoolTimeout, func() (bool, error) {
		if _, err := f.KubeClient.CoreV1().Secrets(secret.Namespace).Get(secret.Name, metav1.GetOptions{}); err != nil {
			return false, nil
		}
		return true, nil
	})
	Expect(err).NotTo(HaveOccurred())
}

var _ = framework.SigKubeDescribe("Machines", func() {
	f, err := framework.NewFramework()
	if err != nil {
		panic(fmt.Errorf("unable to create framework: %v", err))
	}
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

		if f.LibvirtPK != "" {
			libvirtPKSecret, err := BuildPKSecret("libvirt-private-key", testNamespace.Name, f.LibvirtPK)
			Expect(err).NotTo(HaveOccurred())
			log.Infof("Creating %q secret", libvirtPKSecret.Name)
			_, err = f.KubeClient.CoreV1().Secrets(libvirtPKSecret.Namespace).Create(libvirtPKSecret)
			Expect(err).NotTo(HaveOccurred())
		}

		f.DeployClusterAPIStack(testNamespace.Name, f.ActuatorImage, "libvirt-private-key")
	})

	AfterEach(func() {
		// Make sure all machine(set)s are deleted before deleting its namespace
		// machinesToDelete.Delete()

		if testNamespace != nil {
			f.DestroyClusterAPIStack(testNamespace.Name, f.ActuatorImage, "libvirt-private-key")
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
			testMachineProviderConfig, err := utils.TestingMachineProviderConfig(f.LibvirtURI, cluster.Name)
			Expect(err).NotTo(HaveOccurred())
			testMachine := manifests.TestingMachine(cluster.Name, cluster.Namespace, testMachineProviderConfig)
			lcw, err := NewLibvirtClient("qemu:///system")
			Expect(err).NotTo(HaveOccurred())

			f.CreateMachineAndWait(testMachine, lcw)
			machinesToDelete.AddMachine(testMachine, f, lcw)

			// TODO: Run some tests here!!! E.g. check for properly set security groups, iam role, tags

			f.DeleteMachineAndWait(testMachine, lcw)
		})
	})

	// ./machines.test -kubeconfig ~/.kube/config -ginkgo.v -actuator-image openshift/origin-libvirt-machine-controllers:v4.0.0 -libvirt-uri 'qemu+ssh://root@147.75.198.9/system?no_verify=1&keyfile=/root/.ssh/actuator.pem/privatekey' -libvirt-pk /libvirt.pem -ssh-user fedora -ssh-key /root/guest.pem
	It("Can deploy compute nodes through machineset", func() {
		// Any controller linking kubernetes node with its machine
		// needs to run inside the cluster where the node is registered.
		// Which means the cluster API stack needs to be deployed in the same
		// cluster in order to list machine object(s) defining the node.
		//
		// One could run the controller inside the current cluster and have
		// new nodes join the cluster assumming the cluster was created with kubeadm
		// and the bootstrapping token is known in advance. Though, in case of AWS
		// all instances must live in the same vpc, otherwise additional configuration
		// of the underlying environment is required. Which does not have to be
		// available.

		// Thus:
		// 1. create testing cluster (deploy master node)
		// 2. deploy the cluster-api inside the master node
		// 3. deploy machineset with worker nodes
		// 4. check all worker nodes has compute role and corresponding machines
		//    are linked to them
		clusterID := framework.ClusterID
		if clusterID == "" {
			clusterUUID := string(uuid.NewUUID())
			clusterID = "cluster-" + clusterUUID[:8]
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

		// Create master machine and verify the master node is ready
		masterUserDataSecret, err := manifests.MasterMachineUserDataSecret(
			"masteruserdatasecret",
			testNamespace.Name,
			[]string{"127.0.0.1"},
		)
		Expect(err).NotTo(HaveOccurred())

		createSecretAndWait(f, masterUserDataSecret)
		masterMachineProviderConfig, err := utils.MasterMachineProviderConfig(masterUserDataSecret.Name, f.LibvirtURI)
		Expect(err).NotTo(HaveOccurred())
		masterMachine := manifests.MasterMachine(cluster.Name, cluster.Namespace, masterMachineProviderConfig)
		lcw, err := NewLibvirtClient("qemu:///system")
		Expect(err).NotTo(HaveOccurred())

		f.CreateMachineAndWait(masterMachine, lcw)
		machinesToDelete.AddMachine(masterMachine, f, lcw)

		// Wait until the instance has the ip address
		var masterMachinePrivateIP string
		err = wait.Poll(pollInterval, poolTimeout, func() (bool, error) {
			privateIP, err := lcw.GetPrivateIP(masterMachine)
			if err != nil {
				return false, nil
			}
			masterMachinePrivateIP = privateIP
			return true, nil
		})

		log.Infof("Master machine running at %v", masterMachinePrivateIP)

		By("Collecting master kubeconfig")
		restConfig, err := f.GetMasterMachineRestConfig(masterMachine, lcw)
		Expect(err).NotTo(HaveOccurred())

		// Load actuator docker image to the master node
		By("Upload actuator image to the master guest")
		err = f.UploadDockerImageToInstance(f.ActuatorImage, masterMachinePrivateIP)
		Expect(err).NotTo(HaveOccurred())

		// Deploy the cluster API stack inside the master machine
		sshConfig, err := framework.DefaultSSHConfig()
		Expect(err).NotTo(HaveOccurred())
		clusterFramework, err := framework.NewFrameworkFromConfig(restConfig, sshConfig)
		Expect(err).NotTo(HaveOccurred())
		By(fmt.Sprintf("Creating %q namespace", testNamespace.Name))
		_, err = clusterFramework.KubeClient.CoreV1().Namespaces().Create(testNamespace)
		Expect(err).NotTo(HaveOccurred())

		if f.LibvirtPK != "" {
			libvirtPKSecret, err := BuildPKSecret("libvirt-private-key", testNamespace.Name, f.LibvirtPK)
			Expect(err).NotTo(HaveOccurred())
			log.Infof("Creating %q secret", libvirtPKSecret.Name)
			_, err = clusterFramework.KubeClient.CoreV1().Secrets(libvirtPKSecret.Namespace).Create(libvirtPKSecret)
			Expect(err).NotTo(HaveOccurred())
		}

		clusterFramework.DeployClusterAPIStack(testNamespace.Name, f.ActuatorImage, "libvirt-private-key")

		By("Deploy worker nodes through machineset")
		masterPrivateIP := masterMachinePrivateIP

		// Reuse the namespace, secret and the cluster objects
		clusterFramework.CreateClusterAndWait(cluster)

		workerUserDataSecret, err := manifests.WorkerMachineUserDataSecret("workeruserdatasecret", testNamespace.Name, masterPrivateIP)
		Expect(err).NotTo(HaveOccurred())
		createSecretAndWait(clusterFramework, workerUserDataSecret)
		workerMachineSetProviderConfig, err := utils.WorkerMachineProviderConfig(workerUserDataSecret.Name, f.LibvirtURI)
		Expect(err).NotTo(HaveOccurred())
		workerMachineSet := manifests.WorkerMachineSet(cluster.Name, cluster.Namespace, workerMachineSetProviderConfig)

		clusterFramework.CreateMachineSetAndWait(workerMachineSet, lcw)
		machinesToDelete.AddMachineSet(workerMachineSet, clusterFramework, lcw)

		By("Checking master and worker nodes are ready")
		err = clusterFramework.WaitForNodesToGetReady(2)
		Expect(err).NotTo(HaveOccurred())
		By("Both master and worker nodes are ready")

		By("Checking compute node role and node linking")
		err = wait.Poll(framework.PollInterval, 5*framework.PoolTimeout, func() (bool, error) {
			items, err := clusterFramework.KubeClient.CoreV1().Nodes().List(metav1.ListOptions{})
			if err != nil {
				return false, fmt.Errorf("unable to list nodes: %v", err)
			}

			var nonMasterNodes []apiv1.Node
			for _, node := range items.Items {
				// filter out all nodes with master role (assumming it's always set)
				if _, isMaster := node.Labels["node-role.kubernetes.io/master"]; isMaster {
					continue
				}
				nonMasterNodes = append(nonMasterNodes, node)
			}

			log.Infof("Non-master nodes to check: %#v", nonMasterNodes)
			machines, err := clusterFramework.CAPIClient.ClusterV1alpha1().Machines(workerMachineSet.Namespace).List(metav1.ListOptions{
				LabelSelector: labels.SelectorFromSet(workerMachineSet.Spec.Selector.MatchLabels).String(),
			})
			Expect(err).NotTo(HaveOccurred())

			matches := make(map[string]string)
			for _, machine := range machines.Items {
				if machine.Status.NodeRef != nil {
					matches[machine.Status.NodeRef.Name] = machine.Name
				}
			}

			log.Infof("Machine-node matches: %#v\n", matches)
			// non-master node, the workerset deploys only compute nodes
			for _, node := range nonMasterNodes {
				// check role
				_, isCompute := node.Labels["node-role.kubernetes.io/compute"]
				if !isCompute {
					log.Infof("node %q does not have the compute role assigned", node.Name)
					return false, nil
				}
				log.Infof("node %q role set to 'node-role.kubernetes.io/compute'", node.Name)
				// check node linking

				// If there is the same number of machines are compute nodes,
				// each node has to have a machine that refers the node.
				// So it's enough to check each node has its machine linked.
				matchingMachine, found := matches[node.Name]
				if !found {
					log.Infof("node %q is not linked with a machine", node.Name)
					return false, nil
				}
				log.Infof("node %q is linked with %q machine", node.Name, matchingMachine)
			}

			return true, nil
		})
		Expect(err).NotTo(HaveOccurred())

		By("Destroying worker machines")
		// Let it fail and continue (assuming all instances gets removed out of the e2e)
		clusterFramework.DeleteMachineSetAndWait(workerMachineSet, lcw)
		By("Destroying master machine")
		f.DeleteMachineAndWait(masterMachine, lcw)
	})
})
