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

package topologywebhook

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	jsonpatch "github.com/evanphx/json-patch"
	snapv1 "github.com/kubernetes-csi/external-snapshotter/client/v8/apis/volumesnapshot/v1"
	snapshotlisters "github.com/kubernetes-csi/external-snapshotter/client/v8/listers/volumesnapshot/v1"
	admissionv1 "k8s.io/api/admission/v1"
	v1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	corelisters "k8s.io/client-go/listers/core/v1"
	storagelisters "k8s.io/client-go/listers/storage/v1"
	"k8s.io/client-go/tools/cache"

	"github.com/kubernetes-csi/external-snapshotter/v8/pkg/topology"
)

const zoneKey = "topology.kubernetes.io/zone"

type fixture struct {
	pvcs     []*v1.PersistentVolumeClaim
	scs      []*storagev1.StorageClass
	snaps    []*snapv1.VolumeSnapshot
	contents []*snapv1.VolumeSnapshotContent
	nodes    []*v1.Node
	disabled bool
	maxTerms int
}

func newIndexer[T runtime.Object](objs ...T) cache.Indexer {
	idx := cache.NewIndexer(cache.MetaNamespaceKeyFunc, cache.Indexers{cache.NamespaceIndex: cache.MetaNamespaceIndexFunc})
	for _, o := range objs {
		_ = idx.Add(o)
	}
	return idx
}

func (f fixture) webhook() *Webhook {
	maxTerms := f.maxTerms
	if maxTerms == 0 {
		maxTerms = 32
	}
	return &Webhook{
		Resolver: &topology.Resolver{
			PVCs:           corelisters.NewPersistentVolumeClaimLister(newIndexer(f.pvcs...)),
			StorageClasses: storagelisters.NewStorageClassLister(newIndexer(f.scs...)),
			Snapshots:      snapshotlisters.NewVolumeSnapshotLister(newIndexer(f.snaps...)),
			Contents:       snapshotlisters.NewVolumeSnapshotContentLister(newIndexer(f.contents...)),
		},
		Nodes:            corelisters.NewNodeLister(newIndexer(f.nodes...)),
		MaxAffinityTerms: maxTerms,
		Enabled:          func() bool { return !f.disabled },
		Synced:           func() bool { return true },
	}
}

func ptr[T any](v T) *T { return &v }

func wffcClass() *storagev1.StorageClass {
	mode := storagev1.VolumeBindingWaitForFirstConsumer
	return &storagev1.StorageClass{ObjectMeta: metav1.ObjectMeta{Name: "wffc"}, VolumeBindingMode: &mode}
}

func immediateClass() *storagev1.StorageClass {
	mode := storagev1.VolumeBindingImmediate
	return &storagev1.StorageClass{ObjectMeta: metav1.ObjectMeta{Name: "immediate"}, VolumeBindingMode: &mode}
}

func snapshotPVC(name, snapName string) *v1.PersistentVolumeClaim {
	return &v1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: name},
		Spec: v1.PersistentVolumeClaimSpec{
			StorageClassName: ptr("wffc"),
			DataSource:       &v1.TypedLocalObjectReference{APIGroup: ptr(topology.SnapshotAPIGroup), Kind: "VolumeSnapshot", Name: snapName},
		},
	}
}

func boundSnapshot(name, contentName string) *snapv1.VolumeSnapshot {
	return &snapv1.VolumeSnapshot{
		ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: name, UID: types.UID("uid-" + name)},
		Status:     &snapv1.VolumeSnapshotStatus{BoundVolumeSnapshotContentName: ptr(contentName)},
	}
}

// content returns a ready content bound back to snapshot snapName in the
// default namespace.
func content(name, snapName string, terms ...v1.TopologySelectorTerm) *snapv1.VolumeSnapshotContent {
	return &snapv1.VolumeSnapshotContent{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec: snapv1.VolumeSnapshotContentSpec{
			VolumeSnapshotRef: v1.ObjectReference{Namespace: "default", Name: snapName, UID: types.UID("uid-" + snapName)},
			NodeAffinity:      terms,
		},
		Status: &snapv1.VolumeSnapshotContentStatus{ReadyToUse: ptr(true)},
	}
}

