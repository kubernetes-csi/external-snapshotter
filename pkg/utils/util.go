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

package utils

import (
	"context"
	"fmt"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	klog "k8s.io/klog/v2"

	crdv1alpha1 "github.com/kubernetes-csi/external-snapshotter/client/v7/apis/volumegroupsnapshot/v1alpha1"
	crdv1 "github.com/kubernetes-csi/external-snapshotter/client/v7/apis/volumesnapshot/v1"
)

var keyFunc = cache.DeletionHandlingMetaNamespaceKeyFunc

type secretParamsMap struct {
	name               string
	secretNameKey      string
	secretNamespaceKey string
}

const (
	// CSI Parameters prefixed with csiParameterPrefix are not passed through
	// to the driver on CreateSnapshotRequest calls. Instead they are intended
	// to be used by the CSI external-snapshotter and maybe used to populate
	// fields in subsequent CSI calls or Kubernetes API objects. An exception
	// exists for the volume snapshot and volume snapshot content keys, which are
	// passed as parameters on CreateSnapshotRequest calls.
	csiParameterPrefix = "csi.storage.k8s.io/"

	PrefixedSnapshotterSecretNameKey      = csiParameterPrefix + "snapshotter-secret-name"      // Prefixed name key for DeleteSnapshot secret
	PrefixedSnapshotterSecretNamespaceKey = csiParameterPrefix + "snapshotter-secret-namespace" // Prefixed namespace key for DeleteSnapshot secret

	PrefixedGroupSnapshotterSecretNameKey      = csiParameterPrefix + "group-snapshotter-secret-name"      // Prefixed name key for CreateGroupSnapshot secret
	PrefixedGroupSnapshotterSecretNamespaceKey = csiParameterPrefix + "group-snapshotter-secret-namespace" // Prefixed namespace key for DeleteGroupSnapshot secret

	PrefixedSnapshotterListSecretNameKey      = csiParameterPrefix + "snapshotter-list-secret-name"      // Prefixed name key for ListSnapshots secret
	PrefixedSnapshotterListSecretNamespaceKey = csiParameterPrefix + "snapshotter-list-secret-namespace" // Prefixed namespace key for ListSnapshots secret

	PrefixedVolumeSnapshotNameKey        = csiParameterPrefix + "volumesnapshot/name"        // Prefixed VolumeSnapshot name key
	PrefixedVolumeSnapshotNamespaceKey   = csiParameterPrefix + "volumesnapshot/namespace"   // Prefixed VolumeSnapshot namespace key
	PrefixedVolumeSnapshotContentNameKey = csiParameterPrefix + "volumesnapshotcontent/name" // Prefixed VolumeSnapshotContent name key

	PrefixedVolumeGroupSnapshotNameKey        = csiParameterPrefix + "volumegroupsnapshot/name"        // Prefixed VolumeGroupSnapshot name key
	PrefixedVolumeGroupSnapshotNamespaceKey   = csiParameterPrefix + "volumegroupsnapshot/namespace"   // Prefixed VolumeGroupSnapshot namespace key
	PrefixedVolumeGroupSnapshotContentNameKey = csiParameterPrefix + "volumegroupsnapshotcontent/name" // Prefixed VolumeGroupSnapshotContent name key

	// Name of finalizer on VolumeSnapshotContents that are bound by VolumeSnapshots
	VolumeSnapshotContentFinalizer = "snapshot.storage.kubernetes.io/volumesnapshotcontent-bound-protection"
	// Name of finalizer on VolumeSnapshot that is being used as a source to create a PVC
	VolumeSnapshotBoundFinalizer = "snapshot.storage.kubernetes.io/volumesnapshot-bound-protection"
	// Name of finalizer on VolumeSnapshot that is used as a source to create a PVC
	VolumeSnapshotAsSourceFinalizer = "snapshot.storage.kubernetes.io/volumesnapshot-as-source-protection"
	// Name of finalizer on VolumeSnapshot that is a part of a VolumeGroupSnapshot
	VolumeSnapshotInGroupFinalizer = "snapshot.storage.kubernetes.io/volumesnapshot-in-group-protection"
	// Name of finalizer on PVCs that is being used as a source to create VolumeSnapshots
	PVCFinalizer = "snapshot.storage.kubernetes.io/pvc-as-source-protection"
	// Name of finalizer on VolumeGroupSnapshotContents that are bound by VolumeGroupSnapshots
	VolumeGroupSnapshotContentFinalizer = "groupsnapshot.storage.kubernetes.io/volumegroupsnapshotcontent-bound-protection"
	// Name of finalizer on VolumeGroupSnapshots that are bound to VolumeGroupSnapshotContents
	VolumeGroupSnapshotBoundFinalizer = "groupsnapshot.storage.kubernetes.io/volumegroupsnapshot-bound-protection"

	IsDefaultSnapshotClassAnnotation      = "snapshot.storage.kubernetes.io/is-default-class"
	IsDefaultGroupSnapshotClassAnnotation = "groupsnapshot.storage.kubernetes.io/is-default-class"

	// AnnVolumeSnapshotBeingDeleted annotation applies to VolumeSnapshotContents.
	// It indicates that the common snapshot controller has verified that volume
	// snapshot has a deletion timestamp and is being deleted.
	// Sidecar controller needs to check the deletion policy on the
	// VolumeSnapshotContentand and decide whether to delete the volume snapshot
	// backing the snapshot content.
	AnnVolumeSnapshotBeingDeleted = "snapshot.storage.kubernetes.io/volumesnapshot-being-deleted"

	// AnnVolumeSnapshotBeingCreated annotation applies to VolumeSnapshotContents.
	// If it is set, it indicates that the csi-snapshotter
	// sidecar has sent the create snapshot request to the storage system and
	// is waiting for a response of success or failure.
	// This annotation will be removed once the driver's CreateSnapshot
	// CSI function returns success or a final error (determined by isFinalError()).
	// If the create snapshot request fails with a non-final error such as timeout,
	// retry will happen and the annotation will remain.
	// This only applies to dynamic provisioning of snapshots because
	// the create snapshot CSI method will not be called for pre-provisioned
	// snapshots.
	AnnVolumeSnapshotBeingCreated = "snapshot.storage.kubernetes.io/volumesnapshot-being-created"

	// AnnVolumeGroupSnapshotBeingCreated annotation applies to VolumeGroupSnapshotContents.
	// If it is set, it indicates that the csi-snapshotter
	// sidecar has sent the create group snapshot request to the storage system and
	// is waiting for a response of success or failure.
	// This annotation will be removed once the driver's CreateGroupSnapshot
	// CSI function returns success or a final error (determined by isFinalError()).
	// If the create group snapshot request fails with a non-final error such as timeout,
	// retry will happen and the annotation will remain.
	// This only applies to dynamic provisioning of group snapshots because
	// the create group snapshot CSI method will not be called for pre-provisioned
	// group snapshots.
	AnnVolumeGroupSnapshotBeingCreated = "groupsnapshot.storage.kubernetes.io/volumegroupsnapshot-being-created"

	// AnnVolumeGroupSnapshotBeingDeleted annotation applies to VolumeGroupSnapshotContents.
	// It indicates that the common snapshot controller has verified that volume
	// group snapshot has a deletion timestamp and is being deleted.
	// Sidecar controller needs to check the deletion policy on the
	// VolumeGroupSnapshotContent and decide whether to delete the volume group snapshot
	// backing the group snapshot content.
	AnnVolumeGroupSnapshotBeingDeleted = "groupsnapshot.storage.kubernetes.io/volumegroupsnapshot-being-deleted"

	// Annotation for secret name and namespace will be added to the content
	// and used at snapshot content deletion time.
	AnnDeletionSecretRefName      = "snapshot.storage.kubernetes.io/deletion-secret-name"
	AnnDeletionSecretRefNamespace = "snapshot.storage.kubernetes.io/deletion-secret-namespace"

	// Annotation for secret name and namespace will be added to the group
	// snapshot content and used at group snapshot content deletion time.
	AnnDeletionGroupSecretRefName      = "groupsnapshot.storage.kubernetes.io/deletion-secret-name"
	AnnDeletionGroupSecretRefNamespace = "groupsnapshot.storage.kubernetes.io/deletion-secret-namespace"

	// VolumeSnapshotContentInvalidLabel is applied to invalid content as a label key. The value does not matter.
	// See https://github.com/kubernetes/enhancements/blob/master/keps/sig-storage/177-volume-snapshot/tighten-validation-webhook-crd.md#automatic-labelling-of-invalid-objects
	VolumeSnapshotContentInvalidLabel = "snapshot.storage.kubernetes.io/invalid-snapshot-content-resource"
	// VolumeSnapshotInvalidLabel is applied to invalid snapshot as a label key. The value does not matter.
	// See https://github.com/kubernetes/enhancements/blob/master/keps/sig-storage/177-volume-snapshot/tighten-validation-webhook-crd.md#automatic-labelling-of-invalid-objects
	VolumeSnapshotInvalidLabel = "snapshot.storage.kubernetes.io/invalid-snapshot-resource"
	// VolumeSnapshotContentManagedByLabel is applied by the snapshot controller to the VolumeSnapshotContent object in case distributed snapshotting is enabled.
	// The value contains the name of the node that handles the snapshot for the volume local to that node.
	VolumeSnapshotContentManagedByLabel = "snapshot.storage.kubernetes.io/managed-by"
)

