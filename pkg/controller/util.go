/*
Copyright 2018 The Kubernetes Authors.

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

package controller

import (
	"fmt"
	"github.com/golang/glog"
	crdv1 "github.com/kubernetes-csi/external-snapshotter/pkg/apis/volumesnapshot/v1alpha1"
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/cache"
	"strconv"
	"strings"
)

var (
	keyFunc = cache.DeletionHandlingMetaNamespaceKeyFunc
)

// GetNameAndNameSpaceFromSnapshotName retrieves the namespace and
// the short name of a snapshot from its full name
func GetNameAndNameSpaceFromSnapshotName(name string) (string, string, error) {
	strs := strings.Split(name, "/")
	if len(strs) != 2 {
		return "", "", fmt.Errorf("invalid snapshot name")
	}
	return strs[0], strs[1], nil
}

func snapshotKey(vs *crdv1.VolumeSnapshot) string {
	return fmt.Sprintf("%s/%s", vs.Namespace, vs.Name)
}

func snapshotRefKey(vsref *v1.ObjectReference) string {
	return fmt.Sprintf("%s/%s", vsref.Namespace, vsref.Name)
}

// storeObjectUpdate updates given cache with a new object version from Informer
// callback (i.e. with events from etcd) or with an object modified by the
// controller itself. Returns "true", if the cache was updated, false if the
// object is an old version and should be ignored.
func storeObjectUpdate(store cache.Store, obj interface{}, className string) (bool, error) {
	objName, err := keyFunc(obj)
	if err != nil {
		return false, fmt.Errorf("Couldn't get key for object %+v: %v", obj, err)
	}
	oldObj, found, err := store.Get(obj)
	if err != nil {
		return false, fmt.Errorf("Error finding %s %q in controller cache: %v", className, objName, err)
	}

	objAccessor, err := meta.Accessor(obj)
	if err != nil {
		return false, err
	}

	if !found {
		// This is a new object
		glog.V(4).Infof("storeObjectUpdate: adding %s %q, version %s", className, objName, objAccessor.GetResourceVersion())
		if err = store.Add(obj); err != nil {
			return false, fmt.Errorf("error adding %s %q to controller cache: %v", className, objName, err)
		}
		return true, nil
	}

	oldObjAccessor, err := meta.Accessor(oldObj)
	if err != nil {
		return false, err
	}

	objResourceVersion, err := strconv.ParseInt(objAccessor.GetResourceVersion(), 10, 64)
	if err != nil {
		return false, fmt.Errorf("error parsing ResourceVersion %q of %s %q: %s", objAccessor.GetResourceVersion(), className, objName, err)
	}
	oldObjResourceVersion, err := strconv.ParseInt(oldObjAccessor.GetResourceVersion(), 10, 64)
	if err != nil {
		return false, fmt.Errorf("error parsing old ResourceVersion %q of %s %q: %s", oldObjAccessor.GetResourceVersion(), className, objName, err)
	}

	// Throw away only older version, let the same version pass - we do want to
	// get periodic sync events.
	if oldObjResourceVersion > objResourceVersion {
		glog.V(4).Infof("storeObjectUpdate: ignoring %s %q version %s", className, objName, objAccessor.GetResourceVersion())
		return false, nil
	}

	glog.V(4).Infof("storeObjectUpdate updating %s %q with version %s", className, objName, objAccessor.GetResourceVersion())
	if err = store.Update(obj); err != nil {
		return false, fmt.Errorf("error updating %s %q in controller cache: %v", className, objName, err)
	}
	return true, nil
}

// GetSnapshotContentNameForSnapshot returns SnapshotData.Name for the create VolumeSnapshotContent.
// The name must be unique.
func GetSnapshotContentNameForSnapshot(snapshot *crdv1.VolumeSnapshot) string {
	return "snapdata-" + string(snapshot.UID)
}

// IsDefaultAnnotation returns a boolean if
// the annotation is set
func IsDefaultAnnotation(obj metav1.ObjectMeta) bool {
	if obj.Annotations[IsDefaultSnapshotClassAnnotation] == "true" {
		return true
	}

	return false
}
