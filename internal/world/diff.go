package world

import (
	"context"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
)

// This file is the m2 diff leg of the primitive: compare the world's current
// state against a checkpoint and report what moved.

// ErrNotImplemented marks milestone legs that are stubbed in this build tier:
// m3 (run/reconcile report) follows the shipped m1 provisioning and m2
// checkpoint/diff legs.
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
// named checkpoint: filesystem A/C/D, row deltas, and whether the schema moved.
// Purely mechanical — docker diff and pg_dump, no heuristic judgment.
type MutationDiff struct {
	Label         string
	FS            FSChanges
	DBRows        RowDeltas
	SchemaChanged bool
	// Reconcile is the human-readable mutation summary. The verify-claim half of
	// the reconciliation arrives with m3's `run`.
	Reconcile string
}

// DiffAgainst compares the current world state against the checkpoint with the
// given label and returns the mutation diff.
func DiffAgainst(ctx context.Context, r Runner, worldDir, label string) (*MutationDiff, error) {
	ws, err := resolveWorld(worldDir)
	if err != nil {
		return nil, err
	}
	cp, beforeDump, err := loadCheckpoint(worldDir, label)
	if err != nil {
		return nil, err
	}
	run := func(stdin io.Reader, argv ...string) (string, error) {
		return r.Run(ctx, worldDir, stdin, argv...)
	}
	base := []string{"docker", "compose", "-p", ws.Project}

	current, err := containerDiffs(run, base, ws.Project)
	if err != nil {
		return nil, err
	}
	afterDump, err := dumpDatabase(run, base, ws.DBUser, ws.DBName)
	if err != nil {
		return nil, err
	}
	keys, err := primaryKeys(run, base, ws.DBUser, ws.DBName)
	if err != nil {
		return nil, err
	}

	diff := &MutationDiff{
		Label:         label,
		FS:            diffFileChanges(cp.FSBaseline, current),
		DBRows:        DiffRows(ParseDumpRows(beforeDump), ParseDumpRows(afterDump), keys),
		SchemaChanged: SchemaDigest(beforeDump) != SchemaDigest(afterDump),
	}
	diff.Reconcile = summarizeDiff(diff)
	return diff, nil
}

// diffFileChanges reports the mutations that appeared, changed or disappeared
// since the checkpoint. A path's identity is (container, path) and its status
// is the docker-diff kind, so a path whose status differs from the baseline is
// reported by its current kind; a path that was mutated at checkpoint time and
// is no longer mutated at all is reported as deleted, because the mutation was
// undone.
func diffFileChanges(baseline, current []FileChange) FSChanges {
	base := kindsOf(baseline)
	curr := kindsOf(current)
	var out FSChanges
	for _, c := range current {
		if kind, known := base[changeKey(c)]; known && kind == c.Kind {
			continue // already mutated this way when the checkpoint was taken
		}
		switch c.Kind {
		case "A":
			out.Added = append(out.Added, c)
		case "C":
			out.Changed = append(out.Changed, c)
		case "D":
			out.Deleted = append(out.Deleted, c)
		}
	}
	for _, c := range baseline {
		if _, still := curr[changeKey(c)]; still {
			continue
		}
		if c.Kind == "A" || c.Kind == "C" {
			out.Deleted = append(out.Deleted, c)
		}
	}
	sortFileChanges(out.Added)
	sortFileChanges(out.Changed)
	sortFileChanges(out.Deleted)
	return out
}

// primaryKeys returns the primary-key columns of every table in the world's
// public schema, in key order. The row identity a diff keys by is read from the
// catalog, never inferred from the data.
func primaryKeys(run func(io.Reader, ...string) (string, error), base []string, dbUser, dbName string) (map[string][]string, error) {
	const query = `SELECT tc.table_name, kcu.column_name ` +
		`FROM information_schema.table_constraints tc ` +
		`JOIN information_schema.key_column_usage kcu ` +
		`ON tc.constraint_name = kcu.constraint_name AND tc.table_schema = kcu.table_schema ` +
		`WHERE tc.constraint_type = 'PRIMARY KEY' AND tc.table_schema = 'public' ` +
		`ORDER BY tc.table_name, kcu.ordinal_position`
	argv := append(slices.Clone(base),
		"exec", "-T", dbServiceName,
		"psql", "-U", dbUser, "-d", dbName, "-t", "-A", "-F", "|", "-c", query,
	)
	out, err := run(nil, argv...)
	if err != nil {
		return nil, fmt.Errorf("reading the world's primary keys: %w", err)
	}
	keys := map[string][]string{}
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		table, column, ok := strings.Cut(line, "|")
		if !ok {
			continue
		}
		keys[table] = append(keys[table], column)
	}
	return keys, nil
}

// summarizeDiff renders the mechanical mutation counts.
func summarizeDiff(d *MutationDiff) string {
	files := len(d.FS.Added) + len(d.FS.Changed) + len(d.FS.Deleted)
	rows := len(d.DBRows.Inserted) + len(d.DBRows.Updated) + len(d.DBRows.Deleted)
	if files == 0 && rows == 0 && !d.SchemaChanged {
		return "no mutations since the checkpoint"
	}
	schema := "unchanged"
	if d.SchemaChanged {
		schema = "changed"
	}
	return fmt.Sprintf(
		"files %d added / %d changed / %d deleted; rows %d inserted / %d updated / %d deleted; schema %s",
		len(d.FS.Added), len(d.FS.Changed), len(d.FS.Deleted),
		len(d.DBRows.Inserted), len(d.DBRows.Updated), len(d.DBRows.Deleted), schema)
}

func kindsOf(changes []FileChange) map[string]string {
	kinds := make(map[string]string, len(changes))
	for _, c := range changes {
		kinds[changeKey(c)] = c.Kind
	}
	return kinds
}

func changeKey(c FileChange) string {
	return c.Container + "\x1f" + c.Path
}
