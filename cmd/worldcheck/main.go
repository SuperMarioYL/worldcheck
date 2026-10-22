// Command worldcheck is the test-world snapshot and mutation-audit CLI for
// coding agents.
//
// Milestone coverage:
//   - m1 (implemented): `up` provisions a seeded compose world and records its
//     manifest; `status` prints the seeded state and its fingerprint.
//   - m2 (implemented): `checkpoint` snapshots the world under a label and
//     `diff` reports the filesystem and row mutations since it.
//   - m3 (stub): `run` returns the roadmap note until it ships.
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/SuperMarioYL/worldcheck/internal/report"
	"github.com/SuperMarioYL/worldcheck/internal/world"
)

// version is stamped at build time via -ldflags; the VERSION file is the
// release source of truth.
var version = "0.2.0"

func main() {
	root := newRootCmd()
	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}

func newRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:     "worldcheck",
		Short:   "Test-world snapshots and mutation audits for coding agents",
		Version: version,
		Long: `worldcheck gives every agent task a seeded, disposable test world.

  worldcheck up --world worlds/postgres-starter         provision the world and record its manifest
  worldcheck status --world worlds/postgres-starter     print services plus the seeded-state fingerprint
  worldcheck checkpoint t1 --world worlds/postgres-starter   snapshot the world under a label
  worldcheck diff t1 --world worlds/postgres-starter    print the mutations since that checkpoint

Milestones: provisioning (m1) and checkpoint/diff (m2) ship today; the
verification run with its reconciliation report (m3) is on the roadmap.`,
		// Suppress the usage dump on errors, but let cobra print the error
		// itself (as `Error: ...` on stderr) so runtime failures — bad world
		// dir, missing manifest, stub roadmap notes — are never silent.
		SilenceUsage: true,
	}
	root.SetVersionTemplate("worldcheck version {{.Version}}\n")
	root.AddCommand(newUpCmd(), newStatusCmd(), newCheckpointCmd(), newDiffCmd(), newRunCmd())
	return root
}

func newUpCmd() *cobra.Command {
	var worldDir string
	cmd := &cobra.Command{
		Use:   "up --world <dir>",
		Short: "Provision a seeded test world and record its manifest",
		Long: `Provisions the compose project of the given world from a clean slate:
resets its volumes, starts the services, applies seed.sql, and writes
manifest.json with the seeded-state fingerprint.

Because the world is disposable, every ` + "`up`" + ` re-seeds from scratch —
edit seed.sql and run ` + "`up`" + ` again to see the fingerprint change.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			p := &world.Provisioner{}
			m, err := p.Provision(context.Background(), world.ProvisionOptions{
				WorldDir:    worldDir,
				ToolVersion: version,
			})
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "world %s provisioned (project %s)\n", m.World, m.ComposeProject)
			for _, svc := range m.Services {
				fmt.Fprintf(cmd.OutOrStdout(), "  service %s (%s)\n", svc.Name, svc.Image)
			}
			for _, t := range m.Tables {
				fmt.Fprintf(cmd.OutOrStdout(), "  table %s: %d rows\n", t.Name, t.Rows)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "manifest written to %s\n", world.ManifestFileName)
			report.RenderFingerprint(cmd.OutOrStdout(), m)
			return nil
		},
	}
	cmd.Flags().StringVar(&worldDir, "world", "", "world directory containing compose.yml and seed.sql (required)")
	_ = cmd.MarkFlagRequired("world")
	return cmd
}

func newStatusCmd() *cobra.Command {
	var worldDir string
	cmd := &cobra.Command{
		Use:   "status --world <dir>",
		Short: "Print the world's services and seeded-state fingerprint",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			m, err := world.LoadManifest(worldDir)
			if err != nil {
				return err
			}
			return report.RenderStatus(cmd.OutOrStdout(), m)
		},
	}
	cmd.Flags().StringVar(&worldDir, "world", "", "world directory holding manifest.json (required)")
	_ = cmd.MarkFlagRequired("world")
	return cmd
}

// stubRunE returns the roadmap error shared by the m3 command. It wraps
// world.ErrNotImplemented so callers can test for it programmatically.
func stubRunE(milestone, what string) func(cmd *cobra.Command, args []string) error {
	return func(cmd *cobra.Command, args []string) error {
		return fmt.Errorf("%w: %s ships with the %s milestone (%s); see the roadmap in README.md",
			world.ErrNotImplemented, what, milestone, stubNote(milestone))
	}
}

func stubNote(milestone string) string {
	if milestone == "m3" {
		return "run/reconcile report"
	}
	return milestone
}

func newCheckpointCmd() *cobra.Command {
	var worldDir string
	cmd := &cobra.Command{
		Use:   "checkpoint <label> --world <dir>",
		Short: "Snapshot the world's filesystem and database state under a label",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cp, err := world.CreateCheckpoint(context.Background(), world.ExecRunner{}, worldDir, args[0])
			if err != nil {
				return err
			}
			containers := map[string]bool{}
			for _, c := range cp.FSBaseline {
				containers[c.Container] = true
			}
			fmt.Fprintf(cmd.OutOrStdout(), "checkpoint %s recorded\n", cp.Label)
			fmt.Fprintf(cmd.OutOrStdout(), "  filesystem baseline: %d path(s) across %d container(s)\n",
				len(cp.FSBaseline), len(containers))
			fmt.Fprintf(cmd.OutOrStdout(), "  database snapshot:   %s\n", shortDigest(cp.DBDumpSHA256))
			fmt.Fprintf(cmd.OutOrStdout(), "  record:              %s/%s.json\n", world.CheckpointsDir, cp.Label)
			return nil
		},
	}
	cmd.Flags().StringVar(&worldDir, "world", "", "world directory holding compose.yml and seed.sql (required)")
	_ = cmd.MarkFlagRequired("world")
	return cmd
}

func newDiffCmd() *cobra.Command {
	var worldDir string
	cmd := &cobra.Command{
		Use:   "diff <label> --world <dir>",
		Short: "Diff world state against a checkpoint label",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			diff, err := world.DiffAgainst(context.Background(), world.ExecRunner{}, worldDir, args[0])
			if err != nil {
				return err
			}
			return report.RenderDiff(cmd.OutOrStdout(), diff)
		},
	}
	cmd.Flags().StringVar(&worldDir, "world", "", "world directory holding compose.yml and seed.sql (required)")
	_ = cmd.MarkFlagRequired("world")
	return cmd
}

func shortDigest(digest string) string {
	if len(digest) > 12 {
		return "sha256:" + digest[:12]
	}
	return "sha256:" + digest
}

func newRunCmd() *cobra.Command {
	var label string
	cmd := &cobra.Command{
		Use:   "run --label <label> -- <verify-cmd>",
		Short: "(m3 roadmap) Checkpoint, run the verification command, reconcile the mutations",
		Args:  cobra.ArbitraryArgs,
		RunE:  stubRunE("m3", "run"),
	}
	cmd.Flags().StringVar(&label, "label", "", "checkpoint label for this verification run")
	_ = cmd.MarkFlagRequired("label")
	return cmd
}
