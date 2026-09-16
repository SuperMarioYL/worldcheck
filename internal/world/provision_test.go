package world

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// fakeRunner records the commands Provision issues and answers them the way
// the docker CLI would, without a Docker daemon. The pg_dump answer is a
// function of the seed stdin it saw, which is exactly the causal chain the
// real daemon has: different seed -> different seeded state -> different dump.
type fakeRunner struct {
	calls       []string
	seeds       []string
	dumpForSeed func(seed string) string
	tables      string
	counts      map[string]int64
}

var countQuery = regexp.MustCompile(`SELECT count\(\*\) FROM "([^"]+)"`)

func (f *fakeRunner) Run(_ context.Context, dir string, stdin io.Reader, argv ...string) (string, error) {
	joined := strings.Join(argv, " ")
	f.calls = append(f.calls, joined)
	switch {
	case strings.Contains(joined, "down -v"):
		return "", nil
	case strings.Contains(joined, "up -d --wait"):
		return "", nil
	case strings.Contains(joined, "ON_ERROR_STOP"):
		b, err := io.ReadAll(stdin)
		if err != nil {
			return "", err
		}
		f.seeds = append(f.seeds, string(b))
		return "", nil
	case strings.Contains(joined, "pg_dump"):
		seed := ""
		if len(f.seeds) > 0 {
			seed = f.seeds[len(f.seeds)-1]
		}
		return f.dumpForSeed(seed), nil
	case strings.Contains(joined, "information_schema.tables"):
		return f.tables, nil
	case strings.Contains(joined, "count(*)"):
		m := countQuery.FindStringSubmatch(joined)
		if m == nil {
			return "", errors.New("fakeRunner: unparseable count query: " + joined)
		}
		rows, ok := f.counts[m[1]]
		if !ok {
			return "", errors.New("fakeRunner: no count stubbed for table " + m[1])
		}
		return strconv.FormatInt(rows, 10) + "\n", nil
	}
	return "", errors.New("fakeRunner: unexpected command: " + joined)
}

const (
	dumpInitial = "COPY public.tasks (id, title, done) FROM stdin;\n1\tseed the world\tt\n\\.\n"
	dumpEdited  = "COPY public.tasks (id, title, done) FROM stdin;\n1\tseed the world\tt\n7\tbend the world to green\tt\n\\.\n"
)

