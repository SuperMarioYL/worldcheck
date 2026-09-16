**English** | [简体中文](./README.md)

<div align="center">

# worldcheck

<picture>
  <source media="(prefers-color-scheme: dark)" srcset="assets/hero-dark.svg">
  <source media="(prefers-color-scheme: light)" srcset="assets/hero-light.svg">
  <img src="assets/hero-light.svg" alt="worldcheck brand art: particles converging into a checkpoint seal, with the up and status commands below">
</picture>

[![ci](https://img.shields.io/github/actions/workflow/status/SuperMarioYL/worldcheck/ci.yml?style=flat-square&label=ci)](https://github.com/SuperMarioYL/worldcheck/actions/workflows/ci.yml)
[![go](https://img.shields.io/badge/go-1.24-00ADD8?logo=go&logoColor=white&style=flat-square)](https://go.dev)
[![license](https://img.shields.io/badge/license-MIT-blue?style=flat-square)](./LICENSE)
[![platform](https://img.shields.io/badge/platform-macOS%20%7C%20Linux-lightgrey?style=flat-square)](#installation)
[![requires docker](https://img.shields.io/badge/requires-Docker-2496ED?logo=docker&logoColor=white&style=flat-square)](#quick-start)

<img src="https://readme-typing-svg.herokuapp.com?font=JetBrains+Mono&weight=600&size=22&pause=1600&color=56ACE6&center=true&vCenter=true&width=660&height=46&lines=Snapshot%20before%20verify%2C%20audit%20after.;Make%20the%20pass%20claim%20checkable." alt="Snapshot before verify, audit after / Make the pass claim checkable" />

**worldcheck is the test-world checkpoint tool for coding agents.** It hands every task a seeded, disposable test world: one command stands up a compose project plus seed.sql and records the seed manifest and state fingerprint into `manifest.json`. Change one line of the seed, re-run, and the fingerprint must change — an auditable world starts there.

Free and open source (MIT), a single Go binary, no config file, only local Docker.

</div>

## Why

Hand a database-touching task to a coding agent: it invents its own test data, stands up its own environment, verifies against it, and reports "tests pass". What you see is green; what you don't see is what it changed inside that world.

The problem is not the tests — it is that **the verification environment is unauditable**: the agent built it privately, there is no baseline, nothing can be reset, and nothing can answer "what exactly did it touch to get there".

worldcheck takes over at the source: the world is seeded by the tool, and its state is measured by the tool. The v0.1 m1 milestone ships the foundation — a **reproducible seeded world**. Only a world that can be rebuilt and measured exactly gives the later pieces their footing: the pre-verification checkpoint (m2) and the mutation diff (m2/m3). The fingerprint is the first cornerstone: it turns "the seed didn't change" from a verbal promise into a hash anyone can check.

## Architecture

<p align="center">
<picture>
  <source media="(prefers-color-scheme: dark)" srcset="assets/architecture-dark.svg">
  <source media="(prefers-color-scheme: light)" srcset="assets/architecture-light.svg">
  <img src="assets/architecture-light.svg" alt="Architecture: a shell drives the single worldcheck binary, which drives the compose project through the docker CLI; the seed and manifest flow through the world directory">
</picture>
</p>

One binary, one external dependency (the Docker CLI, invoked over stdio — no SDK, no daemon-version coupling):

- **provisioner** — `up` reads `compose.yml` and `seed.sql` from the world directory: `docker compose down -v` resets the world, `up -d --wait` waits for the healthcheck, `psql` applies the seed, and `pg_dump --data-only` captures the result;
- **manifest** — the seed sha256, services and images, per-table row counts, and the overall state fingerprint, written to `manifest.json` in the world directory;
- **report (terminal part)** — `status` renders the manifest as tables.

No server process, no database of its own, no Kubernetes. State lives in Docker (compose volumes) and in two files per world on disk: `manifest.json`, and — with m3 — `worldcheck.html`.

## Installation

```bash
go install github.com/SuperMarioYL/worldcheck/cmd/worldcheck@latest
```

Or build from source (Go 1.24+):

```bash
git clone https://github.com/SuperMarioYL/worldcheck.git
cd worldcheck && go build ./cmd/worldcheck
```

Requirements: a running local Docker daemon (macOS / Linux; Windows is out of scope for v0.1). No configuration file needed.

## Quick start

The bundled world is `worlds/postgres-starter`: a single-service `postgres:16-alpine` compose project plus a seed SQL file. The image is small, so the pull never breaks the one-minute budget. The whole path is two commands:

**1. Seed the world** (real output, see `docs/demo-results.json`):

```console
$ worldcheck up --world worlds/postgres-starter
world postgres-starter provisioned (project wc-postgres-starter)
  service db (postgres:16-alpine)
  table projects: 3 rows
  table tasks: 6 rows
  table users: 4 rows
manifest written to manifest.json
seeded-state fingerprint: 7d5f34937a7855e218e1ea2ab088e38394f6666d41290651cf50be8a0816dfd4
```

`up` always starts from a clean slate: it wipes the world's volumes first, then seeds. The world is disposable; `up` never merges into old state.

**2. Inspect the state and its fingerprint:**

```console
$ worldcheck status --world worlds/postgres-starter
world      postgres-starter
project    wc-postgres-starter
seed       seed.sql (sha256:2a2fb32475ae)
database   app
provisioned 2026-09-16 15:09:52 UTC (worldcheck 0.1.0)

SERVICE  IMAGE
db       postgres:16-alpine

TABLE     ROWS
projects  3
tasks     6
users     4

fingerprint (sha256:pgdump-data-only-sorted-lines)
  7d5f34937a7855e218e1ea2ab088e38394f6666d41290651cf50be8a0816dfd4
```

**The acceptance criterion, in one command**: change a line of `seed.sql`, re-run `up`, and the fingerprint must change; leave it alone, and the fingerprint must not:

```console
$ echo 'UPDATE tasks SET done = true WHERE project_id = 2;' >> worlds/postgres-starter/seed.sql
$ worldcheck up --world worlds/postgres-starter | tail -1
seeded-state fingerprint: 9e0f82b6d9f6a412a92956f99844228a186354c1acac9e27a929980bac8bb6a5
```

That is all of m1: the world can be seeded, its state can be measured, and the seed-to-fingerprint mapping can be checked by anyone.

<p align="center">
<picture>
  <source media="(prefers-color-scheme: dark)" srcset="assets/process-dark.svg">
  <source media="(prefers-color-scheme: light)" srcset="assets/process-light.svg">
  <img src="assets/process-light.svg" alt="The five steps of the up command: reset volumes, wait for health, apply the seed, compute the fingerprint, write the manifest — plus the seed-edit acceptance note">
</picture>
</p>

## Demo

Below is a frame-by-frame static demo of the recorded m1 session: every frame shows a real command and its real output, matching the run documented in `docs/demo-results.json` (frames switch on a timer; this is not a screen capture).

<p align="center">
<img src="assets/demo/worldcheck-demo.gif" alt="Static-frame demo: worldcheck up seeds the world and prints the fingerprint, status inspects the manifest, the seed is edited and up re-run, the fingerprint changes" width="100%">
</p>

The machine this repository was built on could not encode with vhs, so the committed GIF is rendered from the real session records as static frames; the repo also ships `docs/demo.tape` (a vhs script) and `docs/demo-record.sh` (a one-command re-recording script) — run it once in an environment where vhs works to replace this with a true terminal capture. And this is not the final form — the planned full demo (an agent quietly bending its own seeded world to green, caught by the mutation diff) depends on m3's run/reconcile capability and will replace this recording once that milestone lands.

## Capabilities and boundaries

<p align="center">
<picture>
  <source media="(prefers-color-scheme: dark)" srcset="assets/capabilities-dark.svg">
  <source media="(prefers-color-scheme: light)" srcset="assets/capabilities-light.svg">
  <img src="assets/capabilities-light.svg" alt="Capability map: solid nodes are the implemented up, status, manifest.json and fingerprint; dashed nodes are the roadmap's checkpoint, diff, run and worldcheck.html">
</picture>
</p>

| Capability | Status | Notes |
| --- | --- | --- |
| `worldcheck up --world <dir>` | implemented | re-seeds the world from scratch: starts compose, applies the seed, writes `manifest.json`, prints the fingerprint |
| `worldcheck status --world <dir>` | implemented | prints services, table row counts and the seeded-state fingerprint from the manifest |
| seeded-state fingerprint | implemented | sha256 over a denoised, sorted `pg_dump --data-only`; same seed must reproduce the same fingerprint, any seed edit must change it |
| `worldcheck checkpoint <label>` | roadmap (m2) | pre-verification snapshot: docker commit plus a data baseline |
| `worldcheck diff <label>` | roadmap (m2) | file and row-level changes since a named snapshot |
| `worldcheck run -- <verify-cmd>` | roadmap (m3) | chains checkpoint → run → diff → report |

Explicit non-goals for v0.1: no web UI (the report is a static HTML file); no MCP or "universal agent protocol"; checkpoint/diff support Postgres only, local Docker only; no rollback — v0.1 audits, it does not rewind; diffs are purely mechanical (docker diff + pg_dump hashes), no LLM judgment.

## Configuration

There is no config file; everything is read from the world directory:

```
worlds/postgres-starter/
├── compose.yml     # the world: services, image, healthcheck
├── seed.sql        # the baseline: schema plus fixed seed data
└── manifest.json   # generated (written by `up`, not committed): seed manifest + state fingerprint
```

Copy the directory to make it yours. The provisioner follows two conventions: the compose file must define a Postgres service named `db`, and the `POSTGRES_USER` / `POSTGRES_DB` environment variables tell it which credentials and database the seed and fingerprint go through. No host ports are published by default — two worlds never fight over 5432, and the agent's verification commands reach the database through the compose project (`docker compose -p wc-postgres-starter exec db psql ...`); add a `ports:` mapping in your own copy if you need a host socket.

## Roadmap

- **m1 — seed the world** (done, this release): `up` + `status` + the seed manifest + the state fingerprint.
- **m2 — checkpoint and diff**: `checkpoint <label>` freezes the world before verification; `diff <label>` emits machine-checkable filesystem A/C/D and row deltas.
- **m3 — run and reconcile**: `run --label <label> -- <verify-cmd>` chains checkpoint → verification → diff and renders the terminal + `worldcheck.html` reconciliation report, exiting non-zero when the command fails; records the full demo — an agent bending its own world to green, caught by the report.

## License

[MIT](./LICENSE) © 2026 SuperMarioYL. Issues and PRs welcome.

<p align="center"><sub><a href="./LICENSE">MIT</a> © 2026 SuperMarioYL</sub></p>
