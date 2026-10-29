/*
Copyright 2026 The Kubernetes Authors.

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

package topology

import (
	"sort"

	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/labels"
	storagelisters "k8s.io/client-go/listers/storage/v1"
)

const (
	isDefaultStorageClassAnnotation     = "storageclass.kubernetes.io/is-default-class"
	betaIsDefaultStorageClassAnnotation = "storageclass.beta.kubernetes.io/is-default-class"
)

// resolveStorageClass treats nil as default and "" as no class. Defaults follow
// admission ordering: newest creation time, then name. A missing class returns nil.
func resolveStorageClass(lister storagelisters.StorageClassLister, name *string) *storagev1.StorageClass {
	if name != nil {
		if *name == "" {
			return nil
		}
		sc, err := lister.Get(*name)
		if err != nil {
			return nil
		}
		return sc
	}

	all, err := lister.List(labels.Everything())
	if err != nil {
		return nil
	}
	var defaults []*storagev1.StorageClass
	for _, sc := range all {
		if isDefaultClass(sc) {
			defaults = append(defaults, sc)
		}
	}
	if len(defaults) == 0 {
		return nil
	}
	sort.Slice(defaults, func(i, j int) bool {
		ti, tj := defaults[i].CreationTimestamp.UnixNano(), defaults[j].CreationTimestamp.UnixNano()
		if ti == tj {
			return defaults[i].Name < defaults[j].Name
		}
		return ti > tj
	})
	return defaults[0]
}

func isDefaultClass(sc *storagev1.StorageClass) bool {
	return sc.Annotations[isDefaultStorageClassAnnotation] == "true" ||
		sc.Annotations[betaIsDefaultStorageClassAnnotation] == "true"
}

func isWaitForFirstConsumer(sc *storagev1.StorageClass) bool {
	return sc != nil && sc.VolumeBindingMode != nil && *sc.VolumeBindingMode == storagev1.VolumeBindingWaitForFirstConsumer
}
