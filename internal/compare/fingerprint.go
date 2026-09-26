package compare

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"

	"github.com/iv-one/goquality/internal/check"
)

// Fingerprint sets LineHash on each finding that points at a line of a file
// under root: a hash of the line's trimmed content.
func Fingerprint(root string, rep *check.Report) {
	lines := make(map[string][][]byte)
	for i := range rep.Checks {
		for j := range rep.Checks[i].Findings {
			f := &rep.Checks[i].Findings[j]
			if f.File == "" || f.Line <= 0 {
				continue
			}
			ls, ok := lines[f.File]
			if !ok {
				data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(f.File))) //nolint:gosec // a file of the analyzed project
				if err == nil {
					ls = bytes.Split(data, []byte("\n"))
				}
				lines[f.File] = ls
			}
			if f.Line > len(ls) {
				continue
			}
			sum := sha256.Sum256(bytes.TrimSpace(ls[f.Line-1]))
			f.LineHash = hex.EncodeToString(sum[:8])
		}
	}
}
