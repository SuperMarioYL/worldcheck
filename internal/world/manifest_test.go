package world

import (
	"os"
	"path/filepath"
	"testing"
)

func TestComputeFingerprintDeterministic(t *testing.T) {
	dump := "SET statement_timeout = 0;\nCOPY public.users (id, email) FROM stdin;\n1\tada@example.test\n\\.\n"
	if ComputeFingerprint(dump) != ComputeFingerprint(dump) {
		t.Fatal("same dump must produce the same fingerprint")
	}
}

func TestComputeFingerprintSensitiveToData(t *testing.T) {
	before := "COPY public.tasks (id, title) FROM stdin;\n1\tseed the world\n\\.\n"
	after := "COPY public.tasks (id, title) FROM stdin;\n1\tseed the world\n2\tmutated row\n\\.\n"
	if ComputeFingerprint(before) == ComputeFingerprint(after) {
		t.Fatal("a data change must change the fingerprint")
	}
}

func TestComputeFingerprintIgnoresCommentsBlankLinesAndOrder(t *testing.T) {
	a := "-- PostgreSQL database dump\nCOPY public.users (id) FROM stdin;\n1\tada\n\\.\n\nCOPY public.tasks (id) FROM stdin;\n2\tfix\n\\.\n"
	b := "-- PostgreSQL database dump\n\nCOPY public.tasks (id) FROM stdin;\n2\tfix\n\\.\nCOPY public.users (id) FROM stdin;\n1\tada\n\\.\n"
	if ComputeFingerprint(a) != ComputeFingerprint(b) {
		t.Fatal("comment/blank/line-order differences must not change the fingerprint")
	}
}

func TestComputeFingerprintIgnoresPsqlRestrictTokens(t *testing.T) {
	// Current pg_dump emits `\restrict <random token>` / `\unrestrict` per
	// dump; the token must not leak into the fingerprint.
	a := "\\restrict rAnd0mT0kEn1\nCOPY public.users (id) FROM stdin;\n1\tada\n\\.\n\\unrestrict rAnd0mT0kEn1\n"
	b := "\\restrict d1fFeReNtT0kEn2\nCOPY public.users (id) FROM stdin;\n1\tada\n\\.\n\\unrestrict d1fFeReNtT0kEn2\n"
	if ComputeFingerprint(a) != ComputeFingerprint(b) {
		t.Fatal("per-dump restrict tokens must not change the fingerprint")
	}
}

func TestSeedDigest(t *testing.T) {
	if SeedDigest(nil) == SeedDigest([]byte("CREATE TABLE t(id int);")) {
		t.Fatal("digest must depend on the seed content")
	}
}

func TestManifestRoundTrip(t *testing.T) {
	dir := t.TempDir()
	m := &Manifest{
		Version:        1,
		World:          "postgres-starter",
		WorldDir:       dir,
		ComposeProject: "wc-postgres-starter",
		ToolVersion:    "9.9.9-test",
		Services:       []Service{{Name: "db", Image: "postgres:16-alpine"}},
		Seed:           SeedInfo{File: "seed.sql", Database: "app", SHA256: "abc"},
		Tables:         []Table{{Name: "users", Rows: 4}},
		Fingerprint:    Fingerprint{Algorithm: FingerprintAlgorithm, Value: "deadbeef"},
	}
	if err := SaveManifest(dir, m); err != nil {
		t.Fatalf("SaveManifest: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, ManifestFileName)); err != nil {
		t.Fatalf("manifest file missing: %v", err)
	}
	got, err := LoadManifest(dir)
	if err != nil {
		t.Fatalf("LoadManifest: %v", err)
	}
	if got.ComposeProject != m.ComposeProject || len(got.Services) != 1 ||
		got.Tables[0].Name != "users" || got.Fingerprint.Value != "deadbeef" {
		t.Fatalf("round trip mismatch: %+v", got)
	}
}

func TestLoadManifestMissing(t *testing.T) {
	if _, err := LoadManifest(t.TempDir()); err != ErrNoManifest {
		t.Fatalf("want ErrNoManifest, got %v", err)
	}
}