func zoneTerm(values ...string) v1.TopologySelectorTerm {
	return v1.TopologySelectorTerm{MatchLabelExpressions: []v1.TopologySelectorLabelRequirement{{Key: zoneKey, Values: values}}}
}

func node(name, zone string) *v1.Node {
	return &v1.Node{ObjectMeta: metav1.ObjectMeta{Name: name, Labels: map[string]string{zoneKey: zone}}}
}

func resolvedFixture() fixture {
	return fixture{
		pvcs:     []*v1.PersistentVolumeClaim{snapshotPVC("c", "snap")},
		scs:      []*storagev1.StorageClass{wffcClass()},
		snaps:    []*snapv1.VolumeSnapshot{boundSnapshot("snap", "content")},
		contents: []*snapv1.VolumeSnapshotContent{content("content", "snap", zoneTerm("a"))},
		nodes:    []*v1.Node{node("node-a", "a"), node("node-b", "b")},
	}
}

// Admission runs before the API server assigns a name or UID.
func newPod(claimNames ...string) *v1.Pod {
	pod := &v1.Pod{ObjectMeta: metav1.ObjectMeta{GenerateName: "app-"}}
	for _, cn := range claimNames {
		pod.Spec.Volumes = append(pod.Spec.Volumes, v1.Volume{
			Name:         cn,
			VolumeSource: v1.VolumeSource{PersistentVolumeClaim: &v1.PersistentVolumeClaimVolumeSource{ClaimName: cn}},
		})
	}
	return pod
}

func zoneAffinity(values ...string) *v1.NodeAffinity {
	return &v1.NodeAffinity{RequiredDuringSchedulingIgnoredDuringExecution: &v1.NodeSelector{
		NodeSelectorTerms: []v1.NodeSelectorTerm{{MatchExpressions: []v1.NodeSelectorRequirement{
			{Key: zoneKey, Operator: v1.NodeSelectorOpIn, Values: values},
		}}},
	}}
}

func mustJSON(t *testing.T, obj any) []byte {
	t.Helper()
	b, err := json.Marshal(obj)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func post(t *testing.T, wh *Webhook, path string, req *admissionv1.AdmissionRequest) *admissionv1.AdmissionResponse {
	t.Helper()
	review := admissionv1.AdmissionReview{
		TypeMeta: metav1.TypeMeta{APIVersion: "admission.k8s.io/v1", Kind: "AdmissionReview"},
		Request:  req,
	}
	httpReq := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(mustJSON(t, review)))
	httpReq.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	wh.Handler().ServeHTTP(rec, httpReq)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST %s: status %d: %s", path, rec.Code, rec.Body.String())
	}
	out := &admissionv1.AdmissionReview{}
	if err := json.Unmarshal(rec.Body.Bytes(), out); err != nil {
		t.Fatalf("decoding response: %v", err)
	}
	if out.APIVersion != "admission.k8s.io/v1" || out.Kind != "AdmissionReview" {
		t.Errorf("response TypeMeta = %+v", out.TypeMeta)
	}
	if out.Response == nil || out.Response.UID != req.UID {
		t.Fatalf("response %+v does not match request UID %q", out.Response, req.UID)
	}
	return out.Response
}

func admitPod(t *testing.T, wh *Webhook, pod *v1.Pod) (*admissionv1.AdmissionResponse, *v1.Pod) {
	t.Helper()
	raw := mustJSON(t, pod)
	resp := post(t, wh, "/mutate-pods", &admissionv1.AdmissionRequest{
		UID:       "req-uid",
		Operation: admissionv1.Create,
		Namespace: "default",
		Object:    runtime.RawExtension{Raw: raw},
	})
	if !resp.Allowed {
		t.Fatalf("pod not allowed: %+v", resp.Result)
	}
	if len(resp.Patch) == 0 {
		return resp, pod
	}
	if resp.PatchType == nil || *resp.PatchType != admissionv1.PatchTypeJSONPatch {
		t.Fatalf("patch type = %v, want JSONPatch", resp.PatchType)
	}
	patch, err := jsonpatch.DecodePatch(resp.Patch)
	if err != nil {
		t.Fatalf("decoding patch %s: %v", resp.Patch, err)
	}
	patched, err := patch.Apply(raw)
	if err != nil {
		t.Fatalf("applying patch %s: %v", resp.Patch, err)
	}
	out := &v1.Pod{}
	if err := json.Unmarshal(patched, out); err != nil {
		t.Fatal(err)
	}
	return resp, out
}

