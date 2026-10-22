// Package world owns the provisioned-world primitive: parsing a world
// directory, driving docker compose to stand up its services, applying the
// seed, and recording the resulting state in a manifest.
//
// v0.1 milestone coverage: provisioning (m1) is implemented here;
// checkpointing and diffing (m2) exist as typed stubs in checkpoint.go and
// diff.go.
package world

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// ManifestFileName is the state file worldcheck writes into the world
// directory after a successful provision. It is generated state and never
// ships with the repo.
const ManifestFileName = "manifest.json"

// FingerprintAlgorithm names how the seeded-state fingerprint is derived, so
// manifests stay self-describing across versions.
const FingerprintAlgorithm = "sha256:pgdump-schema-and-data-sorted-lines"

// ErrNoManifest is returned when status runs against a world that was never
// provisioned.
var ErrNoManifest = errors.New("no manifest found; run `worldcheck up --world <dir>` first")

// Service is one compose service participating in the world.
type Service struct {
	Name  string `json:"name"`
	Image string `json:"image"`
}

// SeedInfo records which seed was applied and when.
type SeedInfo struct {
	File      string    `json:"file"`
	Database  string    `json:"database"`
	SHA256    string    `json:"sha256"`
	AppliedAt time.Time `json:"applied_at"`
}

// Table is one public table in the seeded database with its row count.
type Table struct {
	Name string `json:"name"`
	Rows int64  `json:"rows"`
}

// Fingerprint is the machine-checkable digest of the seeded state.
type Fingerprint struct {
	Algorithm string `json:"algorithm"`
	Value     string `json:"value"`
}

// Manifest is the seed manifest of one provisioned world: the services compose
// started, the seed that was applied, the tables it created, and a fingerprint
// of the resulting database state.
type Manifest struct {
	Version        int         `json:"version"`
	World          string      `json:"world"`
	WorldDir       string      `json:"world_dir"`
	ComposeProject string      `json:"compose_project"`
	ToolVersion    string      `json:"tool_version"`
	CreatedAt      time.Time   `json:"created_at"`
	Services       []Service   `json:"services"`
	Seed           SeedInfo    `json:"seed"`
	Tables         []Table     `json:"tables"`
	Fingerprint    Fingerprint `json:"fingerprint"`
}

// LoadManifest reads the manifest from a world directory.
func LoadManifest(worldDir string) (*Manifest, error) {
	data, err := os.ReadFile(filepath.Join(worldDir, ManifestFileName))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, ErrNoManifest
		}
		return nil, err
	}
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", ManifestFileName, err)
	}
	return &m, nil
}

// SaveManifest writes the manifest into the world directory.
func SaveManifest(worldDir string, m *Manifest) error {
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(filepath.Join(worldDir, ManifestFileName), data, 0o644)
}

// SeedDigest returns the hex sha256 of the seed file contents.
func SeedDigest(seed []byte) string {
	sum := sha256.Sum256(seed)
	return hex.EncodeToString(sum[:])
}

// ComputeFingerprint derives the seeded-state fingerprint from a full pg_dump
// capture of the world's database — schema and data together, so the digest
// covers the whole seeded state: a data mutation and a schema-only mutation
// (an added index, a dropped NOT NULL, a changed default) both move it.
// Comment lines, blank lines, and psql meta-commands that embed a per-dump
// random token (`\restrict` / `\unrestrict`, emitted by current pg_dump
// security patches) are dropped, and the remainder is sorted before hashing, so
// the digest depends on the logical state rather than dump ordering or session
// state: identical seeded state produces an identical fingerprint, and any
// change — an edited seed.sql, a mutated row, an altered schema — changes it.
func ComputeFingerprint(pgDumpOutput string) string {
	lines := strings.Split(pgDumpOutput, "\n")
	kept := make([]string, 0, len(lines))
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "--") ||
			strings.HasPrefix(trimmed, `\restrict`) || strings.HasPrefix(trimmed, `\unrestrict`) {
			continue
		}
		kept = append(kept, trimmed)
	}
	sort.Strings(kept)
	sum := sha256.Sum256([]byte(strings.Join(kept, "\n")))
	return hex.EncodeToString(sum[:])
}