var SnapshotterSecretParams = secretParamsMap{
	name:               "Snapshotter",
	secretNameKey:      PrefixedSnapshotterSecretNameKey,
	secretNamespaceKey: PrefixedSnapshotterSecretNamespaceKey,
}

var GroupSnapshotterSecretParams = secretParamsMap{
	name:               "GroupSnapshotter",
	secretNameKey:      PrefixedGroupSnapshotterSecretNameKey,
	secretNamespaceKey: PrefixedGroupSnapshotterSecretNamespaceKey,
}

var SnapshotterListSecretParams = secretParamsMap{
	name:               "SnapshotterList",
	secretNameKey:      PrefixedSnapshotterListSecretNameKey,
	secretNamespaceKey: PrefixedSnapshotterListSecretNamespaceKey,
}

// Annotations on VolumeSnapshotContent objects entirely controlled by csi-snapshotter
// Changes to these annotations will be ignored for determining whether to sync changes to content objects
// AnnVolumeSnapshotBeingCreated is managed entirely by the csi-snapshotter sidecar
// AnnVolumeSnapshotBeingDeleted is applied by the snapshot-controller and thus is not sidecar-owned
var sidecarControlledContentAnnotations = map[string]struct{}{
	AnnVolumeSnapshotBeingCreated: {},
}

