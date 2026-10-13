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

// Package topologywebhook applies snapshot topology at pod admission and
// validates PVC node selections (KEP-5943).
package topologywebhook

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"slices"
	"strings"

	admissionv1 "k8s.io/api/admission/v1"
	v1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	corelisters "k8s.io/client-go/listers/core/v1"
	"k8s.io/klog/v2"

	"github.com/kubernetes-csi/external-snapshotter/v8/pkg/topology"
)

// Webhook serves the pod mutating and PVC validating webhooks.
type Webhook struct {
	Resolver         *topology.Resolver
	Nodes            corelisters.NodeLister
	MaxAffinityTerms int
	// Enabled reports whether the VolumeSnapshotTopology feature gate is on.
	Enabled func() bool
	// Synced reports whether every informer the webhooks read has synced.
	Synced func() bool
}

// Handler serves admission endpoints and /readyz.
func (wh *Webhook) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/mutate-pods", func(w http.ResponseWriter, r *http.Request) { serve(w, r, wh.MutatePod) })
	mux.HandleFunc("/validate-pvcs", func(w http.ResponseWriter, r *http.Request) { serve(w, r, wh.ValidatePVC) })
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, r *http.Request) {
		// Missing cache entries can silently skip PVC topology validation.
		if !wh.Synced() {
			http.Error(w, "informer caches not synced", http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte("ok"))
	})
	return mux
}

type admitFunc func(context.Context, *admissionv1.AdmissionRequest) *admissionv1.AdmissionResponse

func serve(w http.ResponseWriter, r *http.Request, admit admitFunc) {
	contentType := r.Header.Get("Content-Type")
	if mediaType, _, err := mime.ParseMediaType(contentType); err != nil || mediaType != "application/json" {
		http.Error(w, fmt.Sprintf("invalid Content-Type %q, expected application/json", contentType), http.StatusUnsupportedMediaType)
		return
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, fmt.Sprintf("reading request body: %v", err), http.StatusBadRequest)
		return
	}
	review := &admissionv1.AdmissionReview{}
	if err := json.Unmarshal(body, review); err != nil || review.Request == nil {
		http.Error(w, fmt.Sprintf("invalid AdmissionReview: %v", err), http.StatusBadRequest)
		return
	}

	response := admit(r.Context(), review.Request)
	response.UID = review.Request.UID
	out := &admissionv1.AdmissionReview{TypeMeta: review.TypeMeta, Response: response}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(out); err != nil {
		klog.Errorf("Failed to write admission response: %v", err)
	}
}

func allowed() *admissionv1.AdmissionResponse {
	return &admissionv1.AdmissionResponse{Allowed: true}
}

// failurePolicy does not override denials; fail open explicitly on internal errors.
func errored(err error) *admissionv1.AdmissionResponse {
	klog.Errorf("Snapshot topology webhook: %v", err)
	return &admissionv1.AdmissionResponse{
		Allowed:  true,
		Warnings: []string{fmt.Sprintf("snapshot topology was not checked: %v", err)},
	}
}

// MutatePod handles CREATE, merging resolved topology or gating the pod until ready.
func (wh *Webhook) MutatePod(ctx context.Context, req *admissionv1.AdmissionRequest) *admissionv1.AdmissionResponse {
	if !wh.Enabled() {
		return allowed()
	}
	pod := &v1.Pod{}
	if err := json.Unmarshal(req.Object.Raw, pod); err != nil {
		return errored(fmt.Errorf("decoding pod: %w", err))
	}
	if pod.Namespace == "" {
		pod.Namespace = req.Namespace
	}

	ops, warning := wh.podPatch(ctx, pod)
	response := allowed()
	if warning != "" {
		klog.Warningf("Pod %s/%s (generateName %q): %s", pod.Namespace, pod.Name, pod.GenerateName, warning)
		response.Warnings = []string{warning}
	}
	if len(ops) > 0 {
		patch, err := json.Marshal(ops)
		if err != nil {
			return errored(fmt.Errorf("encoding patch: %w", err))
		}
		patchType := admissionv1.PatchTypeJSONPatch
		response.Patch = patch
		response.PatchType = &patchType
	}
	return response
}

type patchOp struct {
	Op    string `json:"op"`
	Path  string `json:"path"`
	Value any    `json:"value,omitempty"`
}

