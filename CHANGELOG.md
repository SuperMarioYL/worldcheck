# Changelog

All notable changes to worldcheck are recorded here. The version in `VERSION`
is the release source of truth; a test keeps it in step with the version the
binary reports and the recorded demo run.

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
