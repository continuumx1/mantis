package report

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/continuumx1/mantis/internal/engine"
	"github.com/continuumx1/mantis/internal/graph"
	"github.com/continuumx1/mantis/internal/snapshotapi"
	"github.com/continuumx1/mantis/internal/synth"
)

const secretValue = "s3cr3t-value-never-in-report"

// testSnapshot builds the engine's snapshot response for a synthetic cluster,
// exactly as mantis-engine would serve it.
func testSnapshot(t testing.TB, resources int) snapshotapi.Response {
	t.Helper()
	objs := synth.Generate(synth.Options{Resources: resources, Seed: 3})
	objs = append(objs,
		&corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "leak-check", Namespace: "catalog"}, Data: map[string][]byte{"k": []byte(secretValue)}},
		&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "leak-check", Namespace: "catalog"}, Data: map[string]string{"k": secretValue}},
	)
	cs := fake.NewSimpleClientset(objs...)
	gctx, skipped, err := graph.BuildClusterGraph(context.Background(), cs, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	var ns []string
	for _, o := range objs {
		if n, ok := o.(*corev1.Namespace); ok {
			ns = append(ns, n.Name)
		}
	}
	dto := engine.FromContext(gctx, engine.MetaDTO{Context: "test-cluster", Version: "v1.36.0", Revision: 7, Skipped: skipped, NamespaceList: ns})
	g, _ := json.Marshal(dto)
	o, nr, err := snapshotapi.Collect(context.Background(), cs)
	if err != nil {
		t.Fatal(err)
	}
	return snapshotapi.Response{Revision: 7, SyncMode: "watch", Graph: g, Objects: o, NotReadable: nr}
}

// gridLayout lays every resource out on a grid with straight lines — enough
// geometry to exercise validation and drawing without the browser.
func gridLayout(s *Snapshot) LayoutPage {
	lp := LayoutPage{Title: "Complete cluster", Kind: "full"}
	pos := map[string][2]float64{}
	for i, n := range s.Graph.Nodes {
		x, y := float64(i%30)*260, float64(i/30)*70
		pos[n.ID] = [2]float64{x, y}
		g := "infra"
		if n.Namespace != "" {
			g = "ns:" + n.Namespace
		}
		lp.Nodes = append(lp.Nodes, LNode{VID: n.ID, Type: "resource", ID: n.ID, Group: g, X: x, Y: y, W: 220, H: 44})
	}
	for _, e := range s.Graph.Edges {
		a, b := pos[e.From], pos[e.To]
		lp.Edges = append(lp.Edges, LEdge{From: e.From, To: e.To, Keys: []string{e.Key()}, Points: [][2]float64{{a[0] + 110, a[1]}, {b[0] - 110, b[1]}}})
	}
	lp.Bounds = [4]float64{-150, -50, 30 * 260, float64(len(s.Graph.Nodes)/30+1) * 70}
	return lp
}

func TestSnapshotNeverCarriesSecretOrConfigMapValues(t *testing.T) {
	r := testSnapshot(t, 200)
	b, _ := json.Marshal(r)
	if bytes.Contains(b, []byte(secretValue)) || bytes.Contains(b, []byte(base64.StdEncoding.EncodeToString([]byte(secretValue)))) {
		t.Fatal("the engine snapshot carries a Secret or ConfigMap value")
	}
	found := false
	for _, c := range r.Objects.ConfigMaps {
		if c.Name == "leak-check" {
			found = len(c.Data) == 1 // key kept (for counting), value blanked
		}
	}
	if !found {
		t.Error("ConfigMap key count should survive trimming")
	}
}

