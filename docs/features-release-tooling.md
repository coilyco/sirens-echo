# Features: cross-repo tooling and release

This repository uses `docs/features-release-tooling.md` as the durable
cross-reference for the catalog entry points in `README.md`, `AGENTS.md`, and
`docs/FEATURES.md`.

The release and validation surfaces stay separate:

- `.pre-commit-config.yaml` pins the shared agentic-os validation suite.
- `.forgejo/workflows/ci.yml` runs the repository build, tests, and validation.
- Main pushes publish the private full-source-SHA Sirens Echo image to Forgejo
  OCI only after the test job passes.
- `coilyco-bridge/deploy` owns the read-only pull credential, k3s rollout, and
  rollback.

## See also

- [README.md](../README.md) - human-facing introduction.
- [AGENTS.md](../AGENTS.md) - agent-facing rules.
- [FEATURES.md](FEATURES.md) - shipped capability inventory.
- [.ward/ward.yaml](../.ward/ward.yaml) - allowlisted development commands.
