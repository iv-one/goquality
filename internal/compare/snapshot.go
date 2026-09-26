// Package compare compares two goquality snapshots and applies the
// regression policy: a change must not introduce findings or lower the score
// of a check that has none.
package compare

import (
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"time"

	"github.com/iv-one/goquality/internal/check"
)

// Schema is the snapshot format version. Bump it on incompatible changes.
const Schema = 1

// Snapshot is a report together with what is needed to tell how and from
// which source it was produced.
type Snapshot struct {
	Schema    int          `json:"schema"`
	Version   string       `json:"goquality_version"`
	Commit    string       `json:"commit,omitempty"`
	Dirty     bool         `json:"dirty,omitempty"` // uncommitted changes on top of Commit
	Timestamp time.Time    `json:"timestamp"`
	Settings  Settings     `json:"settings"`
	Report    check.Report `json:"report"`
}

// Settings are the options that change which findings a run reports.
// Snapshots with different settings are not comparable.
type Settings struct {
	Patterns  []string `json:"patterns"`
	Checks    []string `json:"checks"` // checks that were run
	Cover     bool     `json:"cover,omitempty"`
	CycloOver int      `json:"cyclo_over"`
}

// Read loads a snapshot written by goquality collect.
func Read(path string) (Snapshot, error) {
	data, err := os.ReadFile(path) //nolint:gosec // the user names the snapshot to read
	if err != nil {
		return Snapshot{}, err
	}
	var s Snapshot
	if err := json.Unmarshal(data, &s); err != nil {
		return Snapshot{}, fmt.Errorf("%s: %w", path, err)
	}
	if s.Schema != Schema {
		return Snapshot{}, fmt.Errorf("%s: snapshot schema %d is not supported (want %d); collect it again", path, s.Schema, Schema)
	}
	return s, nil
}

// comparable reports why two snapshots cannot be compared, or "" if they
// can. Different check sets are fine: only checks run in both are compared.
func comparable(base, cur Settings) string {
	switch {
	case !slices.Equal(base.Patterns, cur.Patterns):
		return fmt.Sprintf("snapshots analyze different packages (%v vs %v)", base.Patterns, cur.Patterns)
	case base.CycloOver != cur.CycloOver:
		return fmt.Sprintf("snapshots use different --cyclo-over (%d vs %d)", base.CycloOver, cur.CycloOver)
	}
	return ""
}
