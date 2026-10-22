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

// TestProvisionRealWorldWithoutHealthcheck is the integration counterpart of the
// readiness gate: the documented world contract (README: a `db` Postgres service
// plus POSTGRES_USER/POSTGRES_DB) does not mention a `healthcheck:`, so a world
// that omits the bundled one must provision exactly like the bundled world
// rather than racing Postgres startup.
func TestProvisionRealWorldWithoutHealthcheck(t *testing.T) {
	skipWithoutDocker(t)

	worldDir := filepath.Join(t.TempDir(), "postgres-starter-nohc")
	if err := os.CopyFS(worldDir, os.DirFS("../../worlds/postgres-starter")); err != nil {
		t.Fatalf("copying bundled world: %v", err)
	}
	composePath := filepath.Join(worldDir, "compose.yml")
	raw, err := os.ReadFile(composePath)
	if err != nil {
		t.Fatal(err)
	}
	stripped := stripHealthcheck(t, string(raw))
	if stripped == string(raw) {
		t.Fatal("fixture no longer contains a healthcheck block to strip")
	}
	if err := os.WriteFile(composePath, []byte(stripped), 0o644); err != nil {
		t.Fatal(err)
	}

	project := composeProjectName(filepath.Base(worldDir))
	t.Cleanup(func() {
		_, _ = ExecRunner{}.Run(context.Background(), worldDir, nil,
			"docker", "compose", "-p", project, "down", "-v", "--remove-orphans")
	})

	p := &Provisioner{}
	m, err := p.Provision(context.Background(), ProvisionOptions{WorldDir: worldDir, ToolVersion: "test"})
	if err != nil {
		t.Fatalf("a world without a healthcheck must still provision: %v", err)
	}
	if len(m.Tables) == 0 {
		t.Fatalf("seeded tables must be recorded: %+v", m)
	}
	if m.Fingerprint.Value == "" {
		t.Fatal("a fingerprint must be recorded")
	}
}

// TestCheckpointAndDiffRealWorld is the m2 acceptance test: checkpoint a live
// world, have an agent bend one seeded row and drop one file, and require the
// diff to report exactly that row change — plus the file — and no row insertions
// or deletions.
func TestCheckpointAndDiffRealWorld(t *testing.T) {
	skipWithoutDocker(t)

	worldDir := filepath.Join(t.TempDir(), "postgres-starter")
	if err := os.CopyFS(worldDir, os.DirFS("../../worlds/postgres-starter")); err != nil {
		t.Fatalf("copying bundled world: %v", err)
	}
	project := composeProjectName(filepath.Base(worldDir))
	t.Cleanup(func() {
		_, _ = ExecRunner{}.Run(context.Background(), worldDir, nil,
			"docker", "compose", "-p", project, "down", "-v", "--remove-orphans")
	})

	ctx := context.Background()
	p := &Provisioner{}
	if _, err := p.Provision(ctx, ProvisionOptions{WorldDir: worldDir, ToolVersion: "test"}); err != nil {
		t.Fatalf("Provision: %v", err)
	}
	if _, err := CreateCheckpoint(ctx, ExecRunner{}, worldDir, "t1"); err != nil {
		t.Fatalf("CreateCheckpoint: %v", err)
	}

	clean, err := DiffAgainst(ctx, ExecRunner{}, worldDir, "t1")
	if err != nil {
		t.Fatalf("DiffAgainst on a clean world: %v", err)
	}
	if len(clean.DBRows.Inserted)+len(clean.DBRows.Updated)+len(clean.DBRows.Deleted) != 0 {
		t.Errorf("a world nothing touched must show no row mutations: %+v", clean.DBRows)
	}
	if clean.SchemaChanged {
		t.Error("a world nothing touched must not report a schema change")
	}

	// The agent bends the world: one seeded row, plus one file it leaves behind.
	runner := ExecRunner{}
	psql := func(sql string) {
		t.Helper()
		if _, err := runner.Run(ctx, worldDir, nil,
			"docker", "compose", "-p", project, "exec", "-T", dbServiceName,
			"psql", "-U", "postgres", "-d", "app", "-q", "-c", sql); err != nil {
			t.Fatalf("agent mutation %q: %v", sql, err)
		}
	}
	psql("UPDATE tasks SET done = true WHERE id = 3;")
	if _, err := runner.Run(ctx, worldDir, nil,
		"docker", "compose", "-p", project, "exec", "-T", dbServiceName,
		"sh", "-c", "echo bent > /tmp/agent-note"); err != nil {
		t.Fatalf("agent file mutation: %v", err)
	}

	diff, err := DiffAgainst(ctx, ExecRunner{}, worldDir, "t1")
	if err != nil {
		t.Fatalf("DiffAgainst after the mutation: %v", err)
	}
	if len(diff.DBRows.Updated) != 1 || !strings.Contains(diff.DBRows.Updated[0], "tasks[id=3]") {
		t.Errorf("the bent row must be the one update reported, got %v", diff.DBRows.Updated)
	}
	if len(diff.DBRows.Inserted) != 0 || len(diff.DBRows.Deleted) != 0 {
		t.Errorf("no rows were added or removed, got inserted=%v deleted=%v", diff.DBRows.Inserted, diff.DBRows.Deleted)
	}
	if diff.SchemaChanged {
		t.Error("the schema was not touched")
	}
	// The file the agent dropped must be reported. The container's diff also
	// carries Postgres's own writes (WAL and friends) between the two captures,
	// so this asserts the agent's file is present, not that it is the only one.
	found := false
	for _, c := range diff.FS.Added {
		if strings.HasSuffix(c.Path, "/agent-note") {
			found = true
		}
	}
	if !found {
		t.Errorf("the file the agent dropped must be reported, got %+v", diff.FS)
	}
	if diff.Reconcile == "" || strings.Contains(diff.Reconcile, "no mutations") {
		t.Errorf("reconcile summary = %q", diff.Reconcile)
	}
}

// stripHealthcheck removes the bundled world's `healthcheck:` block, leaving a
// compose file that satisfies the documented contract without one.
func stripHealthcheck(t *testing.T, compose string) string {
	t.Helper()
	lines := strings.Split(compose, "\n")
	out := make([]string, 0, len(lines))
	skipping := false
	for _, line := range lines {
		if strings.HasPrefix(line, "    healthcheck:") {
			skipping = true
			continue
		}
		if skipping {
			if strings.HasPrefix(line, "    ") || strings.TrimSpace(line) == "" {
				continue
			}
			skipping = false
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}