// MapContainsKey checks if a given map of string to string contains the provided string.
func MapContainsKey(m map[string]string, s string) bool {
	_, r := m[s]
	return r
}

// RemoveString returns a newly created []string that contains all items from slice that
// are not equal to s.
func RemoveString(slice []string, s string) []string {
	newSlice := make([]string, 0)
	for _, item := range slice {
		if item == s {
			continue
		}
		newSlice = append(newSlice, item)
	}
	if len(newSlice) == 0 {
		// Sanitize for unit tests so we don't need to distinguish empty array
		// and nil.
		newSlice = nil
	}
	return newSlice
}

func SnapshotKey(vs *crdv1.VolumeSnapshot) string {
	return fmt.Sprintf("%s/%s", vs.Namespace, vs.Name)
}

func SnapshotRefKey(vsref *v1.ObjectReference) string {
	return fmt.Sprintf("%s/%s", vsref.Namespace, vsref.Name)
}

// storeObjectUpdate updates given cache with a new object version from Informer
// callback (i.e. with events from etcd) or with an object modified by the
// controller itself. Returns "true", if the cache was updated, false if the
// object is an old version and should be ignored.
func StoreObjectUpdate(store cache.Store, obj interface{}, className string) (bool, error) {
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
		klog.V(4).Infof("storeObjectUpdate: adding %s %q, version %s", className, objName, objAccessor.GetResourceVersion())
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
		klog.V(4).Infof("storeObjectUpdate: ignoring %s %q version %s", className, objName, objAccessor.GetResourceVersion())
		return false, nil
	}

	klog.V(4).Infof("storeObjectUpdate updating %s %q with version %s", className, objName, objAccessor.GetResourceVersion())
	if err = store.Update(obj); err != nil {
		return false, fmt.Errorf("error updating %s %q in controller cache: %v", className, objName, err)
	}
	return true, nil
}

// GetDynamicSnapshotContentNameForSnapshot returns a unique content name for the
// passed in VolumeSnapshot to dynamically provision a snapshot.
func GetDynamicSnapshotContentNameForSnapshot(snapshot *crdv1.VolumeSnapshot) string {
	return "snapcontent-" + string(snapshot.UID)
}

