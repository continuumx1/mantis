package report

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
)

// Encrypt protects a PDF with AES-256 using the ISO 32000-2 (PDF 2.0)
// standard security handler, revision 6, entirely in memory: the unencrypted document is never written
// to disk, and its buffer is zeroed afterwards. The user password opens the
// document; the owner password is random and discarded, so nobody can lift
// the protection without the user password.
func Encrypt(ctx context.Context, plain []byte, userPassword string) (sealed []byte, err error) {
	// pdfcpu reports some failures by panicking; turn them into an error so
	// the caller can report a failed stage instead of a broken stream.
	defer func() {
		if r := recover(); r != nil {
			sealed, err = nil, fmt.Errorf("pdf encryption failed: %v", r)
		}
	}()
	if userPassword == "" {
		return nil, errors.New("a password is required")
	}
	owner := make([]byte, 32)
	if _, err := rand.Read(owner); err != nil {
		return nil, err
	}
	// pdfcpu writes revision 6 — the ISO 32000-2 AES-256 handler — only for
	// PDF 2.0 documents (for older headers it writes Adobe's pre-standard
	// revision 5, whose password check is weaker). The document uses nothing
	// version-specific, so it is declared PDF 2.0.
	plain = asPDF20(plain)
	conf := model.NewAESConfiguration(userPassword, hex.EncodeToString(owner), 256)
	conf.Permissions = model.PermissionsPrint
	var out bytes.Buffer
	err = api.Encrypt(ctx, bytes.NewReader(plain), &out, conf)
	for i := range plain {
		plain[i] = 0
	}
	if err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

// asPDF20 rewrites the "%PDF-1.x" header as "%PDF-2.0" (same length).
func asPDF20(b []byte) []byte {
	if len(b) > 8 && bytes.HasPrefix(b, []byte("%PDF-1.")) {
		copy(b[5:8], "2.0")
	}
	return b
}

// pdfcpu otherwise creates a config directory under $HOME on first use, which
// fails on the read-only root filesystem the service runs with.
func init() { api.DisableConfigDir() }
