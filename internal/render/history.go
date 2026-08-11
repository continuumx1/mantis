package render

import (
	"fmt"
	"strings"
	"time"

	"github.com/continuumx1/knw/internal/history"
)

// DeploymentHistory renders a Deployment's revision history, newest first. Each
// revision shows its number, the time it appeared, an optional recorded change
// cause, and the changes it introduced relative to the previous revision.
func DeploymentHistory(name string, revisions []history.Revision) string {
	var b strings.Builder

	fmt.Fprintf(&b, "DEPLOYMENT/%s\n\n", name)
	b.WriteString("HISTORY\n\n")

	if len(revisions) == 0 {
		b.WriteString("  └── No revision history found\n")
		return b.String()
	}

	for i := len(revisions) - 1; i >= 0; i-- {
		rev := revisions[i]

		marker := ""
		if i == len(revisions)-1 {
			marker = "  ← current"
		}
		fmt.Fprintf(&b, "Revision %d  (%s)%s\n", rev.Number, formatTime(rev.Time), marker)

		if rev.ChangeCause != "" {
			fmt.Fprintf(&b, "  └── cause: %s\n", rev.ChangeCause)
		}

		switch {
		case rev.Initial:
			b.WriteString("  └── initial revision\n")
		case len(rev.Changes) == 0:
			b.WriteString("  └── no pod template changes detected\n")
		default:
			for _, change := range rev.Changes {
				fmt.Fprintf(&b, "  └── %s\n", change)
			}
		}

		b.WriteString("\n")
	}

	return strings.TrimRight(b.String(), "\n") + "\n"
}

// formatTime renders a revision timestamp as an absolute UTC time, so output is
// deterministic and unambiguous across time zones.
func formatTime(t time.Time) string {
	return t.UTC().Format("2006-01-02 15:04 UTC")
}
