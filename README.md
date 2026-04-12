# mgtt-provider-docker

An [MGTT](https://github.com/sajonaro/mgtt) provider for Docker containers.

## Install

```bash
mgtt provider install ./path/to/mgtt-provider-docker
```

Or clone and install:

```bash
git clone https://github.com/sajonaro/mgtt-provider-docker.git
mgtt provider install ./mgtt-provider-docker
```

## What It Provides

**Type: `container`**

| Fact | Type | Description |
|------|------|-------------|
| `running` | `mgtt.bool` | whether the container is running |
| `restart_count` | `mgtt.int` | number of restarts |
| `health_status` | `mgtt.string` | Docker health check status (healthy/unhealthy/none) |
| `exit_code` | `mgtt.int` | last exit code (0 = clean) |

**States:**

| State | Condition | Description |
|-------|-----------|-------------|
| `healthy` | running, restarts < 5 | running normally |
| `degraded` | running, restarts >= 5 | restarting frequently |
| `stopped` | not running, exit 0 | exited cleanly |
| `crashed` | not running, exit != 0 | exited with error |

**Failure modes:**

| State | Can cause |
|-------|-----------|
| `degraded` | timeout, upstream_failure |
| `stopped` | upstream_failure, connection_refused |
| `crashed` | upstream_failure, connection_refused |

## Usage in a Model

```yaml
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

Then troubleshoot:

```bash
mgtt plan
```

## Requirements

- Docker CLI available in PATH
- Access to the Docker socket (`/var/run/docker.sock`)

## Writing Your Own Provider

See the [provider authoring guide](https://github.com/sajonaro/mgtt/blob/main/providers/README.md).