// IsDefaultAnnotation returns a boolean if
// the annotation is set
func IsDefaultAnnotation(tm metav1.TypeMeta, obj metav1.ObjectMeta) bool {
	switch tm.Kind {
	case "VolumeSnapshotClass":
		if obj.Annotations[IsDefaultSnapshotClassAnnotation] == "true" {
			return true
		}
	case "VolumeGroupSnapshotClass":
		if obj.Annotations[IsDefaultGroupSnapshotClassAnnotation] == "true" {
			return true
		}
	}

	return false
}

// verifyAndGetSecretNameAndNamespaceTemplate gets the values (templates) associated
// with the parameters specified in "secret" and verifies that they are specified correctly.
func verifyAndGetSecretNameAndNamespaceTemplate(secret secretParamsMap, snapshotClassParams map[string]string) (nameTemplate, namespaceTemplate string, err error) {
	numName := 0
	numNamespace := 0
	if t, ok := snapshotClassParams[secret.secretNameKey]; ok {
		nameTemplate = t
		numName++
	}
	if t, ok := snapshotClassParams[secret.secretNamespaceKey]; ok {
		namespaceTemplate = t
		numNamespace++
	}

	if numName != numNamespace {
		// Not both 0 or both 1
		return "", "", fmt.Errorf("either name and namespace for %s secrets specified, Both must be specified", secret.name)
	} else if numName == 1 {
		// Case where we've found a name and a namespace template
		if nameTemplate == "" || namespaceTemplate == "" {
			return "", "", fmt.Errorf("%s secrets specified in parameters but value of either namespace or name is empty", secret.name)
		}
		return nameTemplate, namespaceTemplate, nil
	} else if numName == 0 {
		// No secrets specified
		return "", "", nil
	}
	// THIS IS NOT A VALID CASE
	return "", "", fmt.Errorf("unknown error with getting secret name and namespace templates")
}

// getSecretReference returns a reference to the secret specified in the given nameTemplate
// and namespaceTemplate, or an error if the templates are not specified correctly.
// No lookup of the referenced secret is performed, and the secret may or may not exist.
//
// supported tokens for name resolution:
// - ${volumesnapshotcontent.name}
// - ${volumesnapshot.namespace}
// - ${volumesnapshot.name}
//
// supported tokens for namespace resolution:
// - ${volumesnapshotcontent.name}
// - ${volumesnapshot.namespace}
//
// an error is returned in the following situations:
// - the nameTemplate or namespaceTemplate contains a token that cannot be resolved
// - the resolved name is not a valid secret name
// - the resolved namespace is not a valid namespace name
func GetSecretReference(secretParams secretParamsMap, snapshotClassParams map[string]string, snapContentName string, snapshot *crdv1.VolumeSnapshot) (*v1.SecretReference, error) {
	nameTemplate, namespaceTemplate, err := verifyAndGetSecretNameAndNamespaceTemplate(secretParams, snapshotClassParams)
	if err != nil {
		return nil, fmt.Errorf("failed to get name and namespace template from params: %v", err)
	}

	if nameTemplate == "" && namespaceTemplate == "" {
		return nil, nil
	}

	ref := &v1.SecretReference{}

	// Secret namespace template can make use of the VolumeSnapshotContent name, VolumeSnapshot name or namespace.
	// Note that neither of those things are under the control of the VolumeSnapshot user.
	namespaceParams := map[string]string{"volumesnapshotcontent.name": snapContentName}
	// snapshot may be nil when resolving create/delete snapshot secret names because the
	// snapshot may or may not exist at delete time
	if snapshot != nil {
		namespaceParams["volumesnapshot.namespace"] = snapshot.Namespace
	}

	resolvedNamespace, err := resolveTemplate(namespaceTemplate, namespaceParams)
	if err != nil {
		return nil, fmt.Errorf("error resolving value %q: %v", namespaceTemplate, err)
	}
	klog.V(4).Infof("GetSecretReference namespaceTemplate %s, namespaceParams: %+v, resolved %s", namespaceTemplate, namespaceParams, resolvedNamespace)

	if len(validation.IsDNS1123Label(resolvedNamespace)) > 0 {
		if namespaceTemplate != resolvedNamespace {
			return nil, fmt.Errorf("%q resolved to %q which is not a valid namespace name", namespaceTemplate, resolvedNamespace)
		}
		return nil, fmt.Errorf("%q is not a valid namespace name", namespaceTemplate)
	}
	ref.Namespace = resolvedNamespace

	// Secret name template can make use of the VolumeSnapshotContent name, VolumeSnapshot name or namespace.
	// Note that VolumeSnapshot name and namespace are under the VolumeSnapshot user's control.
	nameParams := map[string]string{"volumesnapshotcontent.name": snapContentName}
	if snapshot != nil {
		nameParams["volumesnapshot.name"] = snapshot.Name
		nameParams["volumesnapshot.namespace"] = snapshot.Namespace
	}
	resolvedName, err := resolveTemplate(nameTemplate, nameParams)
	if err != nil {
		return nil, fmt.Errorf("error resolving value %q: %v", nameTemplate, err)
	}
	if len(validation.IsDNS1123Subdomain(resolvedName)) > 0 {
		if nameTemplate != resolvedName {
			return nil, fmt.Errorf("%q resolved to %q which is not a valid secret name", nameTemplate, resolvedName)
		}
		return nil, fmt.Errorf("%q is not a valid secret name", nameTemplate)
	}
	ref.Name = resolvedName

	klog.V(4).Infof("GetSecretReference validated Secret: %+v", ref)
	return ref, nil
}

