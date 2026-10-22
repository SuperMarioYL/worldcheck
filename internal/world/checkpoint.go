package world

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"time"
)

// This file is the m2 checkpoint leg of the primitive: snapshot a provisioned
// world's state — the `docker diff` filesystem baseline of every container plus
// a full pg_dump of its database — under a label, so a later diff can compare
// against it.

// CheckpointsDir is the directory, inside the world directory, where checkpoint
// records and their database snapshots live. Generated state, like
// manifest.json — it never ships with the repo.
const CheckpointsDir = "checkpoints"

// checkpointLabelRe bounds a checkpoint label to a single safe path component:
// the label names files inside the world directory, so it must not be able to
// escape it.
var checkpointLabelRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

// FileChange is one filesystem mutation as reported by `docker diff`
// (A = added, C = changed, D = deleted).
type FileChange struct {
	Container string
	Kind      string
	Path      string
}

// Checkpoint is a named snapshot of world state taken before verification:
// a filesystem baseline for every container plus a hashed Postgres dump.
type Checkpoint struct {
	Label        string       `json:"label"`
	CreatedAt    time.Time    `json:"created_at"`
	FSBaseline   []FileChange `json:"fs_baseline"`
	DBDumpSHA256 string       `json:"db_dump_sha256"`
}

// CreateCheckpoint snapshots every container in the world and the database
// under the given label. The raw dump is written beside the record, so
// DiffAgainst can compare against exactly the state that was checkpointed.
func CreateCheckpoint(ctx context.Context, r Runner, worldDir, label string) (*Checkpoint, error) {
	if !checkpointLabelRe.MatchString(label) {
		return nil, fmt.Errorf("invalid checkpoint label %q: use letters, digits, dot, dash or underscore", label)
	}
	ws, err := resolveWorld(worldDir)
	if err != nil {
		return nil, err
	}
	run := func(stdin io.Reader, argv ...string) (string, error) {
		return r.Run(ctx, worldDir, stdin, argv...)
	}
	base := []string{"docker", "compose", "-p", ws.Project}

	fs, err := containerDiffs(run, base, ws.Project)
	if err != nil {
		return nil, err
	}
	dump, err := dumpDatabase(run, base, ws.DBUser, ws.DBName)
	if err != nil {
		return nil, err
	}

	cp := &Checkpoint{
		Label:        label,
		CreatedAt:    time.Now().UTC(),
		FSBaseline:   fs,
		DBDumpSHA256: ComputeFingerprint(dump),
	}
	if err := saveCheckpoint(worldDir, cp, dump); err != nil {
		return nil, err
	}
	return cp, nil
}

// containerDiffs runs `docker diff` against every container of the compose
// project and returns the mutations, ordered by container then path. A project
// with no running container is an error rather than an empty baseline: a
// checkpoint of nothing would make every later diff vacuously clean.
func containerDiffs(run func(io.Reader, ...string) (string, error), base []string, project string) ([]FileChange, error) {
	out, err := run(nil, append(slices.Clone(base), "ps", "--format", "{{.Name}} {{.ID}}")...)
	if err != nil {
		return nil, fmt.Errorf("listing the containers of project %s: %w", project, err)
	}
	var changes []FileChange
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		name, id := fields[0], fields[1]
		diff, err := run(nil, "docker", "diff", id)
		if err != nil {
			return nil, fmt.Errorf("diffing container %s: %w", name, err)
		}
		for _, line := range strings.Split(diff, "\n") {
			kind, path, ok := strings.Cut(strings.TrimRight(line, "\r"), " ")
			if !ok || len(kind) != 1 {
				continue
			}
			changes = append(changes, FileChange{Container: name, Kind: kind, Path: path})
		}
	}
	if len(changes) == 0 && strings.TrimSpace(out) == "" {
		return nil, fmt.Errorf("no running containers in project %s; run `worldcheck up --world <dir>` first", project)
	}
	sortFileChanges(changes)
	return changes, nil
}

// dumpDatabase captures the world's database with a full pg_dump (schema plus
// data) through the db service.
func dumpDatabase(run func(io.Reader, ...string) (string, error), base []string, dbUser, dbName string) (string, error) {
	argv := append(slices.Clone(base),
		"exec", "-T", dbServiceName,
		"pg_dump", "-U", dbUser, "--no-owner", "--no-privileges", dbName,
	)
	out, err := run(nil, argv...)
	if err != nil {
		return "", fmt.Errorf("dumping database %s: %w", dbName, err)
	}
	return out, nil
}

// saveCheckpoint writes the record and its dump into the world's checkpoints
// directory.
func saveCheckpoint(worldDir string, cp *Checkpoint, dump string) error {
	dir := filepath.Join(worldDir, CheckpointsDir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cp, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if err := os.WriteFile(checkpointDumpPath(worldDir, cp.Label), []byte(dump), 0o644); err != nil {
		return err
	}
	return os.WriteFile(checkpointPath(worldDir, cp.Label), data, 0o644)
}

// loadCheckpoint reads a checkpoint record and the dump it was taken against.
func loadCheckpoint(worldDir, label string) (*Checkpoint, string, error) {
	if !checkpointLabelRe.MatchString(label) {
		return nil, "", fmt.Errorf("invalid checkpoint label %q: use letters, digits, dot, dash or underscore", label)
	}
	data, err := os.ReadFile(checkpointPath(worldDir, label))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, "", fmt.Errorf("no checkpoint %q in %s; run `worldcheck checkpoint %s --world <dir>` first",
				label, filepath.Join(worldDir, CheckpointsDir), label)
		}
		return nil, "", err
	}
	var cp Checkpoint
	if err := json.Unmarshal(data, &cp); err != nil {
		return nil, "", fmt.Errorf("parsing checkpoint %s: %w", label, err)
	}
	dump, err := os.ReadFile(checkpointDumpPath(worldDir, label))
	if err != nil {
		return nil, "", fmt.Errorf("reading the %s snapshot: %w", label, err)
	}
	return &cp, string(dump), nil
}

func checkpointPath(worldDir, label string) string {
	return filepath.Join(worldDir, CheckpointsDir, label+".json")
}

func checkpointDumpPath(worldDir, label string) string {
	return filepath.Join(worldDir, CheckpointsDir, label+".dump")
}

func sortFileChanges(changes []FileChange) {
	sort.Slice(changes, func(i, j int) bool {
		if changes[i].Container != changes[j].Container {
			return changes[i].Container < changes[j].Container
		}
		if changes[i].Path != changes[j].Path {
			return changes[i].Path < changes[j].Path
		}
		return changes[i].Kind < changes[j].Kind
	})
}
