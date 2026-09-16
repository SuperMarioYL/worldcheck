package world

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// WorldDirContractError marks a world directory that does not satisfy the
// compose.yml + seed.sql contract.
type WorldDirContractError struct {
	Dir  string
	What string
}

func (e *WorldDirContractError) Error() string {
	return fmt.Sprintf("%s: %s", e.Dir, e.What)
}

// dbServiceName is the compose service the seed and the state fingerprint go
// through. The bundled postgres-starter world defines it; copied worlds keep
// the same contract.
const dbServiceName = "db"

// composeFile is the subset of the compose spec worldcheck needs: service
// names with their images (recorded in the manifest) and the db service's
// POSTGRES_USER / POSTGRES_DB environment (used to drive psql and pg_dump).
type composeFile struct {
	Services map[string]composeService `yaml:"services"`
}

type composeService struct {
	Image       string            `yaml:"image"`
	Environment map[string]string `yaml:"environment"`
}

// Runner executes external commands. Production uses ExecRunner, which shells
// out to the docker CLI; tests substitute a fake so the whole provision flow
// is verifiable without a Docker daemon.
type Runner interface {
	// Run executes argv in dir, feeds stdin to the process, and returns
	// stdout. A non-zero exit yields an error whose message carries stderr.
	Run(ctx context.Context, dir string, stdin io.Reader, argv ...string) (string, error)
}

// ExecRunner runs real commands via os/exec.
type ExecRunner struct{}

// Run implements Runner.
func (ExecRunner) Run(ctx context.Context, dir string, stdin io.Reader, argv ...string) (string, error) {
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	cmd.Dir = dir
	var out, errOut bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errOut
	if stdin != nil {
		cmd.Stdin = stdin
	}
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(errOut.String())
		if msg == "" {
			msg = err.Error()
		}
		return out.String(), fmt.Errorf("%s: %s", strings.Join(argv, " "), msg)
	}
	return out.String(), nil
}

// Provisioner provisions a world directory: stand up its compose project,
// apply the seed, and record the seeded state in a manifest.
type Provisioner struct {
	// Runner executes the docker CLI calls. nil means ExecRunner.
	Runner Runner
	// Now stamps the manifest; tests inject a fixed clock. nil means time.Now.
	Now func() time.Time
}

// ProvisionOptions select the world to provision and stamp the manifest.
type ProvisionOptions struct {
	// WorldDir must contain compose.yml and seed.sql.
	WorldDir string
	// ToolVersion is recorded in the manifest.
	ToolVersion string
}

// Provision is the m1 leg of the primitive: it always starts from a clean
// slate — `docker compose down -v` resets the world's volumes first — so a
// re-run after editing seed.sql re-seeds from scratch and produces a new
// fingerprint. The world is disposable; up never merges into old state.
func (p *Provisioner) Provision(ctx context.Context, opts ProvisionOptions) (*Manifest, error) {
	worldDir := opts.WorldDir
	worldName := filepath.Base(worldDir)
	runner := p.Runner
	if runner == nil {
		runner = ExecRunner{}
	}

	seedPath := filepath.Join(worldDir, "seed.sql")
	seed, err := os.ReadFile(seedPath)
	if err != nil {
		return nil, &WorldDirContractError{Dir: worldDir, What: "seed.sql not found"}
	}

	compose, err := parseCompose(worldDir)
	if err != nil {
		return nil, err
	}
	dbSvc, ok := compose.Services[dbServiceName]
	if !ok {
		return nil, &WorldDirContractError{
			Dir:  worldDir,
			What: fmt.Sprintf("compose.yml must define a %q service running Postgres", dbServiceName),
		}
	}
	dbUser := dbSvc.Environment["POSTGRES_USER"]
	if dbUser == "" {
		dbUser = "postgres"
	}
	dbName := dbSvc.Environment["POSTGRES_DB"]
	if dbName == "" {
		dbName = dbUser
	}

	project := composeProjectName(worldName)
	// docker compose reads compose.yml from its working directory.
	run := func(stdin io.Reader, argv ...string) (string, error) {
		return runner.Run(ctx, worldDir, stdin, argv...)
	}
	base := []string{"docker", "compose", "-p", project}

	// Reset the world's volumes. On the very first run nothing exists yet and
	// docker may fail; that is fine — the `up --wait` below is the real gate
	// and reports daemon problems itself.
	_, _ = run(nil, append(slices.Clone(base), "down", "-v", "--remove-orphans")...)

	if _, err := run(nil, append(slices.Clone(base), "up", "-d", "--wait")...); err != nil {
		return nil, fmt.Errorf("starting compose project %s: %w (is the Docker daemon running?)", project, err)
	}

	seedArgv := append(slices.Clone(base),
		"exec", "-T", dbServiceName,
		"psql", "-U", dbUser, "-d", dbName, "-v", "ON_ERROR_STOP=1", "-q",
	)
	if _, err := run(bytes.NewReader(seed), seedArgv...); err != nil {
		return nil, fmt.Errorf("applying %s: %w", "seed.sql", err)
	}

	dumpArgv := append(slices.Clone(base),
		"exec", "-T", dbServiceName,
		"pg_dump", "-U", dbUser, "--data-only", "--no-owner", "--no-privileges", dbName,
	)
	dump, err := run(nil, dumpArgv...)
	if err != nil {
		return nil, fmt.Errorf("capturing seeded state: %w", err)
	}

	tables, err := p.listTables(run, base, dbUser, dbName)
	if err != nil {
		return nil, err
	}

	now := p.Now
	if now == nil {
		now = time.Now
	}
	services := make([]Service, 0, len(compose.Services))
	for name, svc := range compose.Services {
		services = append(services, Service{Name: name, Image: svc.Image})
	}
	sortServices(services)

	m := &Manifest{
		Version:        1,
		World:          worldName,
		WorldDir:       worldDir,
		ComposeProject: project,
		ToolVersion:    opts.ToolVersion,
		CreatedAt:      now().UTC(),
		Services:       services,
		Seed: SeedInfo{
			File:      "seed.sql",
			Database:  dbName,
			SHA256:    SeedDigest(seed),
			AppliedAt: now().UTC(),
		},
		Tables: tables,
		Fingerprint: Fingerprint{
			Algorithm: FingerprintAlgorithm,
			Value:     ComputeFingerprint(dump),
		},
	}
	if err := SaveManifest(worldDir, m); err != nil {
		return nil, err
	}
	return m, nil
}