func affinityOf(pod *v1.Pod) *v1.NodeAffinity {
	if pod.Spec.Affinity == nil {
		return nil
	}
	return pod.Spec.Affinity.NodeAffinity
}

func TestMutatePod(t *testing.T) {
	userTerm := v1.NodeSelectorTerm{MatchExpressions: []v1.NodeSelectorRequirement{{Key: "disk", Operator: v1.NodeSelectorOpIn, Values: []string{"ssd"}}}}

	testcases := map[string]struct {
		fixture      fixture
		pod          *v1.Pod
		wantPatch    bool
		wantGated    bool
		wantAffinity *v1.NodeAffinity
		wantWarning  string
		check        func(t *testing.T, pod *v1.Pod)
	}{
		"resolved topology is merged without a gate": {
			fixture:      resolvedFixture(),
			pod:          newPod("c"),
			wantPatch:    true,
			wantAffinity: zoneAffinity("a"),
		},
		"pending snapshot adds the gate": {
			fixture: func() fixture {
				f := resolvedFixture()
				f.snaps = []*snapv1.VolumeSnapshot{{ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "snap"}}}
				return f
			}(),
			pod:       newPod("c"),
			wantPatch: true,
			wantGated: true,
		},
		"missing claim adds the gate": {
			fixture:   fixture{scs: []*storagev1.StorageClass{wffcClass()}},
			pod:       newPod("c"),
			wantPatch: true,
			wantGated: true,
		},
		"missing named storage class adds the gate": {
			fixture: func() fixture {
				f := resolvedFixture()
				f.scs = nil
				return f
			}(),
			pod:       newPod("c"),
			wantPatch: true,
			wantGated: true,
		},
		"user affinity is combined with the topology": {
			fixture: resolvedFixture(),
			pod: func() *v1.Pod {
				p := newPod("c")
				p.Spec.Affinity = &v1.Affinity{NodeAffinity: &v1.NodeAffinity{
					RequiredDuringSchedulingIgnoredDuringExecution: &v1.NodeSelector{NodeSelectorTerms: []v1.NodeSelectorTerm{userTerm}},
				}}
				return p
			}(),
			wantPatch: true,
			wantAffinity: &v1.NodeAffinity{RequiredDuringSchedulingIgnoredDuringExecution: &v1.NodeSelector{
				NodeSelectorTerms: []v1.NodeSelectorTerm{{MatchExpressions: []v1.NodeSelectorRequirement{
					userTerm.MatchExpressions[0],
					{Key: zoneKey, Operator: v1.NodeSelectorOpIn, Values: []string{"a"}},
				}}},
			}},
		},
		"existing labels, annotations, pod affinity and gates are kept": {
			fixture: func() fixture {
				f := resolvedFixture()
				f.snaps = []*snapv1.VolumeSnapshot{{ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "snap"}}}
				return f
			}(),
			pod: func() *v1.Pod {
				p := newPod("c")
				p.Labels = map[string]string{"app": "x"}
				p.Annotations = map[string]string{"note": "y"}
				p.Spec.SchedulingGates = []v1.PodSchedulingGate{{Name: "other"}}
				return p
			}(),
			wantPatch: true,
			wantGated: true,
			check: func(t *testing.T, pod *v1.Pod) {
				if pod.Labels["app"] != "x" || pod.Annotations["note"] != "y" {
					t.Errorf("existing metadata lost: %v %v", pod.Labels, pod.Annotations)
				}
				if len(pod.Spec.SchedulingGates) != 2 || pod.Spec.SchedulingGates[0].Name != "other" {
					t.Errorf("scheduling gates = %v", pod.Spec.SchedulingGates)
				}
			},
		},
		"existing annotations and pod affinity are kept when merging": {
			fixture: resolvedFixture(),
			pod: func() *v1.Pod {
				p := newPod("c")
				p.Labels = map[string]string{"app": "x"}
				p.Annotations = map[string]string{"note": "y"}
				p.Spec.Affinity = &v1.Affinity{PodAntiAffinity: &v1.PodAntiAffinity{}}
				return p
			}(),
			wantPatch:    true,
			wantAffinity: zoneAffinity("a"),
			check: func(t *testing.T, pod *v1.Pod) {
				if pod.Labels["app"] != "x" || pod.Annotations["note"] != "y" || pod.Spec.Affinity.PodAntiAffinity == nil {
					t.Errorf("existing fields lost: %+v", pod)
				}
			},
		},
		"content without affinity is not patched": {
			fixture: func() fixture {
				f := resolvedFixture()
				f.contents = []*snapv1.VolumeSnapshotContent{content("content", "snap")}
				return f
			}(),
			pod: newPod("c"),
		},
		"unsatisfiable topology is admitted unchanged with a warning": {
			fixture: func() fixture {
				f := resolvedFixture()
				f.contents = []*snapv1.VolumeSnapshotContent{content("content", "snap", zoneTerm())}
				return f
			}(),
			pod:         newPod("c"),
			wantWarning: "matches no node",
		},
		"too many terms is not applied": {
			fixture: func() fixture {
				f := resolvedFixture()
				f.contents = []*snapv1.VolumeSnapshotContent{content("content", "snap", zoneTerm("a"), v1.TopologySelectorTerm{
					MatchLabelExpressions: []v1.TopologySelectorLabelRequirement{{Key: "rack", Values: []string{"r"}}},
				})}
				f.maxTerms = 1
				return f
			}(),
			pod:         newPod("c"),
			wantWarning: "more than 1 node selector terms",
		},
		"preset nodeName is not patched": {
			fixture: resolvedFixture(),
			pod: func() *v1.Pod {
				p := newPod("c")
				p.Spec.NodeName = "node-b"
				return p
			}(),
		},
		"already gated and labeled pod is not patched": {
			fixture: resolvedFixture(),
			pod: func() *v1.Pod {
				p := newPod("c")
				p.Labels = map[string]string{topology.ManagedLabel: "true"}
				p.Spec.SchedulingGates = []v1.PodSchedulingGate{{Name: topology.SchedulingGate}}
				return p
			}(),
			wantGated: true,
		},
		"gated pod without the label gets the label": {
			fixture: resolvedFixture(),
			pod: func() *v1.Pod {
				p := newPod("c")
				p.Spec.SchedulingGates = []v1.PodSchedulingGate{{Name: topology.SchedulingGate}}
				return p
			}(),
			wantPatch: true,
			wantGated: true,
		},
		"pod without snapshot volumes is not patched": {
			fixture: resolvedFixture(),
			pod: &v1.Pod{Spec: v1.PodSpec{Volumes: []v1.Volume{{
				Name: "scratch", VolumeSource: v1.VolumeSource{EmptyDir: &v1.EmptyDirVolumeSource{}},
			}}}},
		},
		"claim with Immediate binding is not patched": {
			fixture: func() fixture {
				f := resolvedFixture()
				pvc := snapshotPVC("c", "snap")
				pvc.Spec.StorageClassName = ptr("immediate")
				f.pvcs = []*v1.PersistentVolumeClaim{pvc}
				f.scs = []*storagev1.StorageClass{immediateClass()}
				return f
			}(),
			pod: newPod("c"),
		},
		"ephemeral volume is resolved from its template": {
			fixture: resolvedFixture(),
			pod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{GenerateName: "app-"},
				Spec: v1.PodSpec{Volumes: []v1.Volume{{
					Name: "eph",
					VolumeSource: v1.VolumeSource{Ephemeral: &v1.EphemeralVolumeSource{
						VolumeClaimTemplate: &v1.PersistentVolumeClaimTemplate{Spec: snapshotPVC("", "snap").Spec},
					}},
				}}},
			},
			wantPatch:    true,
			wantAffinity: zoneAffinity("a"),
		},
		"feature gate off": {
			fixture: func() fixture {
				f := resolvedFixture()
				f.disabled = true
				return f
			}(),
			pod: newPod("c"),
		},
	}

	for name, tc := range testcases {
		t.Run(name, func(t *testing.T) {
			resp, pod := admitPod(t, tc.fixture.webhook(), tc.pod)
			if got := len(resp.Patch) > 0; got != tc.wantPatch {
				t.Fatalf("patched = %v, want %v (patch %s)", got, tc.wantPatch, resp.Patch)
			}
			if got := HasGate(pod); got != tc.wantGated {
				t.Errorf("gated = %v, want %v", got, tc.wantGated)
			}
			wantAffinity := tc.wantAffinity
			if wantAffinity == nil {
				wantAffinity = affinityOf(tc.pod)
			}
			if !apiequality.Semantic.DeepEqual(affinityOf(pod), wantAffinity) {
				t.Errorf("node affinity = %+v, want %+v", affinityOf(pod), wantAffinity)
			}
			if labeled := pod.Labels[topology.ManagedLabel] == "true"; tc.wantPatch && labeled != tc.wantGated {
				t.Errorf("managed label = %v, want %v", labeled, tc.wantGated)
			}
			if tc.wantWarning != "" && (len(resp.Warnings) != 1 || !strings.Contains(resp.Warnings[0], tc.wantWarning)) {
				t.Errorf("warnings = %v, want one containing %q", resp.Warnings, tc.wantWarning)
			}
			if tc.wantWarning == "" && len(resp.Warnings) > 0 {
				t.Errorf("unexpected warnings %v", resp.Warnings)
			}
			if tc.check != nil {
				tc.check(t, pod)
			}
		})
	}
}

