package probes

import (
	"context"
	"fmt"

	"github.com/mgt-tool/mgtt/sdk/provider"
)

// container facts inspect one Docker container by name. The container name
// comes from req.Name (the SDK's standard component identifier); facts beyond
// the basic four would take additional --extras here.
//
// Sentinel error mapping:
//   - container missing      → ErrNotFound  (status: not_found in JSON)
//   - docker daemon down     → ErrTransient
//   - permission denied      → ErrForbidden
//   - parse failure          → ErrProtocol
func registerContainer(r *provider.Registry) {
	r.Register("container", map[string]provider.ProbeFn{
		"running": func(ctx context.Context, req provider.Request) (provider.Result, error) {
			data, err := inspect(ctx, req)
			if err != nil {
				return provider.Result{}, err
			}
			return provider.BoolResult(boolFromMap(data, "State", "Running")), nil
		},

		"restart_count": func(ctx context.Context, req provider.Request) (provider.Result, error) {
			data, err := inspect(ctx, req)
			if err != nil {
				return provider.Result{}, err
			}
			return provider.IntResult(jsonInt(data, "RestartCount")), nil
		},

		"health_status": func(ctx context.Context, req provider.Request) (provider.Result, error) {
			data, err := inspect(ctx, req)
			if err != nil {
				return provider.Result{}, err
			}
			// Containers without HEALTHCHECK have no Health block — return
			// "none" so models can write `health_status == "healthy"` without
			// needing a separate "is healthcheck configured?" probe.
			s := stringFromMap(data, "none", "State", "Health", "Status")
			return provider.StringResult(s), nil
		},

		"exit_code": func(ctx context.Context, req provider.Request) (provider.Result, error) {
			data, err := inspect(ctx, req)
			if err != nil {
				return provider.Result{}, err
			}
			state, _ := data["State"].(map[string]any)
			if state == nil {
				return provider.Result{}, fmt.Errorf("%w: container %q: no State in inspect output",
					provider.ErrProtocol, req.Name)
			}
			return provider.IntResult(jsonInt(state, "ExitCode")), nil
		},

		"uptime_seconds": func(ctx context.Context, req provider.Request) (provider.Result, error) {
			data, err := inspect(ctx, req)
			if err != nil {
				return provider.Result{}, err
			}
			startedAt := stringFromMap(data, "", "State", "StartedAt")
			return provider.IntResult(parseUptimeSeconds(startedAt)), nil
		},
	})
}

// inspect resolves the container name from req.Name and runs `docker inspect`.
func inspect(ctx context.Context, req provider.Request) (map[string]any, error) {
	if req.Name == "" {
		return nil, fmt.Errorf("%w: container facts require a component name", provider.ErrUsage)
	}
	c := NewDockerConstructor()
	return c.Inspect(ctx, req.Name)
}