// GetSecretReference for the group snapshot
func GetGroupSnapshotSecretReference(secretParams secretParamsMap, volumeGroupSnapshotClassParams map[string]string, groupSnapContentName string, volumeGroupSnapshot *crdv1alpha1.VolumeGroupSnapshot) (*v1.SecretReference, error) {
	nameTemplate, namespaceTemplate, err := verifyAndGetSecretNameAndNamespaceTemplate(secretParams, volumeGroupSnapshotClassParams)
	if err != nil {
		return nil, fmt.Errorf("failed to get name and namespace template from params: %v", err)
	}
	if nameTemplate == "" && namespaceTemplate == "" {
		return nil, nil
	}

	ref := &v1.SecretReference{}

	// Secret namespace template can make use of the VolumeGroupSnapshotContent name, VolumeGroupSnapshot name or namespace.
	// Note that neither of those things are under the control of the VolumeGroupSnapshot user.
	namespaceParams := map[string]string{"volumegroupsnapshotcontent.name": groupSnapContentName}
	// volume group snapshot may be nil when resolving create/delete volumegroupsnapshot secret names because the
	// volume group snapshot may or may not exist at delete time
	if volumeGroupSnapshot != nil {
		namespaceParams["volumegroupsnapshot.namespace"] = volumeGroupSnapshot.Namespace
	}

	resolvedNamespace, err := resolveTemplate(namespaceTemplate, namespaceParams)
	if err != nil {
		return nil, fmt.Errorf("error resolving value %q: %v", namespaceTemplate, err)
	}

	if len(validation.IsDNS1123Label(resolvedNamespace)) > 0 {
		if namespaceTemplate != resolvedNamespace {
			return nil, fmt.Errorf("%q resolved to %q which is not a valid namespace name", namespaceTemplate, resolvedNamespace)
		}
		return nil, fmt.Errorf("%q is not a valid namespace name", namespaceTemplate)
	}
	ref.Namespace = resolvedNamespace

	// Secret name template can make use of the VolumeGroupSnapshotContent name, VolumeGroupSnapshot name or namespace.
	// Note that VolumeGroupSnapshot name and namespace are under the VolumeGroupSnapshot user's control.
	nameParams := map[string]string{"volumegroupsnapshotcontent.name": groupSnapContentName}
	if volumeGroupSnapshot != nil {
		nameParams["volumegroupsnapshot.name"] = volumeGroupSnapshot.Name
		nameParams["volumegroupsnapshot.namespace"] = volumeGroupSnapshot.Namespace
	}
	resolvedName, err := resolveTemplate(nameTemplate, nameParams)
	if err != nil {
		return nil, fmt.Errorf("error resolving value %q: %v", nameTemplate, err)
	}
	if len(validation.IsDNS1123Subdomain(resolvedName)) > 0 {
		if nameTemplate != resolvedName {
			return nil, fmt.Errorf("%q resolved to %q which is not a valid secret name", nameTemplate, resolvedName)
		}
		return nil, fmt.Errorf("%q is not a valid secret name", nameTemplate)
	}
	ref.Name = resolvedName
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

	secret, err := k8s.CoreV1().Secrets(ref.Namespace).Get(context.TODO(), ref.Name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("error getting secret %s in namespace %s: %v", ref.Name, ref.Namespace, err)
	}

	credentials := map[string]string{}
	for key, value := range secret.Data {
		credentials[key] = string(value)
	}
	return credentials, nil
}

