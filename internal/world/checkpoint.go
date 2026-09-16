package world

import (
	"context"
	"time"
)

// This file is the m2 stub (checkpoint leg of the primitive). The types below
// fix the m2_checkpoint_diff contract from the plan; the behavior ships with
// m2. The CLI surfaces ErrNotImplemented as a friendly roadmap note.

// FileChange is one filesystem mutation as reported by `docker diff`
// (A = added, C = changed, D = deleted).
type FileChange struct {
	Container string
	Kind      string
	Path      string
}

// Checkpoint is a named snapshot of world state taken before verification:
// a filesystem baseline for every container plus a hashed Postgres data dump.
type Checkpoint struct {
	Label        string
	CreatedAt    time.Time
	FSBaseline   []FileChange
	DBDumpSHA256 string
}

// CreateCheckpoint snapshots every container in the world (docker commit) and
// the Postgres data (pg_dump --data-only, hashed) under the given label.
//
// m2 stub — returns ErrNotImplemented until m2_checkpoint_diff ships.
func CreateCheckpoint(ctx context.Context, r Runner, worldDir, label string) (*Checkpoint, error) {
	_ = ctx
	_ = r
	_ = worldDir
	_ = label
	return nil, ErrNotImplemented
}
