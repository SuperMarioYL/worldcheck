package report

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/SuperMarioYL/worldcheck/internal/world"
)

func sampleManifest() *world.Manifest {
	return &world.Manifest{
		World:          "postgres-starter",
		ComposeProject: "wc-postgres-starter",
		ToolVersion:    "9.9.9-test",
		Services:       []world.Service{{Name: "db", Image: "postgres:16-alpine"}},
		Seed:           world.SeedInfo{File: "seed.sql", Database: "app", SHA256: "0123456789abcdef"},
		Tables:         []world.Table{{Name: "users", Rows: 4}, {Name: "tasks", Rows: 6}},
		Fingerprint:    world.Fingerprint{Algorithm: world.FingerprintAlgorithm, Value: "f00dcafe"},
	}
}

func TestRenderStatusContainsSeededState(t *testing.T) {
	var buf bytes.Buffer
	if err := RenderStatus(&buf, sampleManifest()); err != nil {
		t.Fatalf("RenderStatus: %v", err)
	}
	out := buf.String()
	for _, want := range []string{
		"wc-postgres-starter",
		"postgres:16-alpine",
		"seed.sql",
		"0123456789ab", // digest is abbreviated in the seed line
		"users",
		"tasks",
		"f00dcafe",
		world.FingerprintAlgorithm,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("status output missing %q:\n%s", want, out)
		}
	}
}

func TestRenderFingerprintOneLine(t *testing.T) {
	var buf bytes.Buffer
	RenderFingerprint(&buf, sampleManifest())
	if got := buf.String(); !strings.HasPrefix(got, "seeded-state fingerprint: f00dcafe\n") {
		t.Errorf("unexpected fingerprint line: %q", got)
	}
}

func TestWriteHTMLIsM3Stub(t *testing.T) {
	if err := WriteHTML("worldcheck.html", &world.MutationDiff{}); !errors.Is(err, world.ErrNotImplemented) {
		t.Fatalf("WriteHTML must be an m3 stub, got %v", err)
	}
}

func TestRenderDiffPrintsMutationsAndCheckpoint(t *testing.T) {
	diff := &world.MutationDiff{
		Label: "t1",
		FS: world.FSChanges{
			Added:   []world.FileChange{{Container: "wc-db-1", Kind: "A", Path: "/tmp/agent-wrote-this"}},
			Changed: []world.FileChange{{Container: "wc-db-1", Kind: "C", Path: "/tmp"}},
		},
		DBRows: world.RowDeltas{
			Updated: []string{"tasks[id=3] = 3 | 2 | diff row mutations | f  ->  3 | 2 | diff row mutations | t"},
		},
		SchemaChanged: true,
		Reconcile:     "files 1 added / 1 changed / 0 deleted; rows 0 inserted / 1 updated / 0 deleted; schema changed",
	}
	var buf bytes.Buffer
	if err := RenderDiff(&buf, diff); err != nil {
		t.Fatalf("RenderDiff: %v", err)
	}
	out := buf.String()
	for _, want := range []string{
		"checkpoint t1",
		"schema changed",
		"ADDED",
		"/tmp/agent-wrote-this",
		"CHANGED",
		"UPDATED",
		"tasks[id=3]",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("diff output missing %q:\n%s", want, out)
		}
	}
}

func TestRenderDiffCleanWorldSaysSo(t *testing.T) {
	var buf bytes.Buffer
	if err := RenderDiff(&buf, &world.MutationDiff{Label: "t1", Reconcile: "no mutations since the checkpoint"}); err != nil {
		t.Fatalf("RenderDiff: %v", err)
	}
	if !strings.Contains(buf.String(), "no mutations since the checkpoint") {
		t.Errorf("a clean diff must say so:\n%s", buf.String())
	}
}
