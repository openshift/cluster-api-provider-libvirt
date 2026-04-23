package client

import (
	"runtime"
	"testing"

	libvirtxml "github.com/libvirt/libvirt-go-xml"
)

func TestClearX86OnlyDomainFeaturesForArchS390x(t *testing.T) {
	d := newDomainDef()
	if d.Features == nil || d.Features.ACPI == nil {
		t.Fatal("newDomainDef should set x86-style features including ACPI")
	}
	d.OS.Type.Arch = "s390x"
	clearX86OnlyDomainFeaturesForArch(&d)
	if d.Features != nil {
		t.Fatalf("s390x: expected Features=nil, got %#v", d.Features)
	}
}

func TestClearX86OnlyDomainFeaturesForArchAmd64(t *testing.T) {
	d := newDomainDef()
	orig := d.Features
	d.OS.Type.Arch = "x86_64"
	clearX86OnlyDomainFeaturesForArch(&d)
	if d.Features != orig {
		t.Fatal("x86_64: Features should be unchanged")
	}
}

func TestSetCoreOSIgnition(t *testing.T) {
	testCases := []struct {
		ignKey       string
		expected     string
		errorMessage string
	}{
		{
			ignKey:       "myIgnitionConfig",
			expected:     "name=opt/com.coreos/config,file=myIgnitionConfig",
			errorMessage: "",
		},
		{
			ignKey:       "",
			expected:     "",
			errorMessage: "error setting coreos ignition, ignKey is empty",
		},
	}

TestCases:
	for i, tc := range testCases {
		domainDef := libvirtxml.Domain{}

		err := setCoreOSIgnition(&domainDef, tc.ignKey, runtime.GOARCH)
		// if err, verify it returns expected error
		if err != nil {
			if err.Error() != tc.errorMessage {
				t.Errorf("test case %d: failed to return error when key is empty. Got: %s, Expected: %s", i, err.Error(), tc.errorMessage)
			}
		} else {
			// verify it sets right ign config
			for i, v := range domainDef.QEMUCommandline.Args {
				if v.Value == "-fw_cfg" {
					if domainDef.QEMUCommandline.Args[i+1].Value == "name=opt/com.coreos/config,file=myIgnitionConfig" {
						continue TestCases
					}
				}
			}
			t.Errorf("test case %d: failed to setCoreOSIgnition for key %s. Expected: %s", i, tc.ignKey, tc.expected)
		}
	}
}
