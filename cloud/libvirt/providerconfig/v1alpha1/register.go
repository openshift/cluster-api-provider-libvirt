// Copyright © 2018 The Kubernetes Authors.
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package v1alpha1

import (
	"bytes"
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	clusterv1 "sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1"

	"github.com/openshift/cluster-api-provider-libvirt/cloud/libvirt/providerconfig"
)

// LibvirtProviderConfigCodec contains encoder/decoder to convert this types from/to serialize data
// +k8s:deepcopy-gen=false
type LibvirtProviderConfigCodec struct {
	encoder runtime.Encoder
	decoder runtime.Decoder
}

// GroupName is the group which identify this API
const GroupName = "libvirtproviderconfig"

// SchemeGroupVersion contains the group and version which register these types
var SchemeGroupVersion = schema.GroupVersion{Group: GroupName, Version: "v1alpha1"}

var (
	// SchemeBuilder contains the functions to add the libvirtProviderConfig types
	SchemeBuilder      runtime.SchemeBuilder
	localSchemeBuilder = &SchemeBuilder

	// AddToScheme applies functions to SchemeBuilder
	AddToScheme = localSchemeBuilder.AddToScheme
)

var (
	// Scheme contains the methods
	// for serializing and deserializing these API objects
	Scheme, _ = NewScheme()
	// Codecs provides methods for retrieving serializers for this Scheme
	Codecs = serializer.NewCodecFactory(Scheme)
	// Encoder targets SchemeGroupVersionprovided
	Encoder, _ = newEncoder(&Codecs)
)

func init() {
	localSchemeBuilder.Register(addKnownTypes)
}

func addKnownTypes(scheme *runtime.Scheme) error {
	scheme.AddKnownTypes(SchemeGroupVersion,
		&LibvirtMachineProviderConfig{},
	)
	scheme.AddKnownTypes(SchemeGroupVersion,
		&LibvirtClusterProviderConfig{},
	)
	scheme.AddKnownTypes(SchemeGroupVersion,
		&LibvirtMachineProviderStatus{},
	)
	scheme.AddKnownTypes(SchemeGroupVersion,
		&LibvirtClusterProviderStatus{},
	)
	return nil
}

// NewScheme creates a new schema with the necessary methods
// for serializing and deserializing these API objects
func NewScheme() (*runtime.Scheme, error) {
	scheme := runtime.NewScheme()
	if err := AddToScheme(scheme); err != nil {
		return nil, err
	}
	if err := providerconfig.AddToScheme(scheme); err != nil {
		return nil, err
	}
	addKnownTypes(scheme)
	return scheme, nil
}

// NewCodec returns a encode/decoder for this API
func NewCodec() (*LibvirtProviderConfigCodec, error) {
	scheme, err := NewScheme()
	if err != nil {
		return nil, err
	}
	codecFactory := serializer.NewCodecFactory(scheme)
	encoder, err := newEncoder(&codecFactory)
	if err != nil {
		return nil, err
	}
	codec := LibvirtProviderConfigCodec{
		encoder: encoder,
		decoder: codecFactory.UniversalDecoder(SchemeGroupVersion),
	}
	return &codec, nil
}

// DecodeFromProviderConfig decodes a serialised ProviderConfig into an object
func (codec *LibvirtProviderConfigCodec) DecodeFromProviderConfig(providerConfig clusterv1.ProviderConfig, out runtime.Object) error {
	if providerConfig.Value != nil {
		_, _, err := codec.decoder.Decode(providerConfig.Value.Raw, nil, out)
		if err != nil {
			return fmt.Errorf("decoding failure: %v", err)
		}
	}
	return nil
}

// EncodeToProviderConfig encodes an object into a serialised ProviderConfig
func (codec *LibvirtProviderConfigCodec) EncodeToProviderConfig(in runtime.Object) (*clusterv1.ProviderConfig, error) {
	var buf bytes.Buffer
	if err := codec.encoder.Encode(in, &buf); err != nil {
		return nil, fmt.Errorf("encoding failed: %v", err)
	}
	return &clusterv1.ProviderConfig{
		Value: &runtime.RawExtension{Raw: buf.Bytes()},
	}, nil
}

// EncodeProviderStatus encodes an object into serialised data
func (codec *LibvirtProviderConfigCodec) EncodeProviderStatus(in runtime.Object) (*runtime.RawExtension, error) {
	var buf bytes.Buffer
	if err := codec.encoder.Encode(in, &buf); err != nil {
		return nil, fmt.Errorf("encoding failed: %v", err)
	}

	return &runtime.RawExtension{Raw: buf.Bytes()}, nil
}

// DecodeProviderStatus decodes a serialised providerStatus into an object
func (codec *LibvirtProviderConfigCodec) DecodeProviderStatus(providerStatus *runtime.RawExtension, out runtime.Object) error {
	if providerStatus != nil {
		_, _, err := codec.decoder.Decode(providerStatus.Raw, nil, out)
		if err != nil {
			return fmt.Errorf("decoding failure: %v", err)
		}
	}
	return nil
}

func newEncoder(codecFactory *serializer.CodecFactory) (runtime.Encoder, error) {
	serializerInfos := codecFactory.SupportedMediaTypes()
	if len(serializerInfos) == 0 {
		return nil, fmt.Errorf("unable to find any serlializers")
	}
	encoder := codecFactory.EncoderForVersion(serializerInfos[0].Serializer, SchemeGroupVersion)
	return encoder, nil
}
