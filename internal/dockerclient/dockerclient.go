// Package dockerclient wraps `docker inspect` with timeout and the
// classify-to-sentinel-error mapping used by every docker-provider probe.
//
// We shell out to the docker CLI (rather than the Engine API directly) so
// the provider works against any DOCKER_HOST the operator's CLI is already
// configured for: local socket, remote daemon, Docker Desktop on macOS,
// rootless docker, etc. The cost is one fork per probe.
package dockerclient

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/mgt-tool/mgtt/sdk/provider"
)

const defaultTimeout = 10 * time.Second

// Client is a thin docker CLI wrapper. Tests inject Run for fakes.
type Client struct {
	Timeout time.Duration
	// Run executes a docker command and returns combined stdout. Tests
	// override this to avoid forking real processes.
	Run func(ctx context.Context, args ...string) ([]byte, error)
}

// New returns a Client that shells out to the real `docker` binary.
func New() *Client {
	return &Client{
		Timeout: defaultTimeout,
		Run: func(ctx context.Context, args ...string) ([]byte, error) {
			cmd := exec.CommandContext(ctx, "docker", args...)
			return cmd.Output()
		},
	}
}

// Inspect runs `docker inspect <container>` and returns the parsed first
// element of the JSON array (docker returns a list even for single names).
func (c *Client) Inspect(ctx context.Context, container string) (map[string]any, error) {
	cctx, cancel := context.WithTimeout(ctx, c.Timeout)
	defer cancel()

	out, err := c.Run(cctx, "inspect", container)
	if err != nil {
		return nil, classifyExecError(container, err)
	}
	var results []map[string]any
	if err := json.Unmarshal(out, &results); err != nil {
		return nil, fmt.Errorf("%w: parse docker inspect: %v", provider.ErrProtocol, err)
	}
	if len(results) == 0 {
		return nil, fmt.Errorf("%w: container %q not found", provider.ErrNotFound, container)
	}
	return results[0], nil
}

// Version returns the docker server version (used by validate).
func (c *Client) Version(ctx context.Context) (string, error) {
	cctx, cancel := context.WithTimeout(ctx, c.Timeout)
	defer cancel()
	out, err := c.Run(cctx, "info", "--format", "{{.ServerVersion}}")
	if err != nil {
		return "", classifyExecError("info", err)
	}
	return strings.TrimSpace(string(out)), nil
}

// classifyExecError maps `docker` CLI failures to sentinel errors.
//
// docker exits with 1 on most errors, with stderr carrying the cause:
//   - "No such object" / "No such container" → NotFound
//   - "permission denied" / "connect: permission denied" → Forbidden
//   - "Cannot connect to the Docker daemon" → Transient (daemon down or DOCKER_HOST wrong)
//   - "context deadline exceeded" → Transient
func classifyExecError(target string, err error) error {
	var stderr []byte
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		stderr = ee.Stderr
	}
	msg := strings.ToLower(string(stderr) + " " + err.Error())

	switch {
	case strings.Contains(msg, "no such object"),
		strings.Contains(msg, "no such container"):
		return fmt.Errorf("%w: container %q not found", provider.ErrNotFound, target)
	case strings.Contains(msg, "permission denied"):
		return fmt.Errorf("%w: docker socket: %s", provider.ErrForbidden, firstLine(string(stderr)))
	case strings.Contains(msg, "cannot connect to the docker daemon"),
		strings.Contains(msg, "is the docker daemon running"):
		return fmt.Errorf("%w: docker daemon unreachable", provider.ErrTransient)
	case strings.Contains(msg, "context deadline exceeded"),
		strings.Contains(msg, "timeout"):
		return fmt.Errorf("%w: docker timeout", provider.ErrTransient)
	}
	return fmt.Errorf("%w: docker: %s", provider.ErrEnv, firstLine(string(stderr)))
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return strings.TrimSpace(s[:i])
	}
	return strings.TrimSpace(s)
}
