/*
Copyright 2020 The Kubernetes Authors.

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

package webhook

import (
	"fmt"
	"math/rand"
	"reflect"
	"testing"

	fuzz "github.com/google/gofuzz"

	v1 "k8s.io/api/admission/v1"
	"k8s.io/api/admission/v1beta1"
	"k8s.io/apimachinery/pkg/api/apitesting/fuzzer"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apimachinery/pkg/util/diff"
)

func TestConvertAdmissionRequestToV1(t *testing.T) {
	f := fuzzer.FuzzerFor(AdmissionfuzzerFuncs, rand.NewSource(rand.Int63()), serializer.NewCodecFactory(runtime.NewScheme()))
	for i := 0; i < 100; i++ {
		t.Run(fmt.Sprintf("Run %d/100", i), func(t *testing.T) {
			orig := &v1beta1.AdmissionRequest{}
			f.Fuzz(orig)
			converted := convertAdmissionRequestToV1(orig)
			rt := convertAdmissionRequestToV1beta1(converted)
			if !reflect.DeepEqual(orig, rt) {
				t.Errorf("expected all request fields to be in converted object but found unaccounted for differences, diff:\n%s", diff.ObjectReflectDiff(orig, converted))
			}
		})
	}
}

func TestConvertAdmissionResponseToV1beta1(t *testing.T) {
	f := fuzz.New()
	for i := 0; i < 100; i++ {
		t.Run(fmt.Sprintf("Run %d/100", i), func(t *testing.T) {
			orig := &v1.AdmissionResponse{}
			f.Fuzz(orig)
			converted := convertAdmissionResponseToV1beta1(orig)
			rt := convertAdmissionResponseToV1(converted)
			if !reflect.DeepEqual(orig, rt) {
				t.Errorf("expected all fields to be in converted object but found unaccounted for differences, diff:\n%s", diff.ObjectReflectDiff(orig, converted))
			}
		})
	}
}
