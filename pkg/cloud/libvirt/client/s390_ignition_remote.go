package client

import (
        "fmt"
        "strings"
	"io/ioutil"

        "github.com/mitchellh/go-homedir"
        "golang.org/x/crypto/ssh"
        libvirtxml "github.com/libvirt/libvirt-go-xml"
)

type SSHConn struct {
        Host    string
        Port    int
        User    string
        Keypath string
        Client  *ssh.Client
        Session *ssh.Session
}

func (s *SSHConn) Connect() error {
        var keypath string
        if s.Keypath == "" {
                homepath, err := homedir.Dir()
                if err != nil {
                        return err
                }
                keypath = homepath + "/.ssh/id_rsa"
        } else {
                keypath = s.Keypath
        }

        key, err := ioutil.ReadFile(keypath)
        if err != nil {
                return err
        }

        signer, err := ssh.ParsePrivateKey(key)
        if err != nil {
                return err
        }

        config := &ssh.ClientConfig{}
        config.SetDefaults()
        config.User = s.User
        config.Auth = []ssh.AuthMethod{ssh.PublicKeys(signer)}
        config.HostKeyCallback = ssh.InsecureIgnoreHostKey()

        addr := fmt.Sprintf("%s:%d", s.Host, s.Port)
        client, err := ssh.Dial("tcp", addr, config)
        if err != nil {
                return err
        } else {
                s.Client = client
                return nil
        }
}

func (s *SSHConn) CombinedOutput(cmd string) (string, error) {
        session, err := s.Client.NewSession()
        if err != nil {
                return "", err
        }
        defer session.Close()

        bs, err := session.CombinedOutput(cmd)
        if err != nil {
                return "", err
        }
        return string(bs), nil
}


func sshInjectIgnitionByGuestfish(domainDef *libvirtxml.Domain, ignitionFile string, connUrl string) error {
        // runAsRoot := true

        /*
         * Add the image into guestfish, execute the following command,
         *     guestfish --listen -a ${volumeFilePath}
         *
         * output example:
         *     GUESTFISH_PID=4513; export GUESTFISH_PID
         */
        virHost := strings.Split(strings.SplitAfter(connUrl, "//")[1], "/")[0]

        sshConn := &SSHConn{
                Host:    virHost,
                Port:    22,
                User:    "root",
                Keypath: "",
        }
        err := sshConn.Connect()
        if err != nil {
                return fmt.Errorf("SSH connect failed: %v", err)
        }
        defer sshConn.Client.Close()
        output, err := sshConn.CombinedOutput("guestfish --listen -a " + domainDef.Devices.Disks[0].Source.File.File)
        if err != nil {
                return err
        }

        env := strings.Split(output, ";")[0]

        /*
         * Launch guestfish, execute the following command,
         *     guestfish --remote -- run
         */
        _, err = sshConn.CombinedOutput(env + " guestfish --remote -- run")
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
	output, err = sshConn.CombinedOutput(env + " guestfish --remote -- findfs-label boot")
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
        _, err = sshConn.CombinedOutput(env + " guestfish --remote -- mount " + bootDisk + " /")
        if err != nil {
                return err
        }

        // guestfish --remote -- mkdir-p /ignition
        _, err= sshConn.CombinedOutput(env + " guestfish --remote -- mkdir-p /ignition")
        if err != nil {
                return fmt.Errorf("Mkdir failed: %v", err)
        }

        /*
         * Upload the ignition file, execute the following command,
         *     guestfish --remote -- upload ${ignition_filepath} /ignition/config.ign
*
         * The target path is hard coded as "/ignition/config.ign" for now
         */
        _, err = sshConn.CombinedOutput(env + " guestfish --remote -- upload " + ignitionFile + " /ignition/config.ign")
        if err != nil {
                return err
        }
	/*
         * Umount all filesystems, execute the following command,
         *     guestfish --remote -- umount-all
         */
        _, err = sshConn.CombinedOutput(env + " guestfish --remote -- umount-all")
        if err != nil {
                return err
        }

        /*
         * Exit guestfish, execute the following command,
         *     guestfish --remote -- exit
         */
        _, err = sshConn.CombinedOutput(env + " guestfish --remote -- exit")
        if err != nil {
                return err
        }

        return nil
}
