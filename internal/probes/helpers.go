// Package probes implements the docker-provider probe surface. All plumbing
// (argv parsing, exit codes, status:not_found translation) lives in the SDK;
// this package only constructs docker inspect calls and parses results.
package probes

import (
	"time"

	"github.com/mgt-tool/mgtt/sdk/provider"
	"github.com/sajonaro/mgtt-provider-docker/internal/dockerclient"
)

// NewDockerConstructor is overridable for tests.
var NewDockerConstructor = func() *dockerclient.Client {
	return dockerclient.New()
}

// requireExtra is currently unused — container facts only need `name` which
// the SDK already provides via Request.Name. Kept for future facts that may
// take additional flags.
func requireExtra(req provider.Request, key, typeName string) (string, error) {
	if v := req.Extra[key]; v != "" {
		return v, nil
	}
	return "", &usageError{key: key, typeName: typeName}
}

type usageError struct {
	key, typeName string
}

func (e *usageError) Error() string {
	return e.typeName + " requires --" + e.key + " <value>"
}
func (e *usageError) Unwrap() error { return provider.ErrUsage }

// jsonInt extracts an integer from a JSON map, handling both float64
// (the default JSON number type) and int.
func jsonInt(data map[string]any, key string) int {
	switch v := data[key].(type) {
	case float64:
		return int(v)
	case int:
		return v
	}
	return 0
}

// stringFromMap extracts a string from a nested JSON map at path[0]…path[n-1],
// returning fallback when any segment is absent.
func stringFromMap(data map[string]any, fallback string, path ...string) string {
	cur := data
	for i, key := range path {
		if i == len(path)-1 {
			if s, ok := cur[key].(string); ok {
				return s
			}
			return fallback
		}
		next, ok := cur[key].(map[string]any)
		if !ok {
			return fallback
		}
		cur = next
	}
	return fallback
}

// boolFromMap extracts a bool from a nested JSON map at path[0]…path[n-1].
func boolFromMap(data map[string]any, path ...string) bool {
	cur := data
	for i, key := range path {
		if i == len(path)-1 {
			b, _ := cur[key].(bool)
			return b
		}
		next, ok := cur[key].(map[string]any)
		if !ok {
			return false
		}
		cur = next
	}
	return false
}

// parseUptimeSeconds parses docker's RFC3339 StartedAt timestamp and returns
// seconds since start. Returns 0 for empty / unparseable input or for
// containers that have never started.
func parseUptimeSeconds(startedAt string) int {
	if startedAt == "" || startedAt == "0001-01-01T00:00:00Z" {
		return 0
	}
	t, err := time.Parse(time.RFC3339Nano, startedAt)
	if err != nil {
		return 0
	}
	d := time.Since(t).Seconds()
	if d < 0 {
		return 0
	}
	return int(d)
}

// Register adds the docker provider's types to the registry.
func Register(r *provider.Registry) {
	registerContainer(r)
}
