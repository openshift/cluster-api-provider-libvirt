package main

// Tests individual Libvirt actuator actions

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"os/user"
	"time"

	"github.com/golang/glog"
	"github.com/openshift/cluster-api-provider-libvirt/cmd/libvirt-actuator/utils"

	flag "github.com/spf13/pflag"

	goflag "flag"

	testutils "github.com/openshift/cluster-api-provider-libvirt/test/utils"

	"github.com/spf13/cobra"

	apiv1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/apimachinery/pkg/util/wait"
	clusterv1 "sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1"

	"github.com/openshift/cluster-api-actuator-pkg/pkg/e2e/framework"
	"github.com/openshift/cluster-api-actuator-pkg/pkg/manifests"
	libvirtclient "github.com/openshift/cluster-api-provider-libvirt/test/machines"
)

const (
	pollInterval        = 5 * time.Second
	timeoutPoolInterval = 20 * time.Minute
)

func usage() {
	fmt.Printf("Usage: %s\n\n", os.Args[0])
}

var rootCmd = &cobra.Command{
	Use:   "libvirt-actuator-test",
	Short: "Test for Cluster API Libvirt actuator",
}

func createCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "create",
		Short: "Create machine instance for specified cluster",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := checkFlags(cmd); err != nil {
				return err
			}
			cluster, machine, userData, err := utils.ReadClusterResources(
				cmd.Flag("cluster").Value.String(),
				cmd.Flag("machine").Value.String(),
				cmd.Flag("userdata").Value.String(),
			)
			if err != nil {
				return err
			}

			actuator := utils.CreateActuator(machine, userData)
			err = actuator.Create(cluster, machine)
			if err != nil {
				return err
			}
			fmt.Printf("Machine creation was successful!\n")
			return nil
		},
	}
}

func deleteCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "delete INSTANCE-ID",
		Short: "Delete machine instance",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := checkFlags(cmd); err != nil {
				return err
			}

			cluster, machine, userData, err := utils.ReadClusterResources(
				cmd.Flag("cluster").Value.String(),
				cmd.Flag("machine").Value.String(),
				cmd.Flag("userdata").Value.String(),
			)
			if err != nil {
				return err
			}

			actuator := utils.CreateActuator(machine, userData)
			err = actuator.Delete(cluster, machine)
			if err != nil {
				return err
			}
			fmt.Printf("Machine delete operation was successful.\n")
			return nil
		},
	}
}

func existsCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "exists",
		Short: "Determine if underlying machine instance exists",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := checkFlags(cmd); err != nil {
				return err
			}

			cluster, machine, userData, err := utils.ReadClusterResources(
				cmd.Flag("cluster").Value.String(),
				cmd.Flag("machine").Value.String(),
				cmd.Flag("userdata").Value.String(),
			)
			if err != nil {
				return err
			}

			actuator := utils.CreateActuator(machine, userData)
			exists, err := actuator.Exists(cluster, machine)
			if err != nil {
				return err
			}
			if exists {
				fmt.Printf("Underlying machine's instance exists.\n")
			} else {
				fmt.Printf("Underlying machine's instance not found.\n")
			}
			return nil
		},
	}
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

func createSecretAndWait(f *framework.Framework, secret *apiv1.Secret) error {
	_, err := f.KubeClient.CoreV1().Secrets(secret.Namespace).Create(secret)
	if err != nil {
		return err
	}

	err = wait.Poll(framework.PollInterval, framework.PoolTimeout, func() (bool, error) {
		_, err := f.KubeClient.CoreV1().Secrets(secret.Namespace).Get(secret.Name, metav1.GetOptions{})
		return err == nil, nil
	})
	return err
}

func cmdRun(binaryPath string, args ...string) ([]byte, error) {
	cmd := exec.Command(binaryPath, args...)
	return cmd.CombinedOutput()
}

func bootstrapCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "bootstrap",
		Short: "Bootstrap kubernetes cluster with kubeadm",
		RunE: func(cmd *cobra.Command, args []string) error {

			libvirturi := cmd.Flag("libvirt-uri").Value.String()
			if libvirturi == "" {
				return fmt.Errorf("--libvirt-uri needs to be set")
			}

			inclusterlibvirturi := cmd.Flag("in-cluster-libvirt-uri").Value.String()
			if inclusterlibvirturi == "" {
				return fmt.Errorf("--in-cluster-libvirt-uri needs to be set")
			}

			libvirtpk := cmd.Flag("libvirt-private-key").Value.String()
			if libvirtpk == "" {
				return fmt.Errorf("--libvirt-private-key needs to be set")
			}

			masterguestpk := cmd.Flag("master-guest-private-key").Value.String()
			if masterguestpk == "" {
				return fmt.Errorf("--master-guest-private-key needs to be set")
			}

			testNamespace := &apiv1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test",
				},
			}

			glog.Infof("Creating secret with the libvirt PK from %v", libvirtpk)
			libvirtPKSecret, err := BuildPKSecret("libvirt-private-key", testNamespace.Name, libvirtpk)
			if err != nil {
				return fmt.Errorf("unable to create libvirt-private-key secret: %v", err)
			}

			machinePrefix := cmd.Flag("environment-id").Value.String()

			testCluster := &clusterv1.Cluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      machinePrefix,
					Namespace: testNamespace.Name,
				},
				Spec: clusterv1.ClusterSpec{
					ClusterNetwork: clusterv1.ClusterNetworkingConfig{
						Services: clusterv1.NetworkRanges{
							CIDRBlocks: []string{"10.0.0.1/24"},
						},
						Pods: clusterv1.NetworkRanges{
							CIDRBlocks: []string{"10.0.0.1/24"},
						},
						ServiceDomain: "example.com",
					},
				},
			}

			// Create master machine and verify the master node is ready
			masterUserDataSecret, err := manifests.MasterMachineUserDataSecret(
				"masteruserdatasecret",
				testNamespace.Name,
				[]string{"127.0.0.1"},
			)
			if err != nil {
				return err
			}

			masterMachineProviderConfig, err := testutils.MasterMachineProviderConfig(masterUserDataSecret.Name, libvirturi)
			if err != nil {
				return err
			}
			masterMachine := manifests.MasterMachine(testCluster.Name, testCluster.Namespace, masterMachineProviderConfig)

			glog.Infof("Creating master machine")

			actuator := utils.CreateActuator(masterMachine, masterUserDataSecret)
			err = actuator.Create(testCluster, masterMachine)
			if err != nil {
				return err
			}

			lcw, err := libvirtclient.NewLibvirtClient(libvirturi)
			if err != nil {
				return err
			}

			// Wait until the instance has the ip address
			var masterMachinePrivateIP string
			err = wait.Poll(pollInterval, timeoutPoolInterval, func() (bool, error) {
				privateIP, err := lcw.GetPrivateIP(masterMachine)
				if err != nil {
					return false, nil
				}
				masterMachinePrivateIP = privateIP
				return true, nil
			})
			if err != nil {
				return err
			}

			glog.Infof("Master machine running at %v", masterMachinePrivateIP)

			f := framework.Framework{
				SSH: &framework.SSHConfig{
					Key:  masterguestpk,
					User: "fedora",
				},
			}

			glog.Infof("Collecting master kubeconfig")
			config, err := f.GetMasterMachineRestConfig(masterMachine, lcw)
			if err != nil {
				return err
			}

			clusterFramework, err := framework.NewFrameworkFromConfig(
				config,
				&framework.SSHConfig{
					Key:  masterguestpk,
					User: "fedora",
				},
			)
			if err != nil {
				return err
			}

			clusterFramework.ErrNotExpected = func(err error) {
				if err != nil {
					panic(err)
				}
			}

			clusterFramework.By = func(msg string) {
				glog.Info(msg)
			}

			glog.Info("Waiting for all nodes to come up")
			err = clusterFramework.WaitForNodesToGetReady(1)
			if err != nil {
				return err
			}

			glog.Infof("Creating %q namespace", testNamespace.Name)
			if _, err := clusterFramework.KubeClient.CoreV1().Namespaces().Create(testNamespace); err != nil {
				return err
			}

			glog.Infof("Creating %q secret", testNamespace.Name)
			if _, err := clusterFramework.KubeClient.CoreV1().Secrets(libvirtPKSecret.Namespace).Create(libvirtPKSecret); err != nil {
				return err
			}

			clusterFramework.DeployClusterAPIStack(testNamespace.Name, "openshift/origin-libvirt-machine-controllers:v4.0.0", "libvirt-private-key")
			clusterFramework.CreateClusterAndWait(testCluster)

			workerUserDataSecret, err := manifests.WorkerMachineUserDataSecret("workeruserdatasecret", testNamespace.Name, masterMachinePrivateIP)
			if err != nil {
				return err
			}

			createSecretAndWait(clusterFramework, workerUserDataSecret)
			workerMachineSetProviderConfig, err := testutils.WorkerMachineProviderConfig(workerUserDataSecret.Name, inclusterlibvirturi)
			if err != nil {
				return err
			}
			workerMachineSet := manifests.WorkerMachineSet(testCluster.Name, testCluster.Namespace, workerMachineSetProviderConfig)
			clusterFramework.CreateMachineSetAndWait(workerMachineSet, lcw)

			return nil
		},
	}

	// To bootstrap the master guest that will run the cluster api stack
	cmd.PersistentFlags().StringP("libvirt-uri", "", "", "Libvirt URI. E.g. qemu//system")
	// libvirt URI for actuator running inside a container (as part of the cluster API stack)
	cmd.PersistentFlags().StringP("in-cluster-libvirt-uri", "", "", "Libvirt URI for docker container. E.g. qemu+ssh://root@IP/system")
	// ssh private key so the actuator running inside the container can talk to libvirt running in libvirt instance (in case qemu+ssh is used)
	cmd.PersistentFlags().StringP("libvirt-private-key", "", "", "Private key file for libvirt qemu+ssh URI")
	// ssh private key to pull kubeconfig from master guest
	cmd.PersistentFlags().StringP("master-guest-private-key", "", "", "Private key file of the master guest to pull kubeconfig")

	return cmd
}

