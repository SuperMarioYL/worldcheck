-- postgres-starter seed: the baseline state of this test world.
--
-- worldcheck applies this file after the database reports healthy, then
-- records its sha256 digest and the resulting state fingerprint in
-- manifest.json. Timestamps are fixed literals on purpose: the seeded state
-- must depend only on the content of this file, so re-provisioning an
-- unmodified world reproduces the same fingerprint.
BEGIN;

CREATE TABLE users (
    id         serial PRIMARY KEY,
    email      text        NOT NULL UNIQUE,
    created_at timestamptz NOT NULL
);

CREATE TABLE projects (
    id       serial PRIMARY KEY,
    owner_id integer NOT NULL REFERENCES users(id),
    name     text    NOT NULL,
    archived boolean NOT NULL DEFAULT false
);

CREATE TABLE tasks (
    id         serial PRIMARY KEY,
    project_id integer NOT NULL REFERENCES projects(id),
    title      text    NOT NULL,
    done       boolean NOT NULL DEFAULT false
);

INSERT INTO users (email, created_at) VALUES
    ('ada@example.test',   '2026-09-01T09:00:00Z'),
    ('lin@example.test',   '2026-09-01T09:05:00Z'),
    ('maria@example.test', '2026-09-01T09:10:00Z'),
    ('tomas@example.test', '2026-09-01T09:15:00Z');

INSERT INTO projects (owner_id, name, archived) VALUES
    (1, 'worldcheck',     false),
    (2, 'datamimic-spike', false),
    (3, 'sandbox-notes',  false);

INSERT INTO tasks (project_id, title, done) VALUES
    (1, 'seed the world',              true),
    (1, 'checkpoint before verify',    false),
    (2, 'diff row mutations',          false),
    (2, 'reconcile the pass claim',    false),
    (3, 'audit agent changes',         false),
    (3, 'keep diffs mechanical',       false);

COMMIT;