// NoResyncPeriodFunc Returns 0 for resyncPeriod in case resyncing is not needed.
func NoResyncPeriodFunc() time.Duration {
	return 0
}

// NeedToAddContentFinalizer checks if a Finalizer needs to be added for the volume snapshot content.
func NeedToAddContentFinalizer(content *crdv1.VolumeSnapshotContent) bool {
	return content.ObjectMeta.DeletionTimestamp == nil && !slices.Contains(content.ObjectMeta.Finalizers, VolumeSnapshotContentFinalizer)
}

// NeedToAddGroupSnapshotContentFinalizer checks if a Finalizer needs to be added for the volume group snapshot content.
func NeedToAddGroupSnapshotContentFinalizer(groupSnapshotContent *crdv1alpha1.VolumeGroupSnapshotContent) bool {
	return groupSnapshotContent.ObjectMeta.DeletionTimestamp == nil && !slices.Contains(groupSnapshotContent.ObjectMeta.Finalizers, VolumeGroupSnapshotContentFinalizer)
}

// IsSnapshotDeletionCandidate checks if a volume snapshot deletionTimestamp
// is set and any finalizer is on the snapshot.
func IsSnapshotDeletionCandidate(snapshot *crdv1.VolumeSnapshot) bool {
	return snapshot.ObjectMeta.DeletionTimestamp != nil && (slices.Contains(snapshot.ObjectMeta.Finalizers, VolumeSnapshotAsSourceFinalizer) || slices.Contains(snapshot.ObjectMeta.Finalizers, VolumeSnapshotBoundFinalizer))
}

// IsGroupSnapshotDeletionCandidate checks if a volume group snapshot deletionTimestamp
// is set and any finalizer is on the group snapshot.
func IsGroupSnapshotDeletionCandidate(groupSnapshot *crdv1alpha1.VolumeGroupSnapshot) bool {
	return groupSnapshot.ObjectMeta.DeletionTimestamp != nil && slices.Contains(groupSnapshot.ObjectMeta.Finalizers, VolumeGroupSnapshotBoundFinalizer)
}

// NeedToAddSnapshotAsSourceFinalizer checks if a Finalizer needs to be added for the volume snapshot as a source for PVC.
func NeedToAddSnapshotAsSourceFinalizer(snapshot *crdv1.VolumeSnapshot) bool {
	return snapshot.ObjectMeta.DeletionTimestamp == nil && !slices.Contains(snapshot.ObjectMeta.Finalizers, VolumeSnapshotAsSourceFinalizer)
}

// NeedToAddSnapshotBoundFinalizer checks if a Finalizer needs to be added for the bound volume snapshot.
func NeedToAddSnapshotBoundFinalizer(snapshot *crdv1.VolumeSnapshot) bool {
	return snapshot.ObjectMeta.DeletionTimestamp == nil && !slices.Contains(snapshot.ObjectMeta.Finalizers, VolumeSnapshotBoundFinalizer) && IsBoundVolumeSnapshotContentNameSet(snapshot)
}

// NeedToAddGroupSnapshotBoundFinalizer checks if a Finalizer needs to be added for the bound volume group snapshot.
func NeedToAddGroupSnapshotBoundFinalizer(groupSnapshot *crdv1alpha1.VolumeGroupSnapshot) bool {
	return groupSnapshot.ObjectMeta.DeletionTimestamp == nil && !slices.Contains(groupSnapshot.ObjectMeta.Finalizers, VolumeGroupSnapshotBoundFinalizer) && IsBoundVolumeGroupSnapshotContentNameSet(groupSnapshot)
}

