/*
Copyright 2017 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// NOTE: This file is copied from
// https://github.com/kubernetes/kubernetes/blob/v1.29.0-alpha.0/pkg/apis/admission/fuzzer/fuzzer.go
// so that external-snapshotter no longer needs to depend on k8s.io/kubernetes

package webhook

import (
	fuzz "github.com/google/gofuzz"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	runtimeserializer "k8s.io/apimachinery/pkg/runtime/serializer"
)

// Funcs returns the fuzzer functions for the admission api group.
var AdmissionfuzzerFuncs = func(codecs runtimeserializer.CodecFactory) []interface{} {
	return []interface{}{
		func(s *runtime.RawExtension, c fuzz.Continue) {
			u := &unstructured.Unstructured{Object: map[string]interface{}{
				"apiVersion": "unknown.group/unknown",
				"kind":       "Something",
				"somekey":    "somevalue",
			}}
			s.Object = u
		},
	}
}