// Reinvocation must be idempotent and include volumes added by other webhooks.
func TestMutatePodIdempotent(t *testing.T) {
	f := resolvedFixture()
	f.pvcs = append(f.pvcs, snapshotPVC("c2", "snap2"))
	f.snaps = append(f.snaps, boundSnapshot("snap2", "content2"))
	rackTerm := v1.TopologySelectorTerm{MatchLabelExpressions: []v1.TopologySelectorLabelRequirement{{Key: "example.com/rack", Values: []string{"r1"}}}}
	f.contents = append(f.contents, content("content2", "snap2", rackTerm))
	wh := f.webhook()

	_, once := admitPod(t, wh, newPod("c"))
	resp, twice := admitPod(t, wh, once)
	if len(resp.Patch) != 0 {
		t.Fatalf("second admission patched the pod: %s", resp.Patch)
	}
	if !apiequality.Semantic.DeepEqual(once, twice) {
		t.Fatalf("second admission changed the pod")
	}

	withVolume := once.DeepCopy()
	withVolume.Spec.Volumes = append(withVolume.Spec.Volumes, v1.Volume{
		Name:         "c2",
		VolumeSource: v1.VolumeSource{PersistentVolumeClaim: &v1.PersistentVolumeClaimVolumeSource{ClaimName: "c2"}},
	})
	resp, reinvoked := admitPod(t, wh, withVolume)
	if len(resp.Patch) == 0 {
		t.Fatalf("reinvocation with a new snapshot volume did not patch the pod")
	}
	want := &v1.NodeAffinity{RequiredDuringSchedulingIgnoredDuringExecution: &v1.NodeSelector{
		NodeSelectorTerms: []v1.NodeSelectorTerm{{MatchExpressions: []v1.NodeSelectorRequirement{
			{Key: zoneKey, Operator: v1.NodeSelectorOpIn, Values: []string{"a"}},
			{Key: "example.com/rack", Operator: v1.NodeSelectorOpIn, Values: []string{"r1"}},
		}}},
	}}
	if got := affinityOf(reinvoked); !apiequality.Semantic.DeepEqual(got, want) {
		t.Errorf("affinity after reinvocation = %+v, want %+v", got, want)
	}
}