func deprecationWarning(deprecatedParam, newParam, removalVersion string) string {
	if removalVersion == "" {
		removalVersion = "a future release"
	}
	newParamPhrase := ""
	if len(newParam) != 0 {
		newParamPhrase = fmt.Sprintf(", please use \"%s\" instead", newParam)
	}
	return fmt.Sprintf("\"%s\" is deprecated and will be removed in %s%s", deprecatedParam, removalVersion, newParamPhrase)
}

func RemovePrefixedParameters(param map[string]string) (map[string]string, error) {
	newParam := map[string]string{}
	for k, v := range param {
		if strings.HasPrefix(k, csiParameterPrefix) {
			// Check if its well known
			switch k {
			case PrefixedSnapshotterSecretNameKey:
			case PrefixedSnapshotterSecretNamespaceKey:
			case PrefixedSnapshotterListSecretNameKey:
			case PrefixedSnapshotterListSecretNamespaceKey:
			case PrefixedGroupSnapshotterSecretNameKey:
			case PrefixedGroupSnapshotterSecretNamespaceKey:
			default:
				return map[string]string{}, fmt.Errorf("found unknown parameter key \"%s\" with reserved namespace %s", k, csiParameterPrefix)
			}
		} else {
			// Don't strip, add this key-value to new map
			// Deprecated parameters prefixed with "csi" are not stripped to preserve backwards compatibility
			newParam[k] = v
		}
	}
	return newParam, nil
}

// Stateless functions
func GetSnapshotStatusForLogging(snapshot *crdv1.VolumeSnapshot) string {
	snapshotContentName := ""
	if snapshot.Status != nil && snapshot.Status.BoundVolumeSnapshotContentName != nil {
		snapshotContentName = *snapshot.Status.BoundVolumeSnapshotContentName
	}
	ready := false
	if snapshot.Status != nil && snapshot.Status.ReadyToUse != nil {
		ready = *snapshot.Status.ReadyToUse
	}
	return fmt.Sprintf("bound to: %q, Completed: %v", snapshotContentName, ready)
}

func GetGroupSnapshotStatusForLogging(groupSnapshot *crdv1alpha1.VolumeGroupSnapshot) string {
	groupSnapshotContentName := ""
	if groupSnapshot.Status != nil && groupSnapshot.Status.BoundVolumeGroupSnapshotContentName != nil {
		groupSnapshotContentName = *groupSnapshot.Status.BoundVolumeGroupSnapshotContentName
	}
	ready := false
	if groupSnapshot.Status != nil && groupSnapshot.Status.ReadyToUse != nil {
		ready = *groupSnapshot.Status.ReadyToUse
	}
	return fmt.Sprintf("bound to: %q, Completed: %v", groupSnapshotContentName, ready)
}

func IsVolumeSnapshotRefSet(snapshot *crdv1.VolumeSnapshot, content *crdv1.VolumeSnapshotContent) bool {
	if content.Spec.VolumeSnapshotRef.Name == snapshot.Name &&
		content.Spec.VolumeSnapshotRef.Namespace == snapshot.Namespace &&
		content.Spec.VolumeSnapshotRef.UID == snapshot.UID {
		return true
	}
	return false
}

func IsBoundVolumeSnapshotContentNameSet(snapshot *crdv1.VolumeSnapshot) bool {
	if snapshot.Status == nil || snapshot.Status.BoundVolumeSnapshotContentName == nil || *snapshot.Status.BoundVolumeSnapshotContentName == "" {
		return false
	}
	return true
}

func IsSnapshotReady(snapshot *crdv1.VolumeSnapshot) bool {
	if snapshot.Status == nil || snapshot.Status.ReadyToUse == nil || *snapshot.Status.ReadyToUse == false {
		return false
	}
	return true
}

// IsSnapshotCreated indicates that the snapshot has been cut on a storage system
func IsSnapshotCreated(snapshot *crdv1.VolumeSnapshot) bool {
	return snapshot.Status != nil && snapshot.Status.CreationTime != nil
}

func GroupSnapshotKey(vgs *crdv1alpha1.VolumeGroupSnapshot) string {
	return fmt.Sprintf("%s/%s", vgs.Namespace, vgs.Name)
}

func GroupSnapshotRefKey(vgsref *v1.ObjectReference) string {
	return fmt.Sprintf("%s/%s", vgsref.Namespace, vgsref.Name)
}