func (p *Provisioner) listTables(run func(io.Reader, ...string) (string, error), base []string, dbUser, dbName string) ([]Table, error) {
	argv := append(slices.Clone(base),
		"exec", "-T", dbServiceName,
		"psql", "-U", dbUser, "-d", dbName, "-t", "-A", "-c",
		`SELECT table_name FROM information_schema.tables `+
			`WHERE table_schema = 'public' AND table_type = 'BASE TABLE' `+
			`ORDER BY table_name`,
	)
	out, err := run(nil, argv...)
	if err != nil {
		return nil, fmt.Errorf("listing seeded tables: %w", err)
	}
	var tables []Table
	for _, name := range strings.Fields(out) {
		// Identifiers come from information_schema, never from user input;
		// quote-escape them anyway so odd table names cannot break the SQL.
		quoted := `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
		countArgv := append(slices.Clone(base),
			"exec", "-T", dbServiceName,
			"psql", "-U", dbUser, "-d", dbName, "-t", "-A", "-c",
			"SELECT count(*) FROM "+quoted,
		)
		countOut, err := run(nil, countArgv...)
		if err != nil {
			return nil, fmt.Errorf("counting rows of table %s: %w", name, err)
		}
		var rows int64
		if _, err := fmt.Sscanf(strings.TrimSpace(countOut), "%d", &rows); err != nil {
			return nil, fmt.Errorf("parsing row count of table %s: %w", name, err)
		}
		tables = append(tables, Table{Name: name, Rows: rows})
	}
	return tables, nil
}

// parseCompose reads and validates the world's compose.yml.
func parseCompose(worldDir string) (*composeFile, error) {
	data, err := os.ReadFile(filepath.Join(worldDir, "compose.yml"))
	if err != nil {
		return nil, &WorldDirContractError{Dir: worldDir, What: "compose.yml not found"}
	}
	var cf composeFile
	if err := yaml.Unmarshal(data, &cf); err != nil {
		return nil, fmt.Errorf("parsing compose.yml: %w", err)
	}
	if len(cf.Services) == 0 {
		return nil, &WorldDirContractError{Dir: worldDir, What: "compose.yml defines no services"}
	}
	return &cf, nil
}

// composeProjectName derives the docker compose project from the world
// directory name, prefixed per the architecture section of the plan
// (`wc-<world>`) and sanitized to what compose accepts.
var projectSanitizer = regexp.MustCompile(`[^a-z0-9_-]+`)

func composeProjectName(worldName string) string {
	lowered := strings.ToLower(worldName)
	sanitized := projectSanitizer.ReplaceAllString(lowered, "-")
	return "wc-" + strings.Trim(sanitized, "-")
}

func sortServices(services []Service) {
	sort.Slice(services, func(i, j int) bool { return services[i].Name < services[j].Name })
}
