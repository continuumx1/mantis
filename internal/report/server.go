package report

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/continuumx1/mantis/internal/snapshotapi"
)

// Limits on one report request. They bound memory, not usefulness: the UI
// partitions very large clusters into per-namespace diagrams well before
// these are reached.
const (
	maxRequestBytes = 96 << 20
	maxPages        = 500
	maxCards        = 120000
	minPassword     = 8
	maxPassword     = 256
)

// Request is POST /api/report.
type Request struct {
	Revision uint64       `json:"revision"`
	Password string       `json:"password"`
	Contents Contents     `json:"contents"`
	Pages    []LayoutPage `json:"pages"`
}

// Service is the mantis-report HTTP service. It is stateless: each report is
// built within one request and streamed back, so any replica can serve any
// request and nothing about a report outlives it.
type Service struct {
	EngineURL string
	Client    *http.Client
	slots     chan struct{}
}

// NewService builds the service; maxConcurrent bounds simultaneous reports
// per replica (each holds a cluster snapshot in memory while it runs).
func NewService(engineURL string, maxConcurrent int) *Service {
	if maxConcurrent < 1 {
		maxConcurrent = 1
	}
	return &Service{
		EngineURL: strings.TrimRight(engineURL, "/"),
		Client:    &http.Client{Timeout: 2 * time.Minute},
		slots:     make(chan struct{}, maxConcurrent),
	}
}

// Handler serves the report API and probes.
func (s *Service) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/report", s.handleReport)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, "ok") })
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, "ready") })
	return mux
}

func jsonError(w http.ResponseWriter, status int, msg string, extra map[string]any) {
	body := map[string]any{"error": msg}
	for k, v := range extra {
		body[k] = v
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

// handleReport validates the request, fetches the engine's snapshot, then
// streams newline-delimited JSON progress events and finally the encrypted
// PDF (base64) — so the browser can show real stages and cancel by aborting
// the request, which cancels the work here too.
func (s *Service) handleReport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, http.StatusMethodNotAllowed, "use POST", nil)
		return
	}
	start := time.Now()
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBytes)
	var req Request
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, http.StatusBadRequest, "invalid report request", nil)
		return
	}
	// The password is never logged, echoed or stored; it lives only in this
	// request until the document is encrypted.
	if n := len([]rune(req.Password)); n < minPassword || n > maxPassword {
		jsonError(w, http.StatusBadRequest, fmt.Sprintf("the PDF password must be %d to %d characters", minPassword, maxPassword), nil)
		return
	}
	if len(req.Pages) > maxPages {
		jsonError(w, http.StatusRequestEntityTooLarge, "too many topology pages", nil)
		return
	}
	cards := 0
	for _, p := range req.Pages {
		cards += len(p.Nodes)
	}
	if cards > maxCards {
		jsonError(w, http.StatusRequestEntityTooLarge, "topology too large for one report", nil)
		return
	}
	select {
	case s.slots <- struct{}{}:
		defer func() { <-s.slots }()
	default:
		jsonError(w, http.StatusTooManyRequests, "another report is being generated; try again in a moment", nil)
		return
	}

	ctx := r.Context()
	snapResp, err := s.fetchSnapshot(ctx)
	if err != nil {
		slog.Error("report", "event", "snapshot_failed", "error", err.Error())
		jsonError(w, http.StatusBadGateway, "could not read the cluster snapshot from mantis-engine", nil)
		return
	}
	if req.Revision != 0 && req.Revision != snapResp.Revision {
		// The UI laid the topology out for an older graph; it re-lays out
		// and retries rather than receiving a diagram that no longer matches.
		jsonError(w, http.StatusConflict, "the cluster changed while the report was being prepared", map[string]any{"revision": snapResp.Revision})
		return
	}
	snap, err := FromResponse(snapResp)
	if err != nil {
		jsonError(w, http.StatusBadGateway, "the engine snapshot could not be read", nil)
		return
	}
	pages := make([]*page, 0, len(req.Pages))
	for i, lp := range req.Pages {
		p, err := validatePage(lp, snap)
		if err != nil {
			jsonError(w, http.StatusBadRequest, fmt.Sprintf("topology page %d: %s", i+1, err.Error()), nil)
			return
		}
		pages = append(pages, p)
	}

	w.Header().Set("Content-Type", "application/x-ndjson")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	flusher, _ := w.(http.Flusher)
	enc := json.NewEncoder(w)
	emit := func(v map[string]any) {
		_ = enc.Encode(v)
		if flusher != nil {
			flusher.Flush()
		}
	}
	emit(map[string]any{"stage": "building-topology", "pages": len(pages)})

	plain, err := s.build(ctx, snap, pages, req.Contents, func(stage string) { emit(map[string]any{"stage": stage}) })
	if err != nil {
		if !errors.Is(err, context.Canceled) {
			slog.Error("report", "event", "build_failed", "error", err.Error())
		}
		emit(map[string]any{"stage": "error", "error": "the report could not be rendered"})
		return
	}
	emit(map[string]any{"stage": "encrypting"})
	sealed, err := Encrypt(ctx, plain, req.Password)
	req.Password = ""
	if err != nil {
		slog.Error("report", "event", "encrypt_failed", "error", err.Error())
		emit(map[string]any{"stage": "error", "error": "the report could not be encrypted"})
		return
	}
	name := fmt.Sprintf("mantis-report-%s-%s.pdf", safeName(snap.Graph.Context), snap.TakenAt.Format("20060102-1504"))
	emit(map[string]any{"stage": "ready", "filename": name, "bytes": len(sealed), "pdf": base64.StdEncoding.EncodeToString(sealed)})
	slog.Info("report", "event", "report_generated", "revision", snap.Graph.Revision, "topology_pages", len(pages),
		"bytes", len(sealed), "duration_ms", time.Since(start).Milliseconds())
}

// build runs Build, stopping early if the request was cancelled.
func (s *Service) build(ctx context.Context, snap *Snapshot, pages []*page, c Contents, progress func(string)) ([]byte, error) {
	type res struct {
		b   []byte
		err error
	}
	done := make(chan res, 1)
	go func() {
		b, err := Build(snap, pages, c, func(stage string) {
			if ctx.Err() == nil {
				progress(stage)
			}
		})
		done <- res{b, err}
	}()
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case r := <-done:
		return r.b, r.err
	}
}

func (s *Service) fetchSnapshot(ctx context.Context) (snapshotapi.Response, error) {
	var out snapshotapi.Response
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.EngineURL+snapshotapi.Path, nil)
	if err != nil {
		return out, err
	}
	resp, err := s.Client.Do(req)
	if err != nil {
		return out, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return out, fmt.Errorf("engine returned %d: %s", resp.StatusCode, strings.TrimSpace(string(b)))
	}
	err = json.NewDecoder(resp.Body).Decode(&out)
	return out, err
}

var unsafeChars = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

func safeName(s string) string {
	s = unsafeChars.ReplaceAllString(s, "-")
	if s == "" {
		return "cluster"
	}
	if len(s) > 60 {
		s = s[:60]
	}
	return s
}
