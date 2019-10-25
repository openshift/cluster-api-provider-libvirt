package client

import (
	"bytes"
	"fmt"
	"io"
	"os/exec"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/golang/glog"
	libvirtxml "github.com/libvirt/libvirt-go-xml"
	providerconfigv1 "github.com/openshift/cluster-api-provider-libvirt/pkg/apis/libvirtproviderconfig/v1beta1"
	"github.com/pkg/errors"
)

var execCommand = exec.Command

func setIgnitionForS390X(domainDef *libvirtxml.Domain, client *libvirtClient, ignition *providerconfigv1.Ignition, kubeClient kubernetes.Interface, machineNamespace, volumeName string) error {
	glog.Info("Creating ignition file for s390x")
	ignitionDef := newIgnitionDef()

	if ignition.UserDataSecret == "" {
		return fmt.Errorf("ignition.userDataSecret not set")
	}

	secret, err := kubeClient.CoreV1().Secrets(machineNamespace).Get(ignition.UserDataSecret, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("can not retrieve user data secret '%v/%v' when constructing cloud init volume: %v", machineNamespace, ignition.UserDataSecret, err)
	}
	userDataSecret, ok := secret.Data["userData"]
	if !ok {
		return fmt.Errorf("can not retrieve user data secret '%v/%v' when constructing cloud init volume: key 'userData' not found in the secret", machineNamespace, ignition.UserDataSecret)
	}

	ignitionDef.Name = volumeName
	ignitionDef.PoolName = client.poolName
	ignitionDef.Content = string(userDataSecret)

	glog.Infof("Ignition: %+v", ignitionDef)

	ignitionVolumeName, err := ignitionDef.createAndUpload(client)
	if err != nil {
		return err
	}

	// _fw_cfg isn't supported on s390x, so we use guestfish to inject the ignition for now
	connURI, err := client.connection.GetURI()
	if err != nil {
		return err
	}
	virHost := strings.Split(strings.SplitAfter(connURI, "//")[1], "/")[0]
	if virHost != "" {
		return sshInjectIgnitionByGuestfish(domainDef, ignitionVolumeName, virHost)
	}
	return injectIgnitionByGuestfish(domainDef, ignitionVolumeName)
}

func injectIgnitionByGuestfish(domainDef *libvirtxml.Domain, ignitionFile string) error {
	glog.Info("Injecting ignition configuration using guestfish")

	runAsRoot := true

	/*
	 * Add the image into guestfish, execute the following command,
	 *     guestfish --listen -a ${volumeFilePath}
	 *
	 * output example:
	 *     GUESTFISH_PID=4513; export GUESTFISH_PID
	 */
	args := []string{"--listen", "-a", domainDef.Devices.Disks[0].Source.File.File}
	output, err := startCmd(runAsRoot, nil, args...)
	if err != nil {
		return err
	}

	strArray := strings.Split(output, ";")
	if len(strArray) != 2 {
		return fmt.Errorf("invalid output when starting guestfish: %s", output)
	}
	strArray1 := strings.Split(strArray[0], "=")
	if len(strArray1) != 2 {
		return fmt.Errorf("failed to get the guestfish PID from %s", output)
	}
	env := []string{strArray[0]}

	/*
	 * Launch guestfish, execute the following command,
	 *     guestfish --remote -- run
	 */
	args = []string{"--remote", "--", "run"}
	_, err = execCmd(runAsRoot, env, args...)
	if err != nil {
		return err
	}

	/*
	 * Get the boot filesystem, execute the following command,
	 *     findfs-label boot
	 *
	 * output example:
	 *     /dev/sda1
	 */
	args = []string{"--remote", "--", "findfs-label", "boot"}
	output, err = execCmd(runAsRoot, env, args...)
	if err != nil {
		return err
	}

	bootDisk := strings.TrimSpace(output)
	if len(bootDisk) == 0 {
		return fmt.Errorf("failed to get the boot filesystem")
	}

	/*
	 * Mount the boot filesystem, execute the following command,
	 *     guestfish --remote -- mount ${boot_filesystem} /
	 */
	args = []string{"--remote", "--", "mount", bootDisk, "/"}
	_, err = execCmd(runAsRoot, env, args...)
	if err != nil {
		return err
	}

	/*
	 * Upload the ignition file, execute the following command,
	 *     guestfish --remote -- upload ${ignition_filepath} /ignition/config.ign
	 *
	 * The target path is hard coded as "/ignition/config.ign" for now
	 */
	args = []string{"--remote", "--", "upload", ignitionFile, "/ignition/config.ign"}
	_, err = execCmd(runAsRoot, env, args...)
	if err != nil {
		return err
	}

	/*
	 * Umount all filesystems, execute the following command,
	 *     guestfish --remote -- umount-all
	 */
	args = []string{"--remote", "--", "umount-all"}
	_, err = execCmd(runAsRoot, env, args...)
	if err != nil {
		return err
	}

	/*
	 * Exit guestfish, execute the following command,
	 *     guestfish --remote -- exit
	 */
	args = []string{"--remote", "--", "exit"}
	_, err = execCmd(runAsRoot, env, args...)
	if err != nil {
		return err
	}

	return nil
}

func execCmd(runAsRoot bool, env []string, args ...string) (string, error) {
	cmd := genCmd(runAsRoot, env, args...)
	glog.Infof("Running: %v", cmd.Args)

	cmdOut, err := cmd.CombinedOutput()
	glog.Infof("Ran: %v Output: %v", cmd.Args, string(cmdOut))
	if err != nil {
		err = errors.Wrapf(err, "error running command '%v'", strings.Join(cmd.Args, " "))
	}
	return string(cmdOut), err
}

// startCmd starts the command, and doesn't wait for it to complete
func startCmd(runAsRoot bool, env []string, args ...string) (string, error) {
	cmd := genCmd(runAsRoot, env, args...)
	glog.Infof("Starting: %v", cmd.Args)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", errors.Wrapf(err, "error getting stdout pipe for command '%v'", strings.Join(cmd.Args, " "))
	}
	err = cmd.Start()
	glog.Infof("Started: %v", cmd.Args)
	if err != nil {
		return "", errors.Wrapf(err, "error starting command '%v'", strings.Join(cmd.Args, " "))
	}

	outMsg, err := readOutput(stdout)
	glog.Infof("Output message: %s", outMsg)

	return outMsg, err
}

func genCmd(runAsRoot bool, env []string, args ...string) *exec.Cmd {
	executable := "guestfish"
	newArgs := []string{}
	if runAsRoot {
		newArgs = append(newArgs, []string{"--preserve-env", executable}...)
		newArgs = append(newArgs, args...)
		executable = "sudo"
	} else {
		newArgs = args
	}
	cmd := execCommand(executable, newArgs...)
	if env != nil && len(env) > 0 {
		cmd.Env = append(cmd.Env, env...)
	}
	return cmd
}

func readOutput(stream io.ReadCloser) (string, error) {
	var buf bytes.Buffer
	_, err := buf.ReadFrom(stream)
	if err != nil {
		return "", err
	}

	return buf.String(), nil
}
