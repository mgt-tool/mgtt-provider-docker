# mgtt-provider-docker

Container health checks for [mgtt](https://github.com/mgt-tool/mgtt) backed by the Docker daemon.

```yaml
app:
  type: container
  providers: [docker]
  vars:
    max_restart_count: 5
    acceptable_exit_code: 0
```

When `mgtt plan` walks this component, the provider asks the local Docker daemon: "is `app` running, has it restarted more than 5 times, and what does its HEALTHCHECK say?" — using the answer to drive root-cause reasoning.

## Compatibility

| | |
|---|---|
| **Backend** | Docker Engine |
| **Versions** | `>=20.10, <27.0` |
| **Tested against** | `docker:25.0.5-dind` (digest pinned in integration tests) |

The provider shells out to the `docker` CLI (not the Engine API directly), so it works against any `DOCKER_HOST` your CLI is already configured for — local socket, remote daemon, Docker Desktop, rootless docker. Pre-20.10 daemons emit a different `inspect` State block and aren't supported.

## Install

```bash
mgtt provider install docker
```

## Auth

The provider reads from whatever your local `docker` CLI is set up to talk to. No credentials are passed via mgtt itself.

| Source | Used when |
|---|---|
| `DOCKER_HOST` env var | a remote daemon is configured |
| `/var/run/docker.sock` | local socket (default on Linux) |
| `~/.docker/contexts/` | Docker Contexts (Docker Desktop, etc.) |

Probes are `docker inspect` calls only — the provider never writes to the daemon.

## Type: `container`

One per Docker container you want to watch. The component name *is* the container name (or ID).

| Fact | Type | Returns |
|---|---|---|
| `running` | bool | `true` when the container is in the Running state |
| `restart_count` | int | restarts since the container was created |
| `health_status` | string | HEALTHCHECK status (`healthy`/`unhealthy`/`starting`), or `none` when not configured |
| `exit_code` | int | exit code from the most recent exit; 0 when running |
| `uptime_seconds` | int | seconds since the container last started; 0 when never started |

Each component declares its own SLO targets:

| Var | Default | What it controls |
|---|---|---|
| `max_restart_count` | (required) | restarts above this trigger `flapping` |
| `acceptable_exit_code` | (required) | exit codes other than this trigger `crashed` |

States: `live` → `flapping` (restarts > `max_restart_count`) → `unhealthy` (HEALTHCHECK failing) → `stopped` (exited cleanly) → `crashed` (exited with non-acceptable code).

## Example models

Two examples, deliberately separate so each tells one story end-to-end:

- [`examples/homelab.model.yaml`](./examples/homelab.model.yaml) — **steady state.** A small self-hosted homelab (nginx + app + postgres) with per-container restart budgets. The everyday shape of the `container` type.
- [`examples/homelab-backup-window.model.yaml`](./examples/homelab-backup-window.model.yaml) — **the backup moment.** During the nightly db backup the steady-state contract isn't right (postgres should be tighter, the backup container is *expected* to exit). Demonstrates loading a different model for a known operational window.

See the mgtt [Multi-File Models](https://mgt-tool.github.io/mgtt/concepts/multi-file-models/) doc for the methodology behind splitting per operational moment.

## Architecture

- `main.go` — 13 lines: registers types and calls `provider.Main`.
- `internal/probes/` — one ProbeFn per fact; reads from `docker inspect`.
- `internal/dockerclient/` — thin shell wrapper with timeout, sentinel error mapping (`No such container` → `ErrNotFound`, `cannot connect to daemon` → `ErrTransient`, `permission denied` → `ErrForbidden`).

Plumbing (argv parsing, exit codes, `status:not_found` translation, debug tracing) comes from [`mgtt/sdk/provider`](https://github.com/mgt-tool/mgtt/tree/main/sdk/provider).

## Development

```bash
go build .                          # compile
go test -race ./internal/...        # unit tests
go test -tags=integration ./...     # integration tests (requires docker)
mgtt provider validate docker       # static checks
```
