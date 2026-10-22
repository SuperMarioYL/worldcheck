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

**worldcheck is the test-world checkpoint tool for coding agents.** It hands every task a seeded, disposable test world: one command stands up a compose project plus seed.sql, a checkpoint freezes the world before verification, and a diff reports what the world accumulated afterwards — files, rows, and whether the schema moved. The agent's "tests pass" becomes checkable.

Free and open source (MIT), a single Go binary, no config file, only local Docker.

</div>

## Why

Hand a database-touching task to a coding agent: it invents its own test data, stands up its own environment, verifies against it, and reports "tests pass". What you see is green; what you don't see is what it changed inside that world — it may flip one of its own fixture rows, or quietly alter a table definition, to reach green.

The problem is not the tests — it is that **the verification environment is unauditable**: the agent built it privately, there is no baseline, nothing can be reset, and nothing can answer "what exactly did it touch to get there".

worldcheck takes over at the source: the world is seeded by the tool, its state is measured by the tool, and it is checkpointed by the tool before verification. v0.2 ships the complete pair — a **reproducible seeded world** (m1) and **checkpoint plus mutation diff** (m2): `checkpoint` freezes the filesystem baseline and the database snapshot, `diff` answers afterwards what moved.

## Architecture

<p align="center">
<picture>
  <source media="(prefers-color-scheme: dark)" srcset="assets/architecture-dark.svg">
  <source media="(prefers-color-scheme: light)" srcset="assets/architecture-light.svg">
  <img src="assets/architecture-light.svg" alt="Architecture: a shell drives the single worldcheck binary, which drives the compose project through the docker CLI; the seed and manifest flow through the world directory">
</picture>
</p>

One binary, one external dependency (the Docker CLI, invoked over stdio — no SDK, no daemon-version coupling):

- **provisioner** — `up` reads `compose.yml` and `seed.sql` from the world directory: `docker compose down -v` resets the world, `up -d` starts it, a bounded `pg_isready` poll waits until the database really accepts connections, `psql` applies the seed, and `pg_dump` captures the full state;
- **checkpoint** — `checkpoint <label>` records each container's `docker diff` filesystem baseline and stores the database snapshot under `checkpoints/` in the world directory;
- **differ** — `diff <label>` re-captures both and compares them: filesystem A/C/D, row-level insertions/updates/deletions aligned by primary key, and whether the schema changed;
- **manifest / report (terminal part)** — the seed sha256, services and images, per-table row counts and the overall state fingerprint go into `manifest.json`; `status` and `diff` render them as tables.

No server process, no database of its own, no Kubernetes. State lives in Docker (compose volumes) and in the world directory: `manifest.json`, `checkpoints/`, and — with m3 — `worldcheck.html`.

## Installation

```bash
go install github.com/SuperMarioYL/worldcheck/cmd/worldcheck@latest
```

Or build from source (Go 1.24+):

```bash
git clone https://github.com/SuperMarioYL/worldcheck.git
cd worldcheck && go build ./cmd/worldcheck
```

Requirements: a running local Docker daemon (macOS / Linux; Windows is out of scope). No configuration file needed.

## Quick start

The bundled world is `worlds/postgres-starter`: a single-service `postgres:16-alpine` compose project plus a seed SQL file. The image is small, so the pull never breaks the one-minute budget.

**1. Seed the world** (real output, see `docs/demo-results.json`):

```console
$ worldcheck up --world worlds/postgres-starter
world postgres-starter provisioned (project wc-postgres-starter)
  service db (postgres:16-alpine)
  table projects: 3 rows
  table tasks: 6 rows
  table users: 4 rows
manifest written to manifest.json
seeded-state fingerprint: bbc332815fe79e0680b313ee0ce12ead8bb24cc39cf3e0d94533de00cf8e1b3d
```

`up` always starts from a clean slate: it wipes the world's volumes first, then seeds — and it waits for the database to accept connections before seeding, so your world does not need a `healthcheck` in its compose file. The world is disposable; `up` never merges into old state.

**2. Checkpoint the world before verification:**

```console
$ worldcheck checkpoint t1 --world worlds/postgres-starter
checkpoint t1 recorded
  filesystem baseline: 4 path(s) across 1 container(s)
  database snapshot:   sha256:bbc332815fe7
  record:              checkpoints/t1.json
```

**3. Let verification run, then see what it touched:**

```console
$ worldcheck diff t1 --world worlds/postgres-starter
checkpoint t1
mutations  files 1 added / 1 changed / 0 deleted; rows 0 inserted / 1 updated / 0 deleted; schema unchanged

KIND     CONTAINER                 PATH
ADDED    wc-postgres-starter-db-1  /tmp/agent-note
CHANGED  wc-postgres-starter-db-1  /tmp

KIND     ROW
UPDATED  tasks[id=6] = 6 | 3 | keep diffs mechanical | f  ->  6 | 3 | keep diffs mechanical | t
```

That diff is catching an agent that quietly flipped one of its own fixture rows to green and left a file behind. Rows are aligned by primary key, read from the database catalog rather than guessed; whether the schema moved is reported separately.

**4. Inspect the state and its fingerprint:**

```console
$ worldcheck status --world worlds/postgres-starter
world      postgres-starter
project    wc-postgres-starter
seed       seed.sql (sha256:2a2fb32475ae)
database   app
provisioned 2026-10-22 19:28:43 UTC (worldcheck 0.2.0)

SERVICE  IMAGE
db       postgres:16-alpine

TABLE     ROWS
projects  3
tasks     6
users     4

fingerprint (sha256:pgdump-schema-and-data-sorted-lines)
  bbc332815fe79e0680b313ee0ce12ead8bb24cc39cf3e0d94533de00cf8e1b3d
```