func TestMutatePodBadRequest(t *testing.T) {
	wh := resolvedFixture().webhook()
	resp := post(t, wh, "/mutate-pods", &admissionv1.AdmissionRequest{
		UID:    "req-uid",
		Object: runtime.RawExtension{Raw: []byte(`{"spec": "not a pod spec"}`)},
	})
	if !resp.Allowed || len(resp.Patch) != 0 || len(resp.Warnings) != 1 {
		t.Errorf("response = %+v, want allowed unchanged with a warning", resp)
	}
}

func TestServeRejectsInvalidRequests(t *testing.T) {
	wh := resolvedFixture().webhook()
	testcases := map[string]struct {
		contentType string
		body        string
		wantCode    int
	}{
		"wrong content type":        {contentType: "text/plain", body: "{}", wantCode: http.StatusUnsupportedMediaType},
		"invalid json":              {contentType: "application/json", body: "{", wantCode: http.StatusBadRequest},
		"review without request":    {contentType: "application/json", body: `{"apiVersion":"admission.k8s.io/v1","kind":"AdmissionReview"}`, wantCode: http.StatusBadRequest},
		"content type with charset": {contentType: "application/json; charset=utf-8", body: `{"request":{"uid":"u","object":{}}}`, wantCode: http.StatusOK},
	}
	for name, tc := range testcases {
		t.Run(name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/mutate-pods", strings.NewReader(tc.body))
			req.Header.Set("Content-Type", tc.contentType)
			rec := httptest.NewRecorder()
			wh.Handler().ServeHTTP(rec, req)
			if rec.Code != tc.wantCode {
				t.Errorf("status = %d, want %d: %s", rec.Code, tc.wantCode, rec.Body.String())
			}
		})
	}
}

