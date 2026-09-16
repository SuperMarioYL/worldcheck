package main

import (
	"bytes"
	"errors"
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

func TestVersionFlag(t *testing.T) {
	out, err := runRoot(t, "--version")
	if err != nil {
		t.Fatalf("--version: %v", err)
	}
	if !strings.Contains(out, "worldcheck version 0.1.0") {
		t.Errorf("version output = %q", out)
	}
}

func TestUpRequiresWorldFlag(t *testing.T) {
	if _, err := runRoot(t, "up"); err == nil || !strings.Contains(err.Error(), `required flag(s) "world" not set`) {
		t.Fatalf("up without --world must fail on the missing flag, got %v", err)
	}
}

func TestStubCommandsPointAtRoadmap(t *testing.T) {
	cases := [][]string{
		{"checkpoint", "t1"},
		{"diff", "t1"},
		{"run", "--label", "t1", "--", "make", "test"},
	}
	wantMilestone := []string{"m2", "m2", "m3"}
	for i, args := range cases {
		_, err := runRoot(t, args...)
		if err == nil {
			t.Errorf("%v: stub must fail", args)
			continue
		}
		if !strings.Contains(err.Error(), wantMilestone[i]) ||
			!strings.Contains(err.Error(), "not implemented in this milestone") {
			t.Errorf("%v: error must name the milestone and the stub, got %v", args, err)
		}
		if !errors.Is(err, world.ErrNotImplemented) {
			t.Errorf("%v: stub error must wrap world.ErrNotImplemented", args)
		}
	}
}