**The acceptance criterion, in one command**: change a line of `seed.sql`, re-run `up`, and the fingerprint must change; leave it alone, and the fingerprint must not:

```console
$ echo 'UPDATE tasks SET done = true WHERE project_id = 2;' >> worlds/postgres-starter/seed.sql
$ worldcheck up --world worlds/postgres-starter | tail -1
seeded-state fingerprint: 63d0e70e2f5465aa5f376217b788827bcad18fbf802bd198493ca806ff485eff
```

The fingerprint covers the world's **whole state** — schema plus data, not data alone. Adding an index or dropping a `NOT NULL` moves it too, so a schema-only mutation can never read as "nothing happened".

<p align="center">
<picture>
  <source media="(prefers-color-scheme: dark)" srcset="assets/process-dark.svg">
  <source media="(prefers-color-scheme: light)" srcset="assets/process-light.svg">
  <img src="assets/process-light.svg" alt="The five steps of the up command: reset volumes, wait for the database, apply the seed, compute the fingerprint, write the manifest — plus the seed-edit acceptance note">
</picture>
</p>

## Demo

Below is the m2 loop: seed → checkpoint → the agent quietly bends the world to green → the diff catches it (every frame shows a real command and its real output, matching the run documented in `docs/demo-results.json`; frames switch on a timer, this is not a screen capture).

<p align="center">
<img src="assets/demo/worldcheck-demo.gif" alt="Demo: worldcheck up seeds the world, checkpoint marks it, the agent edits a fixture row and leaves a file, and the diff lists both changes" width="100%">
</p>

The machine this repository was built on could not encode with vhs, so the committed GIF is rendered from the real session records as static frames; the repo also ships `docs/demo.tape` (a vhs script) and `docs/demo-record.sh` (a one-command re-recording script) — run it once in an environment where vhs works to replace this with a true terminal capture. The planned final demo (`run` folding the verification command and the mutation diff into one reconciliation report) depends on m3 and will replace this recording once that milestone lands.

## Capabilities and boundaries

<p align="center">
<picture>
  <source media="(prefers-color-scheme: dark)" srcset="assets/capabilities-dark.svg">
  <source media="(prefers-color-scheme: light)" srcset="assets/capabilities-light.svg">
  <img src="assets/capabilities-light.svg" alt="Capability map: solid nodes are the implemented up, status, checkpoint, diff, manifest.json and fingerprint; dashed nodes are the roadmap's run and worldcheck.html">
</picture>
</p>

| Capability | Status | Notes |
| --- | --- | --- |
| `worldcheck up --world <dir>` | implemented | re-seeds the world from scratch: starts compose, waits for the database, applies the seed, writes `manifest.json`, prints the fingerprint |
| `worldcheck status --world <dir>` | implemented | prints services, table row counts and the state fingerprint from the manifest |
| state fingerprint | implemented | sha256 over a denoised, sorted full `pg_dump` (schema + data); the same seed must reproduce the same fingerprint, and any data or schema change must move it |
| `worldcheck checkpoint <label> --world <dir>` | implemented | pre-verification snapshot: each container's `docker diff` baseline plus a database snapshot, stored under `checkpoints/` |
| `worldcheck diff <label> --world <dir>` | implemented | filesystem A/C/D, row-level insertions/updates/deletions aligned by primary key, and whether the schema moved since a named checkpoint |
| `worldcheck run -- <verify-cmd>` | roadmap (m3) | chains checkpoint → run → diff → reconciliation report |

Explicit non-goals: no web UI (the report is a static HTML file); no MCP or "universal agent protocol"; checkpoint/diff support Postgres only, local Docker only; no rollback — worldcheck audits, it does not rewind; diffs are purely mechanical (`docker diff` + `pg_dump`), no LLM judgment.

## Configuration

There is no config file; everything is read from the world directory:

```
worlds/postgres-starter/
├── compose.yml      # the world: services, images (healthcheck optional — worldcheck waits itself)
├── seed.sql         # the baseline: schema plus fixed seed data
├── manifest.json    # generated (written by `up`, not committed): seed manifest + state fingerprint
└── checkpoints/     # generated (written by `checkpoint`, not committed): <label>.json + <label>.dump
```

Copy the directory to make it yours. The provisioner follows two conventions: the compose file must define a Postgres service named `db`, and the `POSTGRES_USER` / `POSTGRES_DB` environment variables tell it which credentials and database the seed and the snapshots go through — written either as a mapping (`POSTGRES_USER: postgres`) or as the list form `docker compose config` normalises to (`- POSTGRES_USER=postgres`). No host ports are published by default — two worlds never fight over 5432, and the agent's verification commands reach the database through the compose project (`docker compose -p wc-postgres-starter exec db psql ...`); add a `ports:` mapping in your own copy if you need a host socket.

A checkpoint label becomes a filename under `checkpoints/`, so it accepts letters, digits, dot, dash and underscore only.

## Roadmap

- **m1 — seed the world** (done): `up` + `status` + the seed manifest + the state fingerprint.
- **m2 — checkpoint and diff** (done, this release): `checkpoint <label>` freezes the world before verification; `diff <label>` emits machine-checkable filesystem A/C/D, row deltas aligned by primary key, and the schema verdict.
- **m3 — run and reconcile**: `run --label <label> -- <verify-cmd>` chains checkpoint → verification → diff and renders the terminal + `worldcheck.html` reconciliation report, exiting non-zero when the command fails; records the full demo.

## License

[MIT](./LICENSE) © 2026 SuperMarioYL. Issues and PRs welcome.

<p align="center"><sub><a href="./LICENSE">MIT</a> © 2026 SuperMarioYL</sub></p>
