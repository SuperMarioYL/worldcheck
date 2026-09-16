package world

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// skipWithoutDocker guards the integration test so `go test ./...` stays green
// on machines without a Docker daemon (CI runners have one).
func skipWithoutDocker(t *testing.T) {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping docker integration test in -short mode")
	}
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker CLI not on PATH")
	}
	if out, err := exec.Command("docker", "info", "--format", "{{.ServerVersion}}").CombinedOutput(); err != nil {
		t.Skipf("docker daemon unavailable: %v (%s)", err, strings.TrimSpace(string(out)))
	}
}

// TestProvisionRealPostgresStarterWorld runs the bundled world for real: it is
// the m1 acceptance test — provision, then edit seed.sql, re-run up, and
// observe the fingerprint change on an actual postgres:16-alpine container.
func TestProvisionRealPostgresStarterWorld(t *testing.T) {
	skipWithoutDocker(t)

	worldDir := filepath.Join(t.TempDir(), "postgres-starter")
	if err := os.CopyFS(worldDir, os.DirFS("../../worlds/postgres-starter")); err != nil {
		t.Fatalf("copying bundled world: %v", err)
	}

	// The test leaves its compose project (container + volume) running; bring
	// it down after the test so `go test ./...` leaves no wc-* resources
	// behind. Best-effort: an error here must not mask the test result.
	project := composeProjectName(filepath.Base(worldDir))
	t.Cleanup(func() {
		_, _ = ExecRunner{}.Run(context.Background(), worldDir, nil,
			"docker", "compose", "-p", project, "down", "-v", "--remove-orphans")
	})

	p := &Provisioner{}
	ctx := context.Background()

	first, err := p.Provision(ctx, ProvisionOptions{WorldDir: worldDir, ToolVersion: "test"})
	if err != nil {
		t.Fatalf("Provision: %v", err)
	}
	if first.ComposeProject == "" || first.Fingerprint.Value == "" {
		t.Fatalf("incomplete manifest: %+v", first)
	}
	if len(first.Tables) == 0 {
		t.Fatal("seeded tables must be recorded")
	}
	for _, want := range []string{"users", "projects", "tasks"} {
		found := false
		for _, tbl := range first.Tables {
			if tbl.Name == want && tbl.Rows > 0 {
				found = true
			}
		}
		if !found {
			t.Errorf("table %s missing or empty in manifest: %+v", want, first.Tables)
		}
	}

	second, err := p.Provision(ctx, ProvisionOptions{WorldDir: worldDir, ToolVersion: "test"})
	if err != nil {
		t.Fatalf("re-Provision without seed edit: %v", err)
	}
	if first.Fingerprint.Value != second.Fingerprint.Value {
		// Guards the whole pipeline against nondeterminism (e.g. pg_dump
		// emitting per-dump `\restrict` tokens into the fingerprint).
		t.Fatalf("same seed must reproduce the same fingerprint: %s vs %s",
			first.Fingerprint.Value, second.Fingerprint.Value)
	}

	seedPath := filepath.Join(worldDir, "seed.sql")
	original, err := os.ReadFile(seedPath)
	if err != nil {
		t.Fatal(err)
	}
	edited := string(original) + "\nUPDATE tasks SET done = true WHERE project_id = 2;\n"
	if err := os.WriteFile(seedPath, []byte(edited), 0o644); err != nil {
		t.Fatal(err)
	}

	third, err := p.Provision(ctx, ProvisionOptions{WorldDir: worldDir, ToolVersion: "test"})
	if err != nil {
		t.Fatalf("re-Provision after seed edit: %v", err)
	}
	if second.Fingerprint.Value == third.Fingerprint.Value {
		t.Fatal("fingerprint must change after seed.sql is edited")
	}
	reloaded, err := LoadManifest(worldDir)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.Fingerprint.Value != third.Fingerprint.Value {
		t.Error("manifest on disk must reflect the latest provision")
	}
}
