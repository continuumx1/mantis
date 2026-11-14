package engine

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/continuumx1/mantis/internal/snapshotapi"
)

// handleInternalSnapshot serves the engine's current graph plus the trimmed
// objects behind it to mantis-report (see internal/snapshotapi). In watch mode
// the objects come from the watch cache, so this costs no Kubernetes API
// calls. It is mounted outside /api/: mantis-web never proxies it, and the
// chart's NetworkPolicy lets only mantis-web and mantis-report reach the
// engine at all.
func (s *Server) handleInternalSnapshot(w http.ResponseWriter, r *http.Request) {
	dto, ok := s.cache.get()
	if !ok {
		http.Error(w, "no graph yet", http.StatusServiceUnavailable)
		return
	}
	objs, notReadable, err := snapshotapi.Collect(r.Context(), s.readClient())
	if err != nil {
		slog.Error("snapshot", "event", "collect_failed", "error", err.Error())
		http.Error(w, "could not read cluster objects", http.StatusBadGateway)
		return
	}
	g, err := json.Marshal(dto)
	if err != nil {
		http.Error(w, "encode graph", http.StatusInternalServerError)
		return
	}
	writeJSON(w, r, snapshotapi.Response{
		Revision: dto.Meta.Revision, TakenAt: time.Now().UTC(), SyncMode: s.syncMode(),
		Graph: g, Objects: objs, NotReadable: notReadable,
	})
}
