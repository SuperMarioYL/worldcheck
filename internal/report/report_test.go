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
		ToolVersion:    "0.1.0",
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
