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
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"os"
	"strconv"
)

var (
	keyFunc = cache.DeletionHandlingMetaNamespaceKeyFunc
)

const snapshotterSecretNameKey = "csiSnapshotterSecretName"
const snapshotterSecretNamespaceKey = "csiSnapshotterSecretNamespace"

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

// GetSnapshotContentNameForSnapshot returns SnapshotContent.Name for the create VolumeSnapshotContent.
// The name must be unique.
func GetSnapshotContentNameForSnapshot(snapshot *crdv1.VolumeSnapshot) string {
	return "snapcontent-" + string(snapshot.UID)
}

// IsDefaultAnnotation returns a boolean if
// the annotation is set
func IsDefaultAnnotation(obj metav1.ObjectMeta) bool {
	if obj.Annotations[IsDefaultSnapshotClassAnnotation] == "true" {
		return true
	}

	return false
}

// GetSecretReference returns a reference to the secret specified in the given nameKey and namespaceKey parameters, or an error if the parameters are not specified correctly.
// if neither the name or namespace parameter are set, a nil reference and no error is returned.
// no lookup of the referenced secret is performed, and the secret may or may not exist.
//
// supported tokens for name resolution:
// - ${volumesnapshotcontent.name}
// - ${volumesnapshot.namespace}
// - ${volumesnapshot.name}
// - ${volumesnapshot.annotations['ANNOTATION_KEY']} (e.g. ${pvc.annotations['example.com/snapshot-create-secret-name']})
//
// supported tokens for namespace resolution:
// - ${volumesnapshotcontent.name}
// - ${volumesnapshot.namespace}
//
// an error is returned in the following situations:
// - only one of name or namespace is provided
// - the name or namespace parameter contains a token that cannot be resolved
// - the resolved name is not a valid secret name
// - the resolved namespace is not a valid namespace name
func GetSecretReference(snapshotClassParams map[string]string, snapContentName string, snapshot *crdv1.VolumeSnapshot) (*v1.SecretReference, error) {
	nameTemplate, hasName := snapshotClassParams[snapshotterSecretNameKey]
	namespaceTemplate, hasNamespace := snapshotClassParams[snapshotterSecretNamespaceKey]

	if !hasName && !hasNamespace {
		return nil, nil
	}

	if len(nameTemplate) == 0 || len(namespaceTemplate) == 0 {
		return nil, fmt.Errorf("%s and %s parameters must be specified together", snapshotterSecretNameKey, snapshotterSecretNamespaceKey)
	}

	ref := &v1.SecretReference{}

	// Secret namespace template can make use of the VolumeSnapshotContent name or the VolumeSnapshot namespace.
	// Note that neither of those things are under the control of the VolumeSnapshot user.
	namespaceParams := map[string]string{"volumesnapshotcontent.name": snapContentName}
	// snapshot may be nil when resolving create/delete snapshot secret names because the
	// snapshot may or may not exist at delete time
	if snapshot != nil {
		namespaceParams["volumesnapshot.namespace"] = snapshot.Namespace
	}

	resolvedNamespace, err := resolveTemplate(namespaceTemplate, namespaceParams)
	if err != nil {
		return nil, fmt.Errorf("error resolving %s value %q: %v", snapshotterSecretNamespaceKey, namespaceTemplate, err)
	}
	glog.V(4).Infof("GetSecretReference namespaceTemplate %s, namespaceParams: %+v, resolved %s", namespaceTemplate, namespaceParams, resolvedNamespace)

	if len(validation.IsDNS1123Label(resolvedNamespace)) > 0 {
		if namespaceTemplate != resolvedNamespace {
			return nil, fmt.Errorf("%s parameter %q resolved to %q which is not a valid namespace name", snapshotterSecretNamespaceKey, namespaceTemplate, resolvedNamespace)
		}
		return nil, fmt.Errorf("%s parameter %q is not a valid namespace name", snapshotterSecretNamespaceKey, namespaceTemplate)
	}
	ref.Namespace = resolvedNamespace

	// Secret name template can make use of the VolumeSnapshotContent name, VolumeSnapshot name or namespace,
	// or a VolumeSnapshot annotation.
	// Note that VolumeSnapshot name and annotations are under the VolumeSnapshot user's control.
	nameParams := map[string]string{"volumesnapshotcontent.name": snapContentName}
	if snapshot != nil {
		nameParams["volumesnapshot.name"] = snapshot.Name
		nameParams["volumesnapshot.namespace"] = snapshot.Namespace
		for k, v := range snapshot.Annotations {
			nameParams["volumesnapshot.annotations['"+k+"']"] = v
		}
	}
	resolvedName, err := resolveTemplate(nameTemplate, nameParams)
	if err != nil {
		return nil, fmt.Errorf("error resolving %s value %q: %v", snapshotterSecretNameKey, nameTemplate, err)
	}
	if len(validation.IsDNS1123Subdomain(resolvedName)) > 0 {
		if nameTemplate != resolvedName {
			return nil, fmt.Errorf("%s parameter %q resolved to %q which is not a valid secret name", snapshotterSecretNameKey, nameTemplate, resolvedName)
		}
		return nil, fmt.Errorf("%s parameter %q is not a valid secret name", snapshotterSecretNameKey, nameTemplate)
	}
	ref.Name = resolvedName

	glog.V(4).Infof("GetSecretReference validated Secret: %+v", ref)
	return ref, nil
}

// resolveTemplate resolves the template by checking if the value is missing for a key
func resolveTemplate(template string, params map[string]string) (string, error) {
	missingParams := sets.NewString()
	resolved := os.Expand(template, func(k string) string {
		v, ok := params[k]
		if !ok {
			missingParams.Insert(k)
		}
		return v
	})
	if missingParams.Len() > 0 {
		return "", fmt.Errorf("invalid tokens: %q", missingParams.List())
	}
	return resolved, nil
}

// GetCredentials retrieves credentials stored in v1.SecretReference
func GetCredentials(k8s kubernetes.Interface, ref *v1.SecretReference) (map[string]string, error) {
	if ref == nil {
		return nil, nil
	}

	secret, err := k8s.CoreV1().Secrets(ref.Namespace).Get(ref.Name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("error getting secret %s in namespace %s: %v", ref.Name, ref.Namespace, err)
	}

	credentials := map[string]string{}
	for key, value := range secret.Data {
		credentials[key] = string(value)
	}
	return credentials, nil
}
