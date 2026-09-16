package world

import (
	"context"
	"errors"
)

// ErrNotImplemented marks milestone legs that are stubbed in this build tier:
// m2 (checkpoint/diff) and m3 (run/reconcile report) follow the shipped m1
// provisioning leg.
var ErrNotImplemented = errors.New("not implemented in this milestone")

// FSChanges groups the filesystem mutations of a diff.
type FSChanges struct {
	Added   []FileChange
	Changed []FileChange
	Deleted []FileChange
}

// RowDeltas groups the database mutations of a diff, compared row by row
// against the checkpoint's data snapshot.
type RowDeltas struct {
	Inserted []string
	Updated  []string
	Deleted  []string
}

// MutationDiff is the machine-checkable comparison of world state against a
// named checkpoint: filesystem A/C/D plus row deltas, purely mechanical —
// docker diff and pg_dump hashes, no heuristic judgment.
type MutationDiff struct {
	Label   string
	FS      FSChanges
	DBRows  RowDeltas
	Reconcile string // verify claim x mutations, human-readable (m3)
}

// DiffAgainst compares the current world state against the checkpoint with
// the given label and returns the mutation diff.
//
// m2 stub — returns ErrNotImplemented until m2_checkpoint_diff ships.
func DiffAgainst(ctx context.Context, r Runner, worldDir, label string) (*MutationDiff, error) {
	_ = ctx
	_ = r
	_ = worldDir
	_ = label
	return nil, ErrNotImplemented
}
