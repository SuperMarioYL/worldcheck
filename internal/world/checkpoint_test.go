package world

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// dbContainerName is the container identity the fake compose project reports.
const (
	dbContainerName = "wc-postgres-starter-db-1"
	dbContainer     = dbContainerName + " cid1\n"
)

func TestCreateCheckpointRecordsBaselineAndSnapshot(t *testing.T) {
	worldDir := newTestWorld(t)
	fake := &fakeRunner{
		containers: dbContainer,
		diffs:      map[string]string{"cid1": "C /run\nA /run/postgresql/.s.PGSQL.5432\nC /tmp\n"},
		dumps:      []string{dumpBefore},
	}

	cp, err := CreateCheckpoint(context.Background(), fake, worldDir, "t1")
	if err != nil {
		t.Fatalf("CreateCheckpoint: %v", err)
	}
	if cp.Label != "t1" {
		t.Errorf("label = %q", cp.Label)
	}
	if len(cp.FSBaseline) != 3 {
		t.Fatalf("filesystem baseline = %+v, want 3 paths", cp.FSBaseline)
	}
	if cp.FSBaseline[0].Container != dbContainerName {
		t.Errorf("baseline must name the container, got %+v", cp.FSBaseline[0])
	}
	if cp.DBDumpSHA256 != ComputeFingerprint(dumpBefore) {
		t.Errorf("snapshot digest = %q, want the digest of the captured dump", cp.DBDumpSHA256)
	}
	if cp.CreatedAt.IsZero() {
		t.Error("checkpoint must be stamped")
	}

	// The record and the exact dump it was taken against are both on disk, so a
	// later diff compares against the checkpointed state rather than a re-dump.
	record := filepath.Join(worldDir, CheckpointsDir, "t1.json")
	if _, err := os.Stat(record); err != nil {
		t.Fatalf("checkpoint record missing: %v", err)
	}
	raw, err := os.ReadFile(filepath.Join(worldDir, CheckpointsDir, "t1.dump"))
	if err != nil {
		t.Fatalf("snapshot missing: %v", err)
	}
	if string(raw) != dumpBefore {
		t.Error("the stored snapshot must be the captured dump verbatim")
	}

	reloaded, dump, err := loadCheckpoint(worldDir, "t1")
	if err != nil {
		t.Fatalf("loadCheckpoint: %v", err)
	}
	if reloaded.Label != cp.Label || reloaded.DBDumpSHA256 != cp.DBDumpSHA256 || dump != dumpBefore {
		t.Errorf("round trip mismatch: %+v", reloaded)
	}
}

func TestCreateCheckpointRejectsUnsafeLabels(t *testing.T) {
	// The label names files inside the world directory, so anything that could
	// escape it (or produce a surprising path) must be refused before any docker
	// call happens.
	for _, label := range []string{"", ".", "..", "../escape", "a/b", ".hidden", "a b"} {
		if _, err := CreateCheckpoint(context.Background(), &fakeRunner{}, t.TempDir(), label); err == nil {
			t.Errorf("label %q must be rejected", label)
		}
	}
}

func TestCreateCheckpointRequiresARunningWorld(t *testing.T) {
	worldDir := newTestWorld(t)
	_, err := CreateCheckpoint(context.Background(), &fakeRunner{containers: ""}, worldDir, "t1")
	if err == nil || !strings.Contains(err.Error(), "no running containers") {
		t.Fatalf("want a 'no running containers' error, got %v", err)
	}
}

func TestLoadCheckpointMissing(t *testing.T) {
	_, _, err := loadCheckpoint(t.TempDir(), "nope")
	if err == nil || !strings.Contains(err.Error(), `no checkpoint "nope"`) {
		t.Fatalf("want a missing-checkpoint error, got %v", err)
	}
}
