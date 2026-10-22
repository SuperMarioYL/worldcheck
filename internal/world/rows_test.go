package world

import (
	"strings"
	"testing"
)

// The two captures below are one seeded world before and after an agent bent it
// to green: a row updated, a row inserted, a row deleted, and an index added.
// They are hand-written to the shape pg_dump emits, including the per-dump
// `\restrict` token and the sequence `setval` line that is data, not schema.
const (
	dumpBefore = `--
-- PostgreSQL database dump
--

\restrict t0kEnA

SET statement_timeout = 0;
SELECT pg_catalog.set_config('search_path', '', false);

CREATE TABLE public.tasks (
    id integer NOT NULL,
    project_id integer NOT NULL,
    title text NOT NULL,
    done boolean NOT NULL
);

ALTER TABLE ONLY public.tasks
    ADD CONSTRAINT tasks_pkey PRIMARY KEY (id);

COPY public.tasks (id, project_id, title, done) FROM stdin;
1	1	seed the world	t
2	1	checkpoint before verify	f
3	2	diff row mutations	f
\.

SELECT pg_catalog.setval('public.tasks_id_seq', 3, true);

\unrestrict t0kEnA
`

	dumpAfter = `--
-- PostgreSQL database dump
--

\restrict oThErT0kEn

SET statement_timeout = 0;
SELECT pg_catalog.set_config('search_path', '', false);

CREATE TABLE public.tasks (
    id integer NOT NULL,
    project_id integer NOT NULL,
    title text NOT NULL,
    done boolean NOT NULL
);

ALTER TABLE ONLY public.tasks
    ADD CONSTRAINT tasks_pkey PRIMARY KEY (id);

CREATE INDEX idx_agent ON public.tasks USING btree (project_id);

COPY public.tasks (id, project_id, title, done) FROM stdin;
1	1	seed the world	t
3	2	diff row mutations	t
4	2	bend the world to green	t
\.

SELECT pg_catalog.setval('public.tasks_id_seq', 4, true);

\unrestrict oThErT0kEn
`
)

func TestParseDumpRows(t *testing.T) {
	parsed := ParseDumpRows(dumpBefore)
	tasks, ok := parsed["tasks"]
	if !ok {
		t.Fatalf("no tasks table parsed; got %v", parsed)
	}
	if len(tasks.Columns) != 4 || tasks.Columns[0] != "id" || tasks.Columns[3] != "done" {
		t.Errorf("columns = %v", tasks.Columns)
	}
	if len(tasks.Rows) != 3 {
		t.Fatalf("rows = %d, want 3", len(tasks.Rows))
	}
	if strings.Join(tasks.Rows[0], "|") != "1|1|seed the world|t" {
		t.Errorf("first row = %v", tasks.Rows[0])
	}
}

func TestParseDumpRowsMultipleTables(t *testing.T) {
	dump := "COPY public.users (id) FROM stdin;\n1\n\\.\nCOPY public.tasks (id) FROM stdin;\n7\n\\.\n"
	parsed := ParseDumpRows(dump)
	if len(parsed) != 2 || len(parsed["users"].Rows) != 1 || len(parsed["tasks"].Rows) != 1 {
		t.Fatalf("parsed = %v", parsed)
	}
	if parsed["tasks"].Rows[0][0] != "7" {
		t.Errorf("tasks row = %v", parsed["tasks"].Rows)
	}
}

func TestSchemaDigestIgnoresRowsAndSequenceValues(t *testing.T) {
	// The data changed between the two captures, and so did the sequence value,
	// but the schema did not: only the index in dumpAfter is a schema change.
	withoutIndex := strings.Replace(dumpAfter, "CREATE INDEX idx_agent ON public.tasks USING btree (project_id);\n\n", "", 1)
	if SchemaDigest(dumpBefore) != SchemaDigest(withoutIndex) {
		t.Errorf("row and setval changes must not move the schema digest:\n%q\n%q",
			SchemaDigest(dumpBefore), SchemaDigest(withoutIndex))
	}
	if SchemaDigest(dumpBefore) == SchemaDigest(dumpAfter) {
		t.Fatal("an added index must move the schema digest")
	}
}

func TestDiffRowsReportsInsertUpdateDelete(t *testing.T) {
	deltas := DiffRows(ParseDumpRows(dumpBefore), ParseDumpRows(dumpAfter), map[string][]string{"tasks": {"id"}})

	if len(deltas.Inserted) != 1 || !strings.Contains(deltas.Inserted[0], "tasks[id=4]") {
		t.Errorf("inserted = %v", deltas.Inserted)
	}
	if len(deltas.Deleted) != 1 || !strings.Contains(deltas.Deleted[0], "tasks[id=2]") {
		t.Errorf("deleted = %v", deltas.Deleted)
	}
	if len(deltas.Updated) != 1 {
		t.Fatalf("updated = %v", deltas.Updated)
	}
	if !strings.Contains(deltas.Updated[0], "tasks[id=3]") || !strings.Contains(deltas.Updated[0], "->") {
		t.Errorf("an updated row must name its key and show both states, got %q", deltas.Updated[0])
	}
	if !strings.Contains(deltas.Updated[0], "diff row mutations | f") ||
		!strings.Contains(deltas.Updated[0], "diff row mutations | t") {
		t.Errorf("the update must show the before and after content, got %q", deltas.Updated[0])
	}
}

func TestDiffRowsWithoutPrimaryKeyReportsDeletePlusInsert(t *testing.T) {
	// A table with no primary key offers no row identity, so an edited row reads
	// as one deletion plus one insertion — the mechanical reading, not a guess.
	deltas := DiffRows(ParseDumpRows(dumpBefore), ParseDumpRows(dumpAfter), nil)
	if len(deltas.Updated) != 0 {
		t.Errorf("a keyless table must not report updates, got %v", deltas.Updated)
	}
	if len(deltas.Inserted) != 2 || len(deltas.Deleted) != 2 {
		t.Errorf("inserted = %v, deleted = %v (want 2 and 2: row 3 edited, row 4 added, row 2 removed)", deltas.Inserted, deltas.Deleted)
	}
}

func TestDiffRowsIgnoresUnchangedTables(t *testing.T) {
	same := ParseDumpRows(dumpBefore)
	deltas := DiffRows(same, ParseDumpRows(dumpBefore), map[string][]string{"tasks": {"id"}})
	if len(deltas.Inserted)+len(deltas.Updated)+len(deltas.Deleted) != 0 {
		t.Errorf("an unchanged world must produce no deltas, got %+v", deltas)
	}
}

func TestDiffRowsReportsATableThatAppeared(t *testing.T) {
	before := ParseDumpRows("COPY public.users (id) FROM stdin;\n1\n\\.\n")
	after := ParseDumpRows("COPY public.users (id) FROM stdin;\n1\n\\.\nCOPY public.tasks (id) FROM stdin;\n1\n2\n\\.\n")
	deltas := DiffRows(before, after, map[string][]string{"users": {"id"}, "tasks": {"id"}})
	if len(deltas.Inserted) != 2 || len(deltas.Deleted) != 0 || len(deltas.Updated) != 0 {
		t.Errorf("a new table's rows must all be insertions, got %+v", deltas)
	}
	if !strings.Contains(deltas.Inserted[0], "tasks[id=1]") || !strings.Contains(deltas.Inserted[1], "tasks[id=2]") {
		t.Errorf("inserted = %v", deltas.Inserted)
	}
}
