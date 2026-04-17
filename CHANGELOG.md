# Changelog

Format: [Keep a Changelog](https://keepachangelog.com/en/1.1.0/). Versioning: [SemVer](https://semver.org/).

## [0.2.0] — 2026-04-16

Major restructure to align with the architecture used by sibling tracing providers (mgtt-provider-tempo, mgtt-provider-quickwit). The single-file standalone implementation is replaced by an SDK-based provider with sentinel error mapping, parameterized SLO targets, integration tests, and the new compatibility-declaration contract.

### Added

- **`compatibility:` block in `manifest.yaml`** — declares the backend versions this provider is built against (Docker `>=20.10, <27.0`), the exact tested image digest, and version-sensitive behaviors. README surfaces the contract prominently near the top.
- **`internal/dockerclient/`** — thin shell wrapper around the `docker` CLI with timeout, classify-to-sentinel-error mapping (`No such container` → `ErrNotFound`, `cannot connect` → `ErrTransient`, `permission denied` → `ErrForbidden`, parse failures → `ErrProtocol`).
- **`internal/probes/`** — ProbeFn-shaped probes consumed by `mgtt/sdk/provider`. Argv parsing, JSON output, and exit-code translation all come from the SDK.
- **`uptime_seconds` fact** — seconds since the container last started; useful as a freshness signal for one-shot/cron containers.
- **Per-component SLO targets** — `max_restart_count` and `acceptable_exit_code` are now per-component vars referenced by the type manifest's `healthy:` / `states:` expressions instead of hardcoded constants. Each component reads as a contract.
- **Integration tests** in `test/integration/` exercising five end-to-end scenarios against a real Docker daemon (running container, exited non-zero, crash-looping, missing container → `status:not_found`, usage errors → exit 1). Image pinned by SHA digest.
- **Two example models** in `examples/` following the [Multi-File Models](https://mgt-tool.github.io/mgtt/concepts/multi-file-models/) methodology: `homelab.model.yaml` (steady-state) and `homelab-backup-window.model.yaml` (the nightly backup moment with a tighter contract).
- **`hooks/uninstall.sh`** — symmetric with `install.sh`; cleans up `bin/` so `mgtt provider uninstall docker` is a real operation.
- **`VERSION`** + this `CHANGELOG.md` — neither existed before.

### Changed

- **Module path** — `mgtt-provider-docker` → `github.com/mgt-tool/mgtt-provider-docker` to match the registry entry.
- **`main.go`** is now 13 lines (was 138). All argv parsing, JSON output, and error mapping moved to the SDK.
- **`states:` matrix** — added `unhealthy` (HEALTHCHECK failing) as a distinct state from `flapping`, since the operator response is different (read the app logs vs check the HEALTHCHECK script).

### Migration notes

If you have an existing model written against v0.1.0:

- Add `max_restart_count` and `acceptable_exit_code` vars to every `container` component. v0.1.0's hardcoded `restart_count < 5` constraint is now per-component.
- The `healthy` state was renamed to `live` for symmetry with the rest of the provider ecosystem (tempo, quickwit). The `degraded` state was renamed to `flapping`.
- Component names continue to map directly to container names — no change.

## [0.1.0] — 2026-04-13

Initial release. Standalone Go binary with `container` type, four facts (`running`, `restart_count`, `health_status`, `exit_code`), no integration tests.