func newTestWorld(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	worldDir := filepath.Join(dir, "postgres-starter")
	if err := os.MkdirAll(worldDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeWorldFiles(t, worldDir, "-- initial seed\nCREATE TABLE tasks (id serial PRIMARY KEY, title text, done boolean);\nINSERT INTO tasks (title, done) VALUES ('seed the world', true);\n")
	return worldDir
}

func writeWorldFiles(t *testing.T, worldDir, seed string) {
	t.Helper()
	compose := "services:\n  db:\n    image: postgres:16-alpine\n    environment:\n      POSTGRES_USER: postgres\n      POSTGRES_PASSWORD: postgres\n      POSTGRES_DB: app\n"
	if err := os.WriteFile(filepath.Join(worldDir, "compose.yml"), []byte(compose), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(worldDir, "seed.sql"), []byte(seed), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestProvisionHappyPath(t *testing.T) {
	worldDir := newTestWorld(t)
	fake := &fakeRunner{
		dumpForSeed: func(string) string { return dumpInitial },
		tables:      "tasks\n",
		counts:      map[string]int64{"tasks": 1},
	}
	p := &Provisioner{Runner: fake}
	m, err := p.Provision(context.Background(), ProvisionOptions{WorldDir: worldDir, ToolVersion: "test"})
	if err != nil {
		t.Fatalf("Provision: %v", err)
	}

	if m.ComposeProject != "wc-postgres-starter" {
		t.Errorf("project = %q, want wc-postgres-starter", m.ComposeProject)
	}
	if len(m.Services) != 1 || m.Services[0].Name != "db" || m.Services[0].Image != "postgres:16-alpine" {
		t.Errorf("services mismatch: %+v", m.Services)
	}
	if m.Seed.Database != "app" {
		t.Errorf("seed database = %q, want app (from POSTGRES_DB)", m.Seed.Database)
	}
	if len(m.Tables) != 1 || m.Tables[0].Name != "tasks" || m.Tables[0].Rows != 1 {
		t.Errorf("tables mismatch: %+v", m.Tables)
	}
	if m.Fingerprint.Value != ComputeFingerprint(dumpInitial) {
		t.Errorf("fingerprint must hash the pg_dump capture")
	}
	if m.Fingerprint.Algorithm != FingerprintAlgorithm {
		t.Errorf("fingerprint algorithm = %q", m.Fingerprint.Algorithm)
	}

	// The seed the fake saw must be the file's exact content, proving stdin
	// (not a copied file) carries the seed into psql.
	onDisk, err := os.ReadFile(filepath.Join(worldDir, "seed.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if len(fake.seeds) != 1 || fake.seeds[0] != string(onDisk) {
		t.Errorf("psql stdin must be the seed file content")
	}

	// Command sequence: reset volumes, start with health waiting, seed, dump.
	joined := strings.Join(fake.calls, "\n")
	for _, want := range []string{"down -v", "up -d --wait", "ON_ERROR_STOP", "pg_dump", "--data-only"} {
		if !strings.Contains(joined, want) {
			t.Errorf("provision sequence missing %q; got:\n%s", want, joined)
		}
	}

	if _, err := LoadManifest(worldDir); err != nil {
		t.Errorf("manifest.json must be written into the world dir: %v", err)
	}
}

func TestProvisionSeedEditChangesFingerprint(t *testing.T) {
	// The m1 acceptance criterion: edit seed.sql, re-run up, see the
	// fingerprint change.
	worldDir := newTestWorld(t)
	fake := &fakeRunner{
		dumpForSeed: func(seed string) string {
			if strings.Contains(seed, "bend the world to green") {
				return dumpEdited
			}
			return dumpInitial
		},
		tables: "tasks\n",
		counts: map[string]int64{"tasks": 1},
	}
	p := &Provisioner{Runner: fake}

	first, err := p.Provision(context.Background(), ProvisionOptions{WorldDir: worldDir, ToolVersion: "test"})
	if err != nil {
		t.Fatalf("first Provision: %v", err)
	}

	edited := "-- initial seed\nCREATE TABLE tasks (id serial PRIMARY KEY, title text, done boolean);\nINSERT INTO tasks (title, done) VALUES ('seed the world', true);\nINSERT INTO tasks (title, done) VALUES ('bend the world to green', true);\n"
	if err := os.WriteFile(filepath.Join(worldDir, "seed.sql"), []byte(edited), 0o644); err != nil {
		t.Fatal(err)
	}
	fake.counts["tasks"] = 2

	second, err := p.Provision(context.Background(), ProvisionOptions{WorldDir: worldDir, ToolVersion: "test"})
	if err != nil {
		t.Fatalf("second Provision: %v", err)
	}
	if first.Fingerprint.Value == second.Fingerprint.Value {
		t.Fatal("re-provisioning after a seed edit must change the fingerprint")
	}
	if second.Tables[0].Rows != 2 {
		t.Errorf("rows = %d, want 2 after re-seed", second.Tables[0].Rows)
	}
	reloaded, err := LoadManifest(worldDir)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.Fingerprint.Value != second.Fingerprint.Value {
		t.Error("manifest on disk must reflect the latest provision")
	}
}

func TestProvisionContractErrors(t *testing.T) {
	t.Run("missing seed.sql", func(t *testing.T) {
		dir := t.TempDir()
		p := &Provisioner{Runner: &fakeRunner{}}
		_, err := p.Provision(context.Background(), ProvisionOptions{WorldDir: dir, ToolVersion: "test"})
		var contractErr *WorldDirContractError
		if !errors.As(err, &contractErr) || !strings.Contains(err.Error(), "seed.sql") {
			t.Fatalf("want contract error about seed.sql, got %v", err)
		}
	})
	t.Run("no db service", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "compose.yml"), []byte("services:\n  api:\n    image: nginx\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "seed.sql"), []byte("SELECT 1;\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		p := &Provisioner{Runner: &fakeRunner{}}
		_, err := p.Provision(context.Background(), ProvisionOptions{WorldDir: dir, ToolVersion: "test"})
		var contractErr *WorldDirContractError
		if !errors.As(err, &contractErr) || !strings.Contains(err.Error(), `"db"`) {
			t.Fatalf("want contract error about the db service, got %v", err)
		}
	})
}

func TestComposeProjectName(t *testing.T) {
	cases := map[string]string{
		"postgres-starter": "wc-postgres-starter",
		"My World!!":       "wc-my-world",
		"  --weird--  ":    "wc-weird",
	}
	for in, want := range cases {
		if got := composeProjectName(in); got != want {
			t.Errorf("composeProjectName(%q) = %q, want %q", in, got, want)
		}
	}
}