func TestServeLimitsRequestBody(t *testing.T) {
	wh := resolvedFixture().webhook()
	// A valid review padded with an ignored field to a given total size.
	review := func(size int) string {
		const prefix, suffix = `{"request":{"uid":"u","object":{}},"padding":"`, `"}`
		return prefix + strings.Repeat("x", size-len(prefix)-len(suffix)) + suffix
	}
	testcases := map[string]struct {
		body     string
		wantCode int
	}{
		"at the limit":   {body: review(maxAdmissionReviewBytes), wantCode: http.StatusOK},
		"over the limit": {body: review(maxAdmissionReviewBytes + 1), wantCode: http.StatusRequestEntityTooLarge},
	}
	for name, tc := range testcases {
		t.Run(name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/mutate-pods", strings.NewReader(tc.body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			wh.Handler().ServeHTTP(rec, req)
			if rec.Code != tc.wantCode {
				t.Errorf("status = %d, want %d", rec.Code, tc.wantCode)
			}
		})
	}
}

func pvcUpdate(t *testing.T, oldPVC, newPVC *v1.PersistentVolumeClaim) *admissionv1.AdmissionRequest {
	return &admissionv1.AdmissionRequest{
		UID:       "req-uid",
		Operation: admissionv1.Update,
		Namespace: "default",
		Object:    runtime.RawExtension{Raw: mustJSON(t, newPVC)},
		OldObject: runtime.RawExtension{Raw: mustJSON(t, oldPVC)},
	}
}

func withSelectedNode(pvc *v1.PersistentVolumeClaim, nodeName string) *v1.PersistentVolumeClaim {
	pvc = pvc.DeepCopy()
	if pvc.Annotations == nil {
		pvc.Annotations = map[string]string{}
	}
	pvc.Annotations[topology.SelectedNodeAnnotation] = nodeName
	return pvc
}

