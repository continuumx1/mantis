package render

import (
	"testing"
	"time"

	"github.com/continuumx1/knw/internal/history"
)

func TestDeploymentHistory_Golden(t *testing.T) {
	revisions := []history.Revision{
		{
			Number:  1,
			Time:    time.Date(2026, 8, 2, 9, 0, 0, 0, time.UTC),
			Initial: true,
		},
		{
			Number:  2,
			Time:    time.Date(2026, 8, 9, 14, 30, 0, 0, time.UTC),
			Changes: []string{"env LOG_LEVEL: info → debug"},
		},
		{
			Number:      3,
			Time:        time.Date(2026, 8, 11, 20, 15, 0, 0, time.UTC),
			ChangeCause: "kubectl set image",
			Changes:     []string{"container web image: nginx:1.24 → nginx:1.25"},
		},
	}

	want := "DEPLOYMENT/web\n\n" +
		"HISTORY\n\n" +
		"Revision 3  (2026-08-11 20:15 UTC)  ← current\n" +
		"  └── cause: kubectl set image\n" +
		"  └── container web image: nginx:1.24 → nginx:1.25\n" +
		"\n" +
		"Revision 2  (2026-08-09 14:30 UTC)\n" +
		"  └── env LOG_LEVEL: info → debug\n" +
		"\n" +
		"Revision 1  (2026-08-02 09:00 UTC)\n" +
		"  └── initial revision\n"

	got := DeploymentHistory("web", revisions)
	if got != want {
		t.Errorf("DeploymentHistory output mismatch.\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

func TestDeploymentHistory_Empty(t *testing.T) {
	want := "DEPLOYMENT/web\n\n" +
		"HISTORY\n\n" +
		"  └── No revision history found\n"

	got := DeploymentHistory("web", nil)
	if got != want {
		t.Errorf("empty history mismatch.\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}