// podPatch returns a patch and an optional admission warning.
func (wh *Webhook) podPatch(ctx context.Context, pod *v1.Pod) ([]patchOp, string) {
	// The API server rejects nodeName combined with scheduling gates.
	if pod.Spec.NodeName != "" {
		return nil, ""
	}
	if HasGate(pod) {
		// The controller cannot find gated pods without the managed label.
		if pod.Labels[topology.ManagedLabel] != "true" {
			return []patchOp{labelOp(pod)}, ""
		}
		return nil, ""
	}
	topo := wh.Resolver.ResolvePod(ctx, pod)
	if !topo.Relevant {
		return nil, ""
	}
	if topo.Pending != "" {
		return gatePatch(pod), ""
	}

	var existing *v1.NodeAffinity
	if pod.Spec.Affinity != nil {
		existing = pod.Spec.Affinity.NodeAffinity
	}
	merged, err := topology.MergeAtCreate(existing, topo.TermSets, wh.MaxAffinityTerms)
	switch {
	case errors.Is(err, topology.ErrTooManyTerms):
		return nil, fmt.Sprintf("snapshot topology was not applied to the pod's node affinity: it would need more than %d node selector terms", wh.MaxAffinityTerms)
	case errors.Is(err, topology.ErrUnsatisfiable):
		return nil, "snapshot topology was not applied to the pod's node affinity: it matches no node, so no node can run this pod"
	case err != nil:
		return nil, fmt.Sprintf("snapshot topology was not applied to the pod's node affinity: %v", err)
	}

	if apiequality.Semantic.DeepEqual(existing, merged) {
		return nil, ""
	}
	return []patchOp{affinityOp(pod, merged)}, ""
}

// HasGate reports whether pod carries the snapshot topology scheduling gate.
func HasGate(pod *v1.Pod) bool {
	return slices.ContainsFunc(pod.Spec.SchedulingGates, func(g v1.PodSchedulingGate) bool {
		return g.Name == topology.SchedulingGate
	})
}

func gatePatch(pod *v1.Pod) []patchOp {
	gate := v1.PodSchedulingGate{Name: topology.SchedulingGate}
	gateOp := patchOp{Op: "add", Path: "/spec/schedulingGates/-", Value: gate}
	if len(pod.Spec.SchedulingGates) == 0 {
		gateOp = patchOp{Op: "add", Path: "/spec/schedulingGates", Value: []v1.PodSchedulingGate{gate}}
	}
	return []patchOp{labelOp(pod), gateOp}
}

func labelOp(pod *v1.Pod) patchOp {
	if pod.Labels == nil {
		return patchOp{Op: "add", Path: "/metadata/labels", Value: map[string]string{topology.ManagedLabel: "true"}}
	}
	return patchOp{Op: "add", Path: "/metadata/labels/" + escape(topology.ManagedLabel), Value: "true"}
}

func affinityOp(pod *v1.Pod, na *v1.NodeAffinity) patchOp {
	if pod.Spec.Affinity == nil {
		return patchOp{Op: "add", Path: "/spec/affinity", Value: &v1.Affinity{NodeAffinity: na}}
	}
	return patchOp{Op: "add", Path: "/spec/affinity/nodeAffinity", Value: na}
}

// escape encodes a JSON pointer reference token (RFC 6901).
func escape(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(s, "~", "~0"), "/", "~1")
}

// ValidatePVC rejects UPDATEs selecting a node outside the snapshot topology.
// Unresolved topology is allowed.
func (wh *Webhook) ValidatePVC(_ context.Context, req *admissionv1.AdmissionRequest) *admissionv1.AdmissionResponse {
	if !wh.Enabled() {
		return allowed()
	}
	pvc, oldPVC := &v1.PersistentVolumeClaim{}, &v1.PersistentVolumeClaim{}
	if err := json.Unmarshal(req.Object.Raw, pvc); err != nil {
		return errored(fmt.Errorf("decoding PersistentVolumeClaim: %w", err))
	}
	if err := json.Unmarshal(req.OldObject.Raw, oldPVC); err != nil {
		return errored(fmt.Errorf("decoding old PersistentVolumeClaim: %w", err))
	}

	nodeName := pvc.Annotations[topology.SelectedNodeAnnotation]
	if nodeName == "" || nodeName == oldPVC.Annotations[topology.SelectedNodeAnnotation] {
		return allowed()
	}
	content, terms, ok := wh.Resolver.PVCTopology(pvc)
	if !ok {
		return allowed()
	}
	node, err := wh.Nodes.Get(nodeName)
	if err != nil {
		klog.V(4).Infof("Allowing node %q for PersistentVolumeClaim %s/%s: %v", nodeName, pvc.Namespace, pvc.Name, err)
		return allowed()
	}
	if topology.NodeMatches(node.Labels, terms) {
		return allowed()
	}
	msg := fmt.Sprintf("node %q does not satisfy the nodeAffinity of VolumeSnapshotContent %q that PersistentVolumeClaim %s/%s is restored from",
		nodeName, content, pvc.Namespace, pvc.Name)
	klog.V(2).Info(msg)
	return &admissionv1.AdmissionResponse{Result: &metav1.Status{
		Status:  metav1.StatusFailure,
		Code:    http.StatusForbidden,
		Reason:  metav1.StatusReasonForbidden,
		Message: msg,
	}}
}
