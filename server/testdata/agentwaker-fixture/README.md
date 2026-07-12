# AgentWaker test fixture

A minimal AgentWaker directory used by the `agentwaker` package tests and the
M1 daemon scanner tests. It exercises every contract without depending on the
real (large) AgentWaker repository.

## Layout

- `capabilities/registry.yaml` — registry of 2 capabilities.
- `capabilities/information-collection/` — capability with 2 profiles, 2 adapters.
- `capabilities/visual-generation/` — capability with 2 profiles, `local-write`.
- `schemas/profile-v2.1.schema.json` — stub satisfying the discovery gate.
- `research-operator/` — role WITH two capability dependencies, MCP `${ENV}`
  references, a secret env value, a `workdir/` runtime artifact, and a binary
  `cover.png` asset that must hit the skip path.
- `plain-operator/` — role with NO capability dependencies (empty-binding path).
- `.git/`, `node_modules/` — must be skipped by the scanner.
- `research-operator/secrets.env` — an unrelated `.env`-style file OUTSIDE the
  recognized `{role}/env/.env` path. The scanner must NOT read it.

## Secret hygiene

`research-operator/env/.env` contains the literal string
`super-secret-value-do-not-leak`. The redaction tests assert that this string
never appears in any sanitized output, digest, or manifest.