func IsGroupSnapshotReady(groupSnapshot *crdv1alpha1.VolumeGroupSnapshot) bool {
	if groupSnapshot.Status == nil || groupSnapshot.Status.ReadyToUse == nil || *groupSnapshot.Status.ReadyToUse == false {
		return false
	}
	return true
}

func IsBoundVolumeGroupSnapshotContentNameSet(groupSnapshot *crdv1alpha1.VolumeGroupSnapshot) bool {
	if groupSnapshot.Status == nil || groupSnapshot.Status.BoundVolumeGroupSnapshotContentName == nil || *groupSnapshot.Status.BoundVolumeGroupSnapshotContentName == "" {
		return false
	}
	return true
}

func IsPVCVolumeSnapshotRefListSet(groupSnapshot *crdv1alpha1.VolumeGroupSnapshot) bool {
	if groupSnapshot.Status == nil || len(groupSnapshot.Status.PVCVolumeSnapshotRefList) == 0 {
		return false
	}
	return true
}

func IsVolumeGroupSnapshotRefSet(groupSnapshot *crdv1alpha1.VolumeGroupSnapshot, content *crdv1alpha1.VolumeGroupSnapshotContent) bool {
	if content.Spec.VolumeGroupSnapshotRef.Name == groupSnapshot.Name &&
		content.Spec.VolumeGroupSnapshotRef.Namespace == groupSnapshot.Namespace &&
		content.Spec.VolumeGroupSnapshotRef.UID == groupSnapshot.UID {
		return true
	}
	return false
}

// IsGroupSnapshotCreated indicates that the group snapshot has been cut on a storage system
func IsGroupSnapshotCreated(groupSnapshot *crdv1alpha1.VolumeGroupSnapshot) bool {
	return groupSnapshot.Status != nil && groupSnapshot.Status.CreationTime != nil
}

// GetDynamicSnapshotContentNameFoGrouprSnapshot returns a unique content name for the
// passed in VolumeGroupSnapshot to dynamically provision a group snapshot.
func GetDynamicSnapshotContentNameForGroupSnapshot(groupSnapshot *crdv1alpha1.VolumeGroupSnapshot) string {
	return "groupsnapcontent-" + string(groupSnapshot.UID)
}

// ShouldEnqueueContentChange indicated whether or not a change to a VolumeSnapshotContent object
// is a change that should be enqueued for sync
//
// The following changes are sanitized (and thus, not considered for determining whether to sync)
//   - Resource Version (always changed between objects)
//   - Status (owned and updated only by the sidecar)
//   - Managed Fields (updated by sidecar, and will not change the sync status)
//   - Finalizers (updated by sidecar, and will not change the sync status)
//   - Sidecar-Owned Annotations (annotations that are owned and updated only by the sidecar)
//     (some annotations, such as AnnVolumeSnapshotBeingDeleted, are applied by the controller - so
//     only annotatinons entirely controlled by the sidecar are ignored)
//
// If the VolumeSnapshotContent object still contains other changes after this sanitization, the changes
// are potentially meaningful and the object is enqueued to be considered for syncing
func ShouldEnqueueContentChange(old *crdv1.VolumeSnapshotContent, new *crdv1.VolumeSnapshotContent) bool {
	sanitized := new.DeepCopy()
	// ResourceVersion always changes between revisions
	sanitized.ResourceVersion = old.ResourceVersion
	// Fields that should not result in a sync
	sanitized.Status = old.Status
	sanitized.ManagedFields = old.ManagedFields
	sanitized.Finalizers = old.Finalizers
	// Annotations should cause a sync, except for annotations that csi-snapshotter controls
	if old.Annotations != nil {
		// This can happen if the new version has all annotations removed
		if sanitized.Annotations == nil {
			sanitized.Annotations = map[string]string{}
		}
		for annotation, _ := range sidecarControlledContentAnnotations {
			if value, ok := old.Annotations[annotation]; ok {
				sanitized.Annotations[annotation] = value
			} else {
				delete(sanitized.Annotations, annotation)
			}
		}
	} else {
		// Old content has no annotations, so delete any sidecar-controlled annotations present on the new content
		for annotation, _ := range sidecarControlledContentAnnotations {
			delete(sanitized.Annotations, annotation)
		}
	}

	if equality.Semantic.DeepEqual(old, sanitized) {
		// The only changes are in the fields we don't care about, so don't enqueue for sync
		return false
	}
	return true
}