func TestBuildAndEncrypt(t *testing.T) {
	// The service runs with a read-only root filesystem: encryption must not
	// need a writable home or config directory.
	t.Setenv("HOME", "/nonexistent-read-only-home")
	t.Setenv("XDG_CONFIG_HOME", "/nonexistent-read-only-home")
	snap, err := FromResponse(testSnapshot(t, 600))
	if err != nil {
		t.Fatal(err)
	}
	p, err := validatePage(gridLayout(snap), snap)
	if err != nil {
		t.Fatal(err)
	}
	plain, err := Build(snap, []*page{p}, Contents{Topology: true, Inventory: true, Health: true, Findings: true}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(plain, []byte("%PDF")) {
		t.Fatal("not a PDF")
	}
	conf := model.NewDefaultConfiguration()
	if err := api.Validate(context.Background(), bytes.NewReader(plain), conf, nil); err != nil {
		t.Fatalf("generated PDF does not validate: %v", err)
	}
	n, _ := api.PageCount(context.Background(), bytes.NewReader(plain), conf)
	if n < 8 {
		t.Errorf("only %d pages", n)
	}
	t.Logf("%d resources, %d relationships -> %d pages, %d KB", len(snap.Graph.Nodes), len(snap.Graph.Edges), n, len(plain)/1024)

	plainCopy := append([]byte(nil), plain...)
	sealed, err := Encrypt(context.Background(), plain, "correct horse")
	if err != nil {
		t.Fatal(err)
	}
	for _, b := range plain {
		if b != 0 {
			t.Fatal("unencrypted buffer was not wiped after encryption")
		}
	}
	if bytes.Equal(sealed, plainCopy) {
		t.Fatal("encryption did not change the document")
	}
	// Without the password (or with a wrong one) the document cannot be read.
	if _, err := api.ReadContext(context.Background(), bytes.NewReader(sealed), model.NewDefaultConfiguration()); err == nil {
		t.Error("encrypted PDF opened without a password")
	}
	wrong := model.NewDefaultConfiguration()
	wrong.UserPW = "wrong password"
	if _, err := api.ReadContext(context.Background(), bytes.NewReader(sealed), wrong); err == nil {
		t.Error("encrypted PDF opened with the wrong password")
	}
	right := model.NewDefaultConfiguration()
	right.UserPW = "correct horse"
	ctx, err := api.ReadContext(context.Background(), bytes.NewReader(sealed), right)
	if err != nil {
		t.Fatalf("encrypted PDF does not open with its password: %v", err)
	}
	var dec bytes.Buffer
	if err := api.Decrypt(context.Background(), bytes.NewReader(sealed), &dec, right); err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if err := api.Validate(context.Background(), bytes.NewReader(dec.Bytes()), model.NewDefaultConfiguration(), nil); err != nil {
		t.Errorf("decrypted report does not validate: %v", err)
	}
	if ctx.E == nil || ctx.E.R != 6 || !ctx.AES4Strings || !ctx.AES4Streams {
		t.Errorf("encryption = %+v; want AES-256 (standard security handler revision 6)", ctx.E)
	}
}

func TestValidatePageRejectsAnythingNotInTheSnapshot(t *testing.T) {
	snap, err := FromResponse(testSnapshot(t, 150))
	if err != nil {
		t.Fatal(err)
	}
	base := gridLayout(snap)
	cases := map[string]func(*LayoutPage){
		"invented resource":     func(lp *LayoutPage) { lp.Nodes[0].ID, lp.Nodes[0].VID = "default/Pod/made-up", "default/Pod/made-up" },
		"invented relationship": func(lp *LayoutPage) { lp.Edges[0].Keys = []string{"a|selects|b"} },
		"unknown namespace":     func(lp *LayoutPage) { lp.Groups = []LGroup{{Key: "ns:nope", X1: 1, Y1: 1}} },
		"non-finite geometry":   func(lp *LayoutPage) { lp.Nodes[1].X = 1e12 },
		"line to nowhere":       func(lp *LayoutPage) { lp.Edges[0].To = "not-on-page" },
	}
	for name, mutate := range cases {
		lp := base
		lp.Nodes = append([]LNode(nil), base.Nodes...)
		lp.Edges = append([]LEdge(nil), base.Edges...)
		mutate(&lp)
		if _, err := validatePage(lp, snap); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

func TestHealthAndFindingsComeFromObservedState(t *testing.T) {
	r := testSnapshot(t, 600)
	snap, _ := FromResponse(r)
	h := ComputeHealth(snap)
	total := 0
	for _, n := range h.Pods {
		total += n
	}
	if total != len(snap.Pods) {
		t.Errorf("pod states cover %d of %d pods", total, len(snap.Pods))
	}
	fs := ComputeFindings(snap, h)
	titles := map[string]int{}
	for _, f := range fs {
		titles[f.Title]++
	}
	// synth generates crash-looping replicas and a missing ConfigMap mount.
	if titles["Pod cannot run"] == 0 {
		t.Errorf("crash-looping pods not reported: %v", titles)
	}
	if titles["Dangling reference"] == 0 {
		t.Errorf("missing ConfigMap not reported: %v", titles)
	}
	for _, f := range fs {
		low := strings.ToLower(f.Title + " " + f.Description)
		for _, claim := range []string{"vulnerab", "compliance", "cve", "insecure"} {
			if strings.Contains(low, claim) {
				t.Errorf("finding makes an assessment Mantis does not perform: %+v", f)
			}
		}
	}
}

// TestServiceEndToEnd runs mantis-report against a stand-in engine: input
// validation, the revision check, streaming progress, and an encrypted PDF.
func TestServiceEndToEnd(t *testing.T) {
	resp := testSnapshot(t, 300)
	engineSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != snapshotapi.Path {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer engineSrv.Close()
	svc := httptest.NewServer(NewService(engineSrv.URL, 1).Handler())
	defer svc.Close()
	snap, _ := FromResponse(resp)
	post := func(req Request) *http.Response {
		b, _ := json.Marshal(req)
		r, err := http.Post(svc.URL+"/api/report", "application/json", bytes.NewReader(b))
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	all := Contents{true, true, true, true}
	if r := post(Request{Revision: 7, Password: "short", Contents: all}); r.StatusCode != 400 {
		t.Errorf("short password: status %d, want 400", r.StatusCode)
	}
	if r := post(Request{Revision: 6, Password: "long enough", Contents: all}); r.StatusCode != 409 {
		t.Errorf("stale revision: status %d, want 409", r.StatusCode)
	}
	r := post(Request{Revision: 7, Password: "long enough", Contents: all, Pages: []LayoutPage{gridLayout(snap)}})
	if r.StatusCode != 200 || r.Header.Get("Content-Type") != "application/x-ndjson" {
		t.Fatalf("status %d %s", r.StatusCode, r.Header.Get("Content-Type"))
	}
	var stages []string
	var final map[string]any
	dec := json.NewDecoder(r.Body)
	for dec.More() {
		var ev map[string]any
		if err := dec.Decode(&ev); err != nil {
			t.Fatal(err)
		}
		stages = append(stages, fmt.Sprint(ev["stage"]))
		final = ev
	}
	if final["stage"] != "ready" {
		t.Fatalf("stages %v, last %v", stages, final["stage"])
	}
	pdf, _ := base64.StdEncoding.DecodeString(final["pdf"].(string))
	if _, err := api.ReadContext(context.Background(), bytes.NewReader(pdf), model.NewDefaultConfiguration()); err == nil {
		t.Error("delivered PDF opens without a password")
	}
	t.Logf("stages: %v; %d KB", stages, len(pdf)/1024)
}

var _ = runtime.Object(nil)
