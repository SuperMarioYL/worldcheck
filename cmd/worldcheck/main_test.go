package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/SuperMarioYL/worldcheck/internal/world"
)

func runRoot(t *testing.T, args ...string) (string, error) {
	t.Helper()
	root := newRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs(args)
	err := root.Execute()
	return out.String(), err
}

// repoVersion reads the VERSION file at the repo root, the release source of
// truth the binary is stamped from.
func repoVersion(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "VERSION"))
	if err != nil {
		t.Fatalf("reading VERSION: %v", err)
	}
	return strings.TrimSpace(string(raw))
}

func TestVersionFlag(t *testing.T) {
	out, err := runRoot(t, "--version")
	if err != nil {
		t.Fatalf("--version: %v", err)
	}
	if want := "worldcheck version " + repoVersion(t); !strings.Contains(out, want) {
		t.Errorf("version output = %q, want it to contain %q", out, want)
	}
}

// TestVersionLockstep guards the surfaces that must carry one version: the
// VERSION file, the version the binary reports, the recorded demo run and the
// product copy that quotes it. Drift between them is caught here rather than by
// a reader noticing.
func TestVersionLockstep(t *testing.T) {
	v := repoVersion(t)
	if version != v {
		t.Errorf("cmd/worldcheck version = %q, VERSION = %q", version, v)
	}
	quoted := "(worldcheck " + v + ")"
	for _, name := range []string{"README.md", "README.en.md", "web/site.json", "docs/demo-results.json"} {
		raw, err := os.ReadFile(filepath.Join("..", "..", name))
		if err != nil {
			t.Fatalf("reading %s: %v", name, err)
		}
		if !strings.Contains(string(raw), quoted) {
			t.Errorf("%s does not quote the recorded run as %q", name, quoted)
		}
	}

	raw, err := os.ReadFile(filepath.Join("..", "..", "docs", "demo-results.json"))
	if err != nil {
		t.Fatal(err)
	}
	var demo struct {
		ImplementationVersion string `json:"implementation_version"`
	}
	if err := json.Unmarshal(raw, &demo); err != nil {
		t.Fatalf("parsing docs/demo-results.json: %v", err)
	}
	if demo.ImplementationVersion != v {
		t.Errorf("docs/demo-results.json implementation_version = %q, VERSION = %q", demo.ImplementationVersion, v)
	}
}

func TestUpRequiresWorldFlag(t *testing.T) {
	if _, err := runRoot(t, "up"); err == nil || !strings.Contains(err.Error(), `required flag(s) "world" not set`) {
		t.Fatalf("up without --world must fail on the missing flag, got %v", err)
	}
}

func TestCheckpointAndDiffRequireWorldFlag(t *testing.T) {
	for _, args := range [][]string{{"checkpoint", "t1"}, {"diff", "t1"}} {
		if _, err := runRoot(t, args...); err == nil || !strings.Contains(err.Error(), `required flag(s) "world" not set`) {
			t.Fatalf("%v without --world must fail on the missing flag, got %v", args, err)
		}
	}
}

// TestCheckpointAndDiffAreNoLongerStubs: m2 ships in this version, so only `run`
// (m3) still reports the roadmap note.
func TestRunIsTheOnlyRemainingStub(t *testing.T) {
	_, err := runRoot(t, "run", "--label", "t1", "--", "make", "test")
	if err == nil {
		t.Fatal("run must still be a stub")
	}
	if !strings.Contains(err.Error(), "m3") || !strings.Contains(err.Error(), "not implemented in this milestone") {
		t.Errorf("run stub error must name the milestone, got %v", err)
	}
	if !errors.Is(err, world.ErrNotImplemented) {
		t.Errorf("run stub error must wrap world.ErrNotImplemented, got %v", err)
	}
}

func TestCheckpointReportsAMissingWorld(t *testing.T) {
	_, err := runRoot(t, "checkpoint", "t1", "--world", t.TempDir())
	if err == nil {
		t.Fatal("checkpoint against a directory that is not a world must fail")
	}
	var contractErr *world.WorldDirContractError
	if !errors.As(err, &contractErr) {
		t.Errorf("want a WorldDirContractError, got %v", err)
	}
}
