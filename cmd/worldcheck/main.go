// Command worldcheck is the test-world snapshot and mutation-audit CLI for
// coding agents.
//
// v0.1 milestone coverage:
//   - m1 (implemented): `up` provisions a seeded compose world and records its
//     manifest; `status` prints the seeded state and its fingerprint.
//   - m2/m3 (stubs): `checkpoint`, `diff`, and `run` return the roadmap note
//     until those milestones ship.
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
var version = "0.1.0"

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

  worldcheck up --world worlds/postgres-starter   provision the world and record its manifest
  worldcheck status --world worlds/postgres-starter   print services plus the seeded-state fingerprint

Milestones: provisioning ships today; checkpoint/diff (m2) and the
verification run with its reconciliation report (m3) are on the roadmap.`,
		SilenceUsage:  true,
		SilenceErrors: true,
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

// stubRunE returns the roadmap error shared by the m2/m3 commands. It wraps
// world.ErrNotImplemented so callers can test for it programmatically.
func stubRunE(milestone, what string) func(cmd *cobra.Command, args []string) error {
	return func(cmd *cobra.Command, args []string) error {
		return fmt.Errorf("%w: %s ships with the %s milestone (%s); see the roadmap in README.md",
			world.ErrNotImplemented, what, milestone, stubNote(milestone))
	}
}

func stubNote(milestone string) string {
	switch milestone {
	case "m2":
		return "checkpoint/diff"
	case "m3":
		return "run/reconcile report"
	}
	return milestone
}

func newCheckpointCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "checkpoint <label>",
		Short: "(m2 roadmap) Snapshot world state under a label",
		Args:  cobra.ExactArgs(1),
		RunE:  stubRunE("m2", "checkpoint"),
	}
}

func newDiffCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "diff <label>",
		Short: "(m2 roadmap) Diff world state against a checkpoint label",
		Args:  cobra.ExactArgs(1),
		RunE:  stubRunE("m2", "diff"),
	}
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
