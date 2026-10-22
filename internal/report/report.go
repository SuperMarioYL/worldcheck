// Package report renders worldcheck results: the terminal tables the m1 and m2
// commands print today, and (as an m3 stub) the self-contained HTML
// reconciliation report.
package report

import (
	"fmt"
	"io"
	"text/tabwriter"

	"github.com/SuperMarioYL/worldcheck/internal/world"
)

// RenderStatus prints a provisioned world's manifest as terminal tables:
// services, seeded tables, and the seeded-state fingerprint.
func RenderStatus(w io.Writer, m *world.Manifest) error {
	fmt.Fprintf(w, "world      %s\n", m.World)
	fmt.Fprintf(w, "project    %s\n", m.ComposeProject)
	fmt.Fprintf(w, "seed       %s (sha256:%s)\n", m.Seed.File, short(m.Seed.SHA256))
	fmt.Fprintf(w, "database   %s\n", m.Seed.Database)
	fmt.Fprintf(w, "provisioned %s (worldcheck %s)\n", m.CreatedAt.Format("2006-01-02 15:04:05 MST"), m.ToolVersion)

	tw := tabwriter.NewWriter(w, 2, 4, 2, ' ', 0)
	fmt.Fprintln(tw, "\nSERVICE\tIMAGE")
	for _, svc := range m.Services {
		fmt.Fprintf(tw, "%s\t%s\n", svc.Name, svc.Image)
	}
	fmt.Fprintln(tw, "\nTABLE\tROWS")
	for _, t := range m.Tables {
		fmt.Fprintf(tw, "%s\t%d\n", t.Name, t.Rows)
	}
	if err := tw.Flush(); err != nil {
		return err
	}
	fmt.Fprintf(w, "\nfingerprint (%s)\n  %s\n", m.Fingerprint.Algorithm, m.Fingerprint.Value)
	return nil
}

// RenderFingerprint prints the one-line seeded-state fingerprint summary the
// `up` command ends with.
func RenderFingerprint(w io.Writer, m *world.Manifest) {
	fmt.Fprintf(w, "seeded-state fingerprint: %s\n", m.Fingerprint.Value)
}

// RenderDiff prints a mutation diff as terminal tables: the filesystem A/C/D
// lists and the row deltas, mapped to the checkpoint they diff against.
func RenderDiff(w io.Writer, d *world.MutationDiff) error {
	fmt.Fprintf(w, "checkpoint %s\n", d.Label)
	fmt.Fprintf(w, "mutations  %s\n", d.Reconcile)

	tw := tabwriter.NewWriter(w, 2, 4, 2, ' ', 0)
	fmt.Fprintln(tw, "\nKIND\tCONTAINER\tPATH")
	renderChanges(tw, "ADDED", d.FS.Added)
	renderChanges(tw, "CHANGED", d.FS.Changed)
	renderChanges(tw, "DELETED", d.FS.Deleted)
	fmt.Fprintln(tw, "\nKIND\tROW")
	renderRows(tw, "INSERTED", d.DBRows.Inserted)
	renderRows(tw, "UPDATED", d.DBRows.Updated)
	renderRows(tw, "DELETED", d.DBRows.Deleted)
	return tw.Flush()
}

func renderChanges(tw io.Writer, kind string, changes []world.FileChange) {
	for _, c := range changes {
		fmt.Fprintf(tw, "%s\t%s\t%s\n", kind, c.Container, c.Path)
	}
}

func renderRows(tw io.Writer, kind string, rows []string) {
	for _, row := range rows {
		fmt.Fprintf(tw, "%s\t%s\n", kind, row)
	}
}

// WriteHTML renders the reconciliation report as a self-contained HTML file:
// verification result plus file changes and row deltas mapped to the
// checkpoint they diff against.
//
// m3 stub — returns world.ErrNotImplemented until m3_run_reconcile ships.
func WriteHTML(path string, diff *world.MutationDiff) error {
	_ = path
	_ = diff
	return world.ErrNotImplemented
}

func short(digest string) string {
	if len(digest) > 12 {
		return digest[:12]
	}
	return digest
}
