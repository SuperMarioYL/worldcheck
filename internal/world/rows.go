package world

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// This file reduces a pg_dump capture to the two things a mutation diff needs:
// per-table row content (ParseDumpRows) and a digest of everything that is NOT
// row content (SchemaDigest). Both are purely mechanical — no judgment, no
// heuristics, no schema inference beyond the COPY headers pg_dump writes.

// copyHeader matches the `COPY <table> (<columns>) FROM stdin;` line that opens
// every data block in a pg_dump capture.
var copyHeader = regexp.MustCompile(`^COPY (.+?) \((.*)\) FROM stdin;$`)

// TableRows is one table's rows as pg_dump emitted them: the COPY header's
// column list plus every data line in dump order.
type TableRows struct {
	Columns []string
	Rows    [][]string
}

// ParsedDump is a pg_dump capture reduced to its COPY blocks, keyed by the
// table name without its schema qualifier (worldcheck's world contract is the
// `public` schema, matching the table names the provisioner records).
type ParsedDump map[string]*TableRows

// ParseDumpRows extracts the COPY blocks of a pg_dump capture. Rows are split
// on tabs exactly as pg_dump wrote them; no value is unescaped, so the diff
// reports the dump's own representation.
func ParseDumpRows(dump string) ParsedDump {
	parsed := ParsedDump{}
	var current *TableRows
	for _, line := range strings.Split(dump, "\n") {
		if current != nil {
			if line == `\.` {
				current = nil
				continue
			}
			current.Rows = append(current.Rows, strings.Split(line, "\t"))
			continue
		}
		m := copyHeader.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		cols := strings.Split(m[2], ", ")
		current = &TableRows{Columns: cols}
		parsed[shortTableName(m[1])] = current
	}
	return parsed
}

// SchemaDigest hashes everything in a pg_dump capture that is not row content:
// DDL, indexes, constraints, sequence definitions and the SET boilerplate.
// Sequence *values* (the `setval` calls) are data, not schema, so they are
// excluded — otherwise a sequence-only data change would be mislabelled as a
// schema change. Comment lines, blank lines and the per-dump `\restrict` tokens
// are dropped and the remainder sorted, reusing ComputeFingerprint's
// normalisation so both digests ignore dump ordering.
func SchemaDigest(dump string) string {
	kept := make([]string, 0, 64)
	inRows := false
	for _, line := range strings.Split(dump, "\n") {
		trimmed := strings.TrimSpace(line)
		if inRows {
			if trimmed == `\.` {
				inRows = false
			}
			continue
		}
		if copyHeader.MatchString(trimmed) {
			inRows = true
			continue
		}
		if trimmed == "" || strings.HasPrefix(trimmed, "--") ||
			strings.HasPrefix(trimmed, `\restrict`) || strings.HasPrefix(trimmed, `\unrestrict`) ||
			strings.HasPrefix(trimmed, "SELECT pg_catalog.setval") {
			continue
		}
		kept = append(kept, trimmed)
	}
	sort.Strings(kept)
	sum := sha256.Sum256([]byte(strings.Join(kept, "\n")))
	return hex.EncodeToString(sum[:])
}

// DiffRows compares two parsed dumps and returns the row deltas. Rows are
// identified by the primary-key columns of their table (mechanical: read from
// the database catalog). A table with no primary key falls back to identifying
// a row by its whole content, so an edit to such a table is reported as one
// deletion plus one insertion rather than an update — the mechanical reading of
// a table that offers no identity.
func DiffRows(before, after ParsedDump, primaryKeys map[string][]string) RowDeltas {
	var deltas RowDeltas
	for _, table := range unionTables(before, after) {
		b, a := before[table], after[table]
		key := rowKeyFunc(columnsOf(a, b), primaryKeys[table])
		beforeRows := indexRows(b, key)
		afterRows := indexRows(a, key)

		for _, k := range sortedKeys(afterRows) {
			row := afterRows[k]
			prev, existed := beforeRows[k]
			switch {
			case !existed:
				deltas.Inserted = append(deltas.Inserted, formatRow(table, key.label(k), row))
			case strings.Join(prev, "\t") != strings.Join(row, "\t"):
				deltas.Updated = append(deltas.Updated,
					formatRow(table, key.label(k), prev)+"  ->  "+strings.Join(row, " | "))
			}
		}
		for _, k := range sortedKeys(beforeRows) {
			if _, stillThere := afterRows[k]; !stillThere {
				deltas.Deleted = append(deltas.Deleted, formatRow(table, key.label(k), beforeRows[k]))
			}
		}
	}
	sort.Strings(deltas.Inserted)
	sort.Strings(deltas.Updated)
	sort.Strings(deltas.Deleted)
	return deltas
}

