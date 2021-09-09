module github.com/openshift/cluster-api-provider-libvirt

go 1.16

require (
	github.com/davecgh/go-spew v1.1.1
	github.com/elazarl/goproxy v0.0.0-20200426045556-49ad98f6dac1 // indirect
	github.com/ghodss/yaml v1.0.0
	github.com/golang/glog v0.0.0-20160126235308-23def4e6c14b
	github.com/golang/mock v1.5.0
	github.com/libvirt/libvirt-go v5.10.0+incompatible
	github.com/libvirt/libvirt-go-xml v5.10.0+incompatible
	github.com/openshift/machine-api-operator v0.2.1-0.20210909104916-c2b7a9b41ccc
	k8s.io/api v0.22.1
	k8s.io/apiextensions-apiserver v0.22.1 // indirect
	k8s.io/apimachinery v0.22.1
	k8s.io/client-go v0.22.1
	k8s.io/code-generator v0.22.1
	k8s.io/gengo v0.0.0-20210813121822-485abfe95c7c // indirect
	k8s.io/klog v1.0.0
	sigs.k8s.io/controller-runtime v0.9.6
)

replace (
	github.com/golang/mock => github.com/golang/mock v1.3.1
	sigs.k8s.io/cluster-api-provider-aws => github.com/openshift/cluster-api-provider-aws v0.2.1-0.20210907081340-a815e7e7e6f7
	sigs.k8s.io/cluster-api-provider-azure => github.com/openshift/cluster-api-provider-azure v0.1.0-alpha.3.0.20210820171458-c689e78f7052
)