func TestValidatePVC(t *testing.T) {
	pvc := snapshotPVC("c", "snap")
	withContent := func(c *snapv1.VolumeSnapshotContent) fixture {
		f := resolvedFixture()
		f.contents = []*snapv1.VolumeSnapshotContent{c}
		return f
	}
	notReady := func(c *snapv1.VolumeSnapshotContent) *snapv1.VolumeSnapshotContent {
		c.Status = nil
		return c
	}
	pendingMessage := func(waitingFor string) []string {
		return []string{"PersistentVolumeClaim default/c", "is not known yet: waiting for " + waitingFor}
	}
	testcases := map[string]struct {
		fixture     fixture
		oldPVC      *v1.PersistentVolumeClaim
		newPVC      *v1.PersistentVolumeClaim
		wantAllowed bool
		// wantMessage defaults to a node mismatch with content "content".
		wantMessage []string
	}{
		"matching node is allowed": {
			fixture: resolvedFixture(), oldPVC: pvc, newPVC: withSelectedNode(pvc, "node-a"), wantAllowed: true,
		},
		"node outside the topology is denied": {
			fixture: resolvedFixture(), oldPVC: pvc, newPVC: withSelectedNode(pvc, "node-b"),
		},
		"changing to a node outside the topology is denied": {
			fixture: resolvedFixture(), oldPVC: withSelectedNode(pvc, "node-a"), newPVC: withSelectedNode(pvc, "node-b"),
		},
		"node matching the second term is allowed": {
			fixture: func() fixture {
				f := resolvedFixture()
				f.contents = []*snapv1.VolumeSnapshotContent{content("content", "snap", zoneTerm("a"), zoneTerm("b"))}
				return f
			}(),
			oldPVC: pvc, newPVC: withSelectedNode(pvc, "node-b"), wantAllowed: true,
		},
		"unchanged annotation is allowed": {
			fixture: resolvedFixture(), oldPVC: withSelectedNode(pvc, "node-b"), newPVC: withSelectedNode(pvc, "node-b"), wantAllowed: true,
		},
		"removed annotation is allowed": {
			fixture: resolvedFixture(), oldPVC: withSelectedNode(pvc, "node-b"), newPVC: pvc, wantAllowed: true,
		},
		"bound claim is allowed": {
			fixture: resolvedFixture(),
			oldPVC:  pvc,
			newPVC: func() *v1.PersistentVolumeClaim {
				p := withSelectedNode(pvc, "node-b")
				p.Spec.VolumeName = "pv"
				return p
			}(),
			wantAllowed: true,
		},
		"claim with Immediate binding is allowed": {
			fixture: func() fixture {
				f := resolvedFixture()
				f.scs = append(f.scs, immediateClass())
				return f
			}(),
			oldPVC: pvc,
			newPVC: func() *v1.PersistentVolumeClaim {
				p := withSelectedNode(pvc, "node-b")
				p.Spec.StorageClassName = ptr("immediate")
				return p
			}(),
			wantAllowed: true,
		},
		"content without affinity is allowed": {
			fixture: func() fixture {
				f := resolvedFixture()
				f.contents = []*snapv1.VolumeSnapshotContent{content("content", "snap")}
				return f
			}(),
			oldPVC: pvc, newPVC: withSelectedNode(pvc, "node-b"), wantAllowed: true,
		},
		"unknown node is allowed": {
			fixture: resolvedFixture(), oldPVC: pvc, newPVC: withSelectedNode(pvc, "node-x"), wantAllowed: true,
		},
		"missing snapshot is denied": {
			fixture: func() fixture {
				f := resolvedFixture()
				f.snaps = nil
				return f
			}(),
			oldPVC: pvc, newPVC: withSelectedNode(pvc, "node-a"),
			wantMessage: pendingMessage("VolumeSnapshot default/snap"),
		},
		"unbound snapshot is denied": {
			fixture: func() fixture {
				f := resolvedFixture()
				f.snaps = []*snapv1.VolumeSnapshot{{ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "snap"}}}
				return f
			}(),
			oldPVC: pvc, newPVC: withSelectedNode(pvc, "node-a"),
			wantMessage: pendingMessage("VolumeSnapshot default/snap"),
		},
		"missing content is denied": {
			fixture: func() fixture {
				f := resolvedFixture()
				f.contents = nil
				return f
			}(),
			oldPVC: pvc, newPVC: withSelectedNode(pvc, "node-a"),
			wantMessage: pendingMessage("VolumeSnapshotContent content"),
		},
		"content bound to another snapshot is denied": {
			fixture: withContent(content("content", "other", zoneTerm("a"))),
			oldPVC:  pvc, newPVC: withSelectedNode(pvc, "node-a"),
			wantMessage: pendingMessage("VolumeSnapshotContent content to bind to VolumeSnapshot default/snap"),
		},
		"content not ready and without affinity yet is denied": {
			fixture: withContent(notReady(content("content", "snap"))),
			oldPVC:  pvc, newPVC: withSelectedNode(pvc, "node-a"),
			wantMessage: pendingMessage("VolumeSnapshotContent content"),
		},
		"content not ready with affinity allows a matching node": {
			fixture: withContent(notReady(content("content", "snap", zoneTerm("a")))),
			oldPVC:  pvc, newPVC: withSelectedNode(pvc, "node-a"), wantAllowed: true,
		},
		"content not ready with affinity denies another node": {
			fixture: withContent(notReady(content("content", "snap", zoneTerm("a")))),
			oldPVC:  pvc, newPVC: withSelectedNode(pvc, "node-b"),
		},
		"feature gate off is allowed": {
			fixture: func() fixture {
				f := resolvedFixture()
				f.disabled = true
				return f
			}(),
			oldPVC: pvc, newPVC: withSelectedNode(pvc, "node-b"), wantAllowed: true,
		},
	}
	for name, tc := range testcases {
		t.Run(name, func(t *testing.T) {
			resp := post(t, tc.fixture.webhook(), "/validate-pvcs", pvcUpdate(t, tc.oldPVC, tc.newPVC))
			if resp.Allowed != tc.wantAllowed {
				t.Fatalf("allowed = %v, want %v (%+v)", resp.Allowed, tc.wantAllowed, resp.Result)
			}
			if tc.wantAllowed {
				return
			}
			wantMessage := tc.wantMessage
			if wantMessage == nil {
				wantMessage = []string{`VolumeSnapshotContent "content"`, `node "node-b"`}
			}
			if resp.Result == nil || resp.Result.Code != http.StatusForbidden {
				t.Fatalf("denial result = %+v", resp.Result)
			}
			for _, want := range wantMessage {
				if !strings.Contains(resp.Result.Message, want) {
					t.Errorf("denial message %q does not contain %q", resp.Result.Message, want)
				}
			}
		})
	}
}

