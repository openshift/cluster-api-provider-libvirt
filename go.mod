module github.com/openshift/cluster-api-provider-libvirt

go 1.14

require (
	github.com/davecgh/go-spew v1.1.1
	github.com/docker/spdystream v0.0.0-20181023171402-6480d4af844c // indirect
	github.com/elazarl/goproxy v0.0.0-20200426045556-49ad98f6dac1 // indirect
	github.com/ghodss/yaml v1.0.0
	github.com/golang/glog v0.0.0-20160126235308-23def4e6c14b
	github.com/golang/mock v1.2.0
	github.com/golang/protobuf v1.4.2 // indirect
	github.com/json-iterator/go v1.1.9 // indirect
	github.com/libvirt/libvirt-go v4.10.0+incompatible
	github.com/libvirt/libvirt-go-xml v4.10.0+incompatible
	github.com/openshift/machine-api-operator v0.2.1-0.20200513150041-09efe6c914b4
	github.com/pkg/errors v0.9.1 // indirect
	github.com/spf13/cobra v1.0.0 // indirect
	golang.org/x/crypto v0.0.0-20200510223506-06a226fb4e37 // indirect
	golang.org/x/mod v0.3.0 // indirect
	golang.org/x/net v0.0.0-20200519113804-d87ec0cfa476 // indirect
	golang.org/x/oauth2 v0.0.0-20200107190931-bf48bf16ab8d // indirect
	golang.org/x/sys v0.0.0-20200519105757-fe76b779f299 // indirect
	golang.org/x/time v0.0.0-20200416051211-89c76fbcd5d1 // indirect
	golang.org/x/tools v0.0.0-20200519175826-7521f6f42533 // indirect
	google.golang.org/appengine v1.6.6 // indirect
	gopkg.in/yaml.v2 v2.3.0 // indirect
	k8s.io/api v0.18.2
	k8s.io/apiextensions-apiserver v0.18.2 // indirect
	k8s.io/apimachinery v0.18.2
	k8s.io/client-go v0.18.2
	k8s.io/code-generator v0.18.2
	k8s.io/gengo v0.0.0-20200518160137-fb547a11e5e0 // indirect
	k8s.io/klog v1.0.0
	k8s.io/utils v0.0.0-20200414100711-2df71ebbae66 // indirect
	sigs.k8s.io/controller-runtime v0.5.1-0.20200330174416-a11a908d91e0
)

replace (
	github.com/libvirt/libvirt-go => github.com/libvirt/libvirt-go v4.6.0+incompatible
	github.com/libvirt/libvirt-go-xml => github.com/libvirt/libvirt-go-xml v4.6.0+incompatible
)
