module github.com/openshift/cluster-api-provider-libvirt

require (
	cloud.google.com/go v0.36.0 // indirect
	github.com/PuerkitoBio/purell v1.1.1 // indirect
	github.com/davecgh/go-spew v1.1.1
	github.com/emicklei/go-restful v2.9.0+incompatible // indirect
	github.com/ghodss/yaml v1.0.0
	github.com/go-log/log v0.0.0-20181211034820-a514cf01a3eb // indirect
	github.com/go-openapi/jsonpointer v0.18.0 // indirect
	github.com/go-openapi/jsonreference v0.18.0 // indirect
	github.com/go-openapi/spec v0.18.0 // indirect
	github.com/go-openapi/swag v0.18.0 // indirect
	github.com/golang/glog v0.0.0-20160126235308-23def4e6c14b
	github.com/golang/groupcache v0.0.0-20190129154638-5b532d6fd5ef // indirect
	github.com/golang/mock v1.2.0
	github.com/google/uuid v1.1.1 // indirect
	github.com/hashicorp/golang-lru v0.5.1 // indirect
	github.com/imdario/mergo v0.3.7 // indirect
	github.com/inconshreveable/mousetrap v1.0.0 // indirect
	github.com/libvirt/libvirt-go v4.10.0+incompatible
	github.com/libvirt/libvirt-go-xml v4.10.0+incompatible
	github.com/mailru/easyjson v0.0.0-20190312143242-1de009706dbe // indirect
	github.com/onsi/ginkgo v1.7.0
	github.com/onsi/gomega v1.4.3
	github.com/openshift/cluster-api v0.0.0-20190805113604-f8de78af80fc
	github.com/openshift/cluster-api-actuator-pkg v0.0.0-20190614215203-42228d06a2ca
	github.com/openshift/cluster-autoscaler-operator v0.0.0-20190521201101-62768a6ba480 // indirect
	github.com/openshift/machine-api-operator v0.0.0-20190312153711-9650e16c9880 // indirect
	github.com/pborman/uuid v0.0.0-20180906182336-adf5a7427709 // indirect
	github.com/pkg/errors v0.8.1
	github.com/prometheus/client_golang v0.9.2 // indirect
	github.com/prometheus/client_model v0.0.0-20190129233127-fd36f4220a90 // indirect
	github.com/prometheus/common v0.2.0 // indirect
	github.com/prometheus/procfs v0.0.0-20190227231451-bbced9601137 // indirect
	github.com/spf13/cobra v0.0.3
	github.com/spf13/pflag v1.0.3
	golang.org/x/crypto v0.0.0-20190228161510-8dd112bcdc25 // indirect
	golang.org/x/net v0.0.0-20190228165749-92fc7df08ae7 // indirect
	golang.org/x/oauth2 v0.0.0-20190226205417-e64efc72b421 // indirect
	golang.org/x/sys v0.0.0-20190228124157-a34e9553db1e // indirect
	gopkg.in/yaml.v2 v2.2.2 // indirect
	k8s.io/api v0.0.0-20190409021203-6e4e0e4f393b
	k8s.io/apimachinery v0.0.0-20190404173353-6a84e37a896d
	k8s.io/client-go v11.0.1-0.20190409021438-1a26190bd76a+incompatible
	k8s.io/code-generator v0.0.0-20191003035328-700b1226c0bd
	k8s.io/gengo v0.0.0-20190907103519-ebc107f98eab // indirect
	k8s.io/klog v0.3.0
	k8s.io/kube-openapi v0.0.0-20190228160746-b3a7cee44a30 // indirect
	k8s.io/utils v0.0.0-20190529001817-6999998975a7 // indirect
	sigs.k8s.io/controller-runtime v0.0.0-20190520212815-96b67f231945
)

replace (
	github.com/davecgh/go-spew => github.com/davecgh/go-spew v1.1.1
	github.com/ghodss/yaml => github.com/ghodss/yaml v1.0.0
	github.com/gogo/protobuf => github.com/gogo/protobuf v1.1.0
	github.com/golang/mock => github.com/golang/mock v1.2.0
	github.com/golang/protobuf => github.com/golang/protobuf v1.1.0
	github.com/libvirt/libvirt-go => github.com/libvirt/libvirt-go v4.6.0+incompatible
	github.com/libvirt/libvirt-go-xml => github.com/libvirt/libvirt-go-xml v4.6.0+incompatible
	github.com/openshift/cluster-api => github.com/openshift/cluster-api v0.0.0-20190917100308-655e2d6ccdd5
	github.com/spf13/cobra => github.com/spf13/cobra v0.0.3
	github.com/spf13/pflag => github.com/spf13/pflag v1.0.2
	gopkg.in/fsnotify.v1 => github.com/fsnotify/fsnotify v1.4.7
	k8s.io/api => k8s.io/api v0.0.0-20190409021203-6e4e0e4f393b
	k8s.io/apiextensions-apiserver => k8s.io/apiextensions-apiserver v0.0.0-20190409022649-727a075fdec8
	k8s.io/apimachinery => k8s.io/apimachinery v0.0.0-20190404173353-6a84e37a896d
	k8s.io/client-go => k8s.io/client-go v11.0.1-0.20190409021438-1a26190bd76a+incompatible
	k8s.io/code-generator => k8s.io/code-generator v0.0.0-20190311093542-50b561225d70
	sigs.k8s.io/controller-runtime => sigs.k8s.io/controller-runtime v0.2.0-beta.1.0.20190520212815-96b67f231945
	sigs.k8s.io/controller-tools => sigs.k8s.io/controller-tools v0.1.1
)
