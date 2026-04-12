# mgtt-provider-docker

An [MGTT](https://github.com/sajonaro/mgtt) provider for Docker containers. Probes container status via `docker inspect`.

## Install

```bash
mgtt provider install https://github.com/sajonaro/mgtt-provider-docker
```

This clones the repo, builds the binary via the install hook, and registers the provider. Requires Go for the build step.

## What It Provides

**Type: `container`**

| Fact | Type | Description |
|------|------|-------------|
| `running` | `mgtt.bool` | whether the container is running |
| `restart_count` | `mgtt.int` | number of restarts |
| `health_status` | `mgtt.string` | Docker health check status (healthy/unhealthy/none) |
| `exit_code` | `mgtt.int` | last exit code (0 = clean) |

**States** (evaluated top-to-bottom, first match wins):

| State | Condition | Description |
|-------|-----------|-------------|
| `healthy` | running, restarts < 5 | running normally |
| `degraded` | running, restarts >= 5 | restarting frequently |
| `stopped` | not running, exit 0 | exited cleanly |
| `crashed` | not running, exit != 0 | exited with error |

**Failure modes:**

| State | Can cause downstream |
|-------|---------------------|
| `degraded` | timeout, upstream_failure |
| `stopped` | upstream_failure, connection_refused |
| `crashed` | upstream_failure, connection_refused |

## Usage

Write a model referencing the `docker` provider:

```yaml
# system.model.yaml
meta:
  name: my-app
  version: "1.0"
  providers:
    - docker

components:
  redis:
    type: container
    healthy:
      - running == true

  api:
    type: container
    depends:
      - on: redis
```

Simulate a failure:

```yaml
# scenarios/redis-down.yaml
name: redis crashed
inject:
  redis:
    running: false
    exit_code: 1
  api:
    running: true
    restart_count: 0
expect:
  root_cause: redis
  path: [api, redis]
  eliminated: []
```

```bash
mgtt simulate --scenario scenarios/redis-down.yaml
```

Troubleshoot live:

```bash
mgtt plan
```

## Requirements

- [mgtt](https://github.com/sajonaro/mgtt) installed
- Docker CLI in PATH
- Access to the Docker socket (`/var/run/docker.sock`)
- Go toolchain (for building during install)

## Links

- [MGTT documentation](https://sajonaro.github.io/mgtt)
- [Provider authoring guide](https://sajonaro.github.io/mgtt/providers/overview/)
- [Provider registry](https://sajonaro.github.io/mgtt/reference/registry/)