func TestValidatePVCBadRequest(t *testing.T) {
	wh := resolvedFixture().webhook()
	valid := mustJSON(t, snapshotPVC("c", "snap"))
	for name, req := range map[string]*admissionv1.AdmissionRequest{
		"bad object":     {UID: "u", Object: runtime.RawExtension{Raw: []byte(`{"spec": 1}`)}, OldObject: runtime.RawExtension{Raw: valid}},
		"bad old object": {UID: "u", Object: runtime.RawExtension{Raw: valid}, OldObject: runtime.RawExtension{Raw: []byte(`{"spec": 1}`)}},
	} {
		t.Run(name, func(t *testing.T) {
			resp := post(t, wh, "/validate-pvcs", req)
			if !resp.Allowed || len(resp.Warnings) != 1 {
				t.Errorf("response = %+v, want allowed with a warning", resp)
			}
		})
	}
}

func TestReadyz(t *testing.T) {
	synced := false
	wh := resolvedFixture().webhook()
	wh.Synced = func() bool { return synced }

	get := func() int {
		rec := httptest.NewRecorder()
		wh.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/readyz", nil))
		return rec.Code
	}
	if code := get(); code != http.StatusServiceUnavailable {
		t.Errorf("/readyz before sync = %d, want %d", code, http.StatusServiceUnavailable)
	}
	synced = true
	if code := get(); code != http.StatusOK {
		t.Errorf("/readyz after sync = %d, want %d", code, http.StatusOK)
	}
}

func TestMutatePodReinvocationImpliedConstraint(t *testing.T) {
	f := resolvedFixture()
	f.pvcs = append(f.pvcs, snapshotPVC("c2", "snap2"))
	f.snaps = append(f.snaps, boundSnapshot("snap2", "content2"))
	f.contents = append(f.contents, content("content2", "snap2", zoneTerm("a", "b")))
	wh := f.webhook()

	_, once := admitPod(t, wh, newPod("c"))
	once.Spec.Volumes = append(once.Spec.Volumes, v1.Volume{
		Name:         "c2",
		VolumeSource: v1.VolumeSource{PersistentVolumeClaim: &v1.PersistentVolumeClaimVolumeSource{ClaimName: "c2"}},
	})
	if resp, _ := admitPod(t, wh, once); len(resp.Patch) != 0 {
		t.Errorf("implied constraint produced a patch: %s", resp.Patch)
	}
}
