# Changelog

All notable changes to worldcheck are recorded here. The version in `VERSION`
is the release source of truth; a test keeps it in step with the version the
binary reports and the recorded demo run.

## v0.3.0

Three evidence defects, each reproduced by running v0.2.0 against a real
postgres:16-alpine world before it was fixed.

### Fixed

- `diff` refuses a checkpoint whose snapshot is not the one the checkpoint
  recorded. `checkpoint` stores the dump's sha256 in `<label>.json` and printed
  it as the checkpoint's database snapshot, but loading a checkpoint never
  compared the two, so a truncated or foreign `<label>.dump` silently became the
  baseline the report was measured against (an emptied dump reported every
  seeded row as inserted, at exit 0). The digest is now verified on load and a
  mismatch is an error naming the label, the recorded digest and the one on
  disk.
- `diff` reports sequences that moved instead of claiming nothing changed. Row
  deltas come from the `COPY` blocks and the schema digest deliberately excludes
  sequence values, so a mutation that advances a sequence without leaving a row
  — the classic `INSERT` then `ROLLBACK` — produced "no mutations since the
  checkpoint", while the seeded-state fingerprint (which counts those `setval`
  lines) had already moved for the same world. The report now lists the
  sequences that advanced (`tasks_id_seq 6 -> 7`) and the summary counts them;
  schema detection is unchanged, since a sequence value is data, not schema.
- A checkpoint-time filesystem mutation that was undone is reported by the state
  the path holds now. A path the checkpoint's baseline holds and the current
  `docker diff` does not now holds the image's own content — it is not gone. A
  reverted (or container-recreate-reset) `C` path was reported as `DELETED`
  although it exists, and a `D` path that came back was dropped from the report
  entirely. Both now read as what happened: `C` -> changed, `D` -> added, `A` ->
  deleted (unchanged).

## v0.2.0

Checkpoint and diff ship (the plan's m2), on top of three fixes reproduced by
running v0.1.0 against a real Docker daemon.

### Added

- `worldcheck checkpoint <label> --world <dir>` records the world's state under
  a label: the `docker diff` filesystem baseline of every container plus a full
  `pg_dump` snapshot, written to `<world dir>/checkpoints/<label>.json` and
  `.dump`.
- `worldcheck diff <label> --world <dir>` reports what moved since that
  checkpoint: filesystem paths whose `docker diff` status changed, row-level
  insertions/updates/deletions keyed by each table's primary key (read from the
  catalog), and whether the schema moved.

### Fixed

- `up` no longer depends on the world's compose `healthcheck`. It waits until the
  target database answers a real query before applying the seed, so a world copied
  per the documented contract (a `db` service plus `POSTGRES_USER`/`POSTGRES_DB`, no
  `healthcheck:`) provisions instead of failing with
  `database "app" does not exist`. The probe is a query rather than `pg_isready`
  because the postgres image's initdb runs a temporary server that accepts
  connections on the same socket before shutting down again.
- The seeded-state fingerprint now covers the schema as well as the data: the
  capture is a full `pg_dump`, so an added index, a dropped `NOT NULL` or any
  other schema-only mutation moves the fingerprint instead of reading as
  "nothing changed". The algorithm name changes to
  `sha256:pgdump-schema-and-data-sorted-lines`.
- `compose.yml` may write `environment:` in either shape compose accepts — a
  mapping or a list of `KEY=value` strings (the form `docker compose config`
  normalises to) — and scalar values are read as strings. An unreadable shape is
  reported as a world-contract error naming the accepted forms instead of a raw
  YAML unmarshal failure.

## v0.1.0

- `worldcheck up --world <dir>` provisions the bundled seeded Postgres world
  from `compose.yml` plus `seed.sql`, writes `manifest.json`, and prints the
  seeded-state fingerprint.
- `worldcheck status --world <dir>` prints the services, seeded table row counts
  and the fingerprint from the manifest.