func init() {
	rootCmd.PersistentFlags().StringP("machine", "m", "", "Machine manifest")
	rootCmd.PersistentFlags().StringP("cluster", "c", "", "Cluster manifest")
	rootCmd.PersistentFlags().StringP("userdata", "u", "", "User data manifest")

	cUser, err := user.Current()
	if err != nil {
		rootCmd.PersistentFlags().StringP("environment-id", "p", "", "Directory with bootstrapping manifests")
	} else {
		rootCmd.PersistentFlags().StringP("environment-id", "p", cUser.Username, "Machine prefix, by default set to the current user")
	}

	rootCmd.AddCommand(createCommand())
	rootCmd.AddCommand(deleteCommand())
	rootCmd.AddCommand(existsCommand())
	rootCmd.AddCommand(bootstrapCommand())

	flag.CommandLine.AddGoFlagSet(goflag.CommandLine)

	// the following line exists to make glog happy, for more information, see: https://github.com/kubernetes/kubernetes/issues/17162
	flag.CommandLine.Parse([]string{})
}

func checkFlags(cmd *cobra.Command) error {
	if cmd.Flag("cluster").Value.String() == "" {
		return fmt.Errorf("--%v/-%v flag is required", cmd.Flag("cluster").Name, cmd.Flag("cluster").Shorthand)
	}
	if cmd.Flag("machine").Value.String() == "" {
		return fmt.Errorf("--%v/-%v flag is required", cmd.Flag("machine").Name, cmd.Flag("machine").Shorthand)
	}
	return nil
}

func main() {
	err := rootCmd.Execute()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error occurred: %v\n", err)
		os.Exit(1)
	}
}