// rowIdentity maps a data row to the string a diff keys it by, and renders that
// key for humans.
type rowIdentity struct {
	indexes []int
	columns []string
}

// label renders the key of one row, e.g. `id=7` or `id=7, project_id=2`.
func (r rowIdentity) label(key string) string {
	parts := strings.Split(key, "\x1f")
	labels := make([]string, 0, len(parts))
	for i, p := range parts {
		col := "?"
		if i < len(r.columns) {
			col = r.columns[i]
		}
		labels = append(labels, col+"="+p)
	}
	return strings.Join(labels, ", ")
}

// rowKeyFunc resolves which column positions identify a row of the table. With
// no primary key the whole row is the identity (indexes nil).
func rowKeyFunc(columns, pkColumns []string) rowIdentity {
	if len(pkColumns) == 0 {
		return rowIdentity{}
	}
	cols := make([]string, 0, len(pkColumns))
	indexes := make([]int, 0, len(pkColumns))
	for _, want := range pkColumns {
		for i, have := range columns {
			if have == want {
				cols = append(cols, want)
				indexes = append(indexes, i)
				break
			}
		}
	}
	if len(indexes) == 0 {
		return rowIdentity{}
	}
	return rowIdentity{indexes: indexes, columns: cols}
}

// columnsOf returns the column list of a table from whichever dump carries it.
// The two dumps of one world agree on the columns of an unchanged table, and a
// schema change that alters them makes the identity fall back to whole-row
// content for that table, which reports the change as a delete plus an insert.
func columnsOf(after, before *TableRows) []string {
	if after != nil {
		return after.Columns
	}
	if before != nil {
		return before.Columns
	}
	return nil
}

func indexRows(rows *TableRows, key rowIdentity) map[string][]string {
	if rows == nil {
		return map[string][]string{}
	}
	indexed := make(map[string][]string, len(rows.Rows))
	for _, row := range rows.Rows {
		indexed[key.of(row)] = row
	}
	return indexed
}

// of renders a row's key. With no identity (no primary key) the key is the whole
// row content, so only byte-identical rows collide.
func (r rowIdentity) of(row []string) string {
	if len(r.indexes) == 0 {
		return strings.Join(row, "\t")
	}
	parts := make([]string, 0, len(r.indexes))
	for _, i := range r.indexes {
		if i < len(row) {
			parts = append(parts, row[i])
		} else {
			parts = append(parts, "")
		}
	}
	return strings.Join(parts, "\x1f")
}

func formatRow(table, keyLabel string, row []string) string {
	return fmt.Sprintf("%s[%s] = %s", table, keyLabel, strings.Join(row, " | "))
}

func unionTables(a, b ParsedDump) []string {
	seen := map[string]bool{}
	for t := range a {
		seen[t] = true
	}
	for t := range b {
		seen[t] = true
	}
	tables := make([]string, 0, len(seen))
	for t := range seen {
		tables = append(tables, t)
	}
	sort.Strings(tables)
	return tables
}

func sortedKeys(m map[string][]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// shortTableName drops the schema qualifier pg_dump writes, so `public.tasks`
// is reported as `tasks` — the same name the provisioner records in the
// manifest's table list.
func shortTableName(qualified string) string {
	if i := strings.LastIndex(qualified, "."); i >= 0 {
		return qualified[i+1:]
	}
	return qualified
}
