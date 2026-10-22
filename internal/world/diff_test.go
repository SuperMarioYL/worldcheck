package world

import (
	"context"
	"strings"
	"testing"
)

// checkpointedWorld writes a checkpoint whose baseline is the state a seeded
// world has right after `up`, so a diff test only has to describe what the
// agent did afterwards.
func checkpointedWorld(t *testing.T, label string) string {
	t.Helper()
	worldDir := newTestWorld(t)
	cp := &Checkpoint{
		Label: label,
		FSBaseline: []FileChange{
			{Container: dbContainerName, Kind: "C", Path: "/run"},
			{Container: dbContainerName, Kind: "A", Path: "/tmp/baseline-file"},
		},
		DBDumpSHA256: ComputeFingerprint(dumpBefore),
	}
	if err := saveCheckpoint(worldDir, cp, dumpBefore); err != nil {
		t.Fatal(err)
	}
	return worldDir
}

func TestDiffAgainstReportsFilesystemRowsAndSchema(t *testing.T) {
	worldDir := checkpointedWorld(t, "t1")
	// After the checkpoint the agent wrote a file, edited /tmp, undid the
	// baseline file, updated a row, added a row, deleted a row and added an
	// index. /run was already changed at checkpoint time and stays that way, so
	// it must not be reported again.
	fake := &fakeRunner{
		containers:     dbContainer,
		diffs:          map[string]string{"cid1": "C /run\nC /tmp\nA /tmp/agent-wrote-this\n"},
		dumps:          []string{dumpAfter},
		primaryKeyRows: "tasks|id\n",
	}

	diff, err := DiffAgainst(context.Background(), fake, worldDir, "t1")
	if err != nil {
		t.Fatalf("DiffAgainst: %v", err)
	}
	if diff.Label != "t1" {
		t.Errorf("label = %q", diff.Label)
	}

	if len(diff.FS.Added) != 1 || diff.FS.Added[0].Path != "/tmp/agent-wrote-this" {
		t.Errorf("added = %+v", diff.FS.Added)
	}
	if len(diff.FS.Changed) != 1 || diff.FS.Changed[0].Path != "/tmp" {
		t.Errorf("changed = %+v", diff.FS.Changed)
	}
	if len(diff.FS.Deleted) != 1 || diff.FS.Deleted[0].Path != "/tmp/baseline-file" {
		t.Errorf("a checkpoint-time mutation that was undone must read as deleted, got %+v", diff.FS.Deleted)
	}

	if len(diff.DBRows.Inserted) != 1 || !strings.Contains(diff.DBRows.Inserted[0], "tasks[id=4]") {
		t.Errorf("inserted = %v", diff.DBRows.Inserted)
	}
	if len(diff.DBRows.Updated) != 1 || !strings.Contains(diff.DBRows.Updated[0], "tasks[id=3]") {
		t.Errorf("updated = %v", diff.DBRows.Updated)
	}
	if len(diff.DBRows.Deleted) != 1 || !strings.Contains(diff.DBRows.Deleted[0], "tasks[id=2]") {
		t.Errorf("deleted = %v", diff.DBRows.Deleted)
	}

	if !diff.SchemaChanged {
		t.Error("the added index must be reported as a schema change")
	}
	if diff.Reconcile == "" || strings.Contains(diff.Reconcile, "no mutations") {
		t.Errorf("reconcile summary = %q", diff.Reconcile)
	}
}

func TestDiffAgainstCleanWorldReportsNoMutations(t *testing.T) {
	worldDir := checkpointedWorld(t, "t1")
	fake := &fakeRunner{
		containers:     dbContainer,
		diffs:          map[string]string{"cid1": "C /run\nA /tmp/baseline-file\n"},
		dumps:          []string{dumpBefore},
		primaryKeyRows: "tasks|id\n",
	}

	diff, err := DiffAgainst(context.Background(), fake, worldDir, "t1")
	if err != nil {
		t.Fatalf("DiffAgainst: %v", err)
	}
	if diff.Reconcile != "no mutations since the checkpoint" {
		t.Errorf("reconcile = %q, want the no-mutations summary", diff.Reconcile)
	}
	if diff.SchemaChanged {
		t.Error("an unchanged schema must not be reported as changed")
	}
	if len(diff.FS.Added)+len(diff.FS.Changed)+len(diff.FS.Deleted) != 0 {
		t.Errorf("filesystem = %+v", diff.FS)
	}
	if len(diff.DBRows.Inserted)+len(diff.DBRows.Updated)+len(diff.DBRows.Deleted) != 0 {
		t.Errorf("rows = %+v", diff.DBRows)
	}
}

// TestDiffAgainstSeesASchemaOnlyMutation is the regression test for the audit's
// blind spot: an agent that bends the world to green by altering the schema
// without touching a row must not be reported as having changed nothing.
func TestDiffAgainstSeesASchemaOnlyMutation(t *testing.T) {
	worldDir := checkpointedWorld(t, "t1")
	schemaOnly := strings.Replace(dumpAfter,
		"1	1	seed the world	t\n3	2	diff row mutations	t\n4	2	bend the world to green	t\n",
		"1	1	seed the world	t\n2	1	checkpoint before verify	f\n3	2	diff row mutations	f\n", 1)
	if schemaOnly == dumpAfter {
		t.Fatal("test fixture no longer contains the expected row block")
	}
	fake := &fakeRunner{
		containers:     dbContainer,
		diffs:          map[string]string{"cid1": "C /run\nA /tmp/baseline-file\n"},
		dumps:          []string{schemaOnly},
		primaryKeyRows: "tasks|id\n",
	}

	diff, err := DiffAgainst(context.Background(), fake, worldDir, "t1")
	if err != nil {
		t.Fatalf("DiffAgainst: %v", err)
	}
	if len(diff.DBRows.Inserted)+len(diff.DBRows.Updated)+len(diff.DBRows.Deleted) != 0 {
		t.Errorf("rows = %+v, want none for a schema-only mutation", diff.DBRows)
	}
	if !diff.SchemaChanged {
		t.Fatal("a schema-only mutation must be reported, not read as nothing changed")
	}
	if diff.Reconcile == "no mutations since the checkpoint" {
		t.Errorf("reconcile = %q, want the schema change reported", diff.Reconcile)
	}
}

func TestDiffAgainstWithoutCheckpoint(t *testing.T) {
	worldDir := newTestWorld(t)
	_, err := DiffAgainst(context.Background(), &fakeRunner{}, worldDir, "t1")
	if err == nil || !strings.Contains(err.Error(), `no checkpoint "t1"`) {
		t.Fatalf("want a missing-checkpoint error, got %v", err)
	}
}

func TestDiffAgainstRejectsUnsafeLabel(t *testing.T) {
	if _, err := DiffAgainst(context.Background(), &fakeRunner{}, t.TempDir(), "../escape"); err == nil {
		t.Fatal("a label that escapes the world directory must be rejected")
	}
}
