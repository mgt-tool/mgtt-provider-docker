package probes

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/mgt-tool/mgtt/sdk/provider"
	"github.com/sajonaro/mgtt-provider-docker/internal/dockerclient"
)

// fakeDocker swaps NewDockerConstructor for the duration of one test, with
// a Run function that returns the provided JSON inspect output (or an error).
func fakeDocker(t *testing.T, inspectJSON string, runErr error) {
	t.Helper()
	prev := NewDockerConstructor
	t.Cleanup(func() { NewDockerConstructor = prev })
	NewDockerConstructor = func() *dockerclient.Client {
		c := dockerclient.New()
		c.Run = func(ctx context.Context, args ...string) ([]byte, error) {
			if runErr != nil {
				return nil, runErr
			}
			return []byte(inspectJSON), nil
		}
		return c
	}
}

func runProbe(t *testing.T, name, fact string) provider.Result {
	t.Helper()
	r := provider.NewRegistry()
	Register(r)
	res, err := r.Probe(context.Background(), provider.Request{
		Type: "container", Name: name, Fact: fact,
	})
	if err != nil {
		t.Fatalf("probe %s: %v", fact, err)
	}
	return res
}

func runProbeErr(t *testing.T, name, fact string) error {
	t.Helper()
	r := provider.NewRegistry()
	Register(r)
	_, err := r.Probe(context.Background(), provider.Request{
		Type: "container", Name: name, Fact: fact,
	})
	return err
}

const runningContainerJSON = `[{
	"State": {
		"Running": true,
		"ExitCode": 0,
		"StartedAt": "2026-04-16T10:00:00Z",
		"Health": {"Status": "healthy"}
	},
	"RestartCount": 2
}]`

const stoppedContainerJSON = `[{
	"State": {
		"Running": false,
		"ExitCode": 137,
		"StartedAt": "2026-04-16T10:00:00Z"
	},
	"RestartCount": 0
}]`

const noHealthcheckJSON = `[{
	"State": {"Running": true, "ExitCode": 0, "StartedAt": "2026-04-16T10:00:00Z"},
	"RestartCount": 0
}]`

// ----- Happy path coverage -----

func TestRunning_True(t *testing.T) {
	fakeDocker(t, runningContainerJSON, nil)
	if v, _ := runProbe(t, "x", "running").Value.(bool); !v {
		t.Fatal("want running=true")
	}
}

func TestRunning_False(t *testing.T) {
	fakeDocker(t, stoppedContainerJSON, nil)
	if v, _ := runProbe(t, "x", "running").Value.(bool); v {
		t.Fatal("want running=false")
	}
}

func TestRestartCount(t *testing.T) {
	fakeDocker(t, runningContainerJSON, nil)
	if v, _ := runProbe(t, "x", "restart_count").Value.(int); v != 2 {
		t.Fatalf("want 2, got %v", v)
	}
}

func TestExitCode_FromState(t *testing.T) {
	fakeDocker(t, stoppedContainerJSON, nil)
	if v, _ := runProbe(t, "x", "exit_code").Value.(int); v != 137 {
		t.Fatalf("want 137 (SIGKILL), got %v", v)
	}
}

func TestHealthStatus_Configured(t *testing.T) {
	fakeDocker(t, runningContainerJSON, nil)
	if v, _ := runProbe(t, "x", "health_status").Value.(string); v != "healthy" {
		t.Fatalf("want 'healthy', got %q", v)
	}
}

func TestHealthStatus_NoHealthcheckReturnsNone(t *testing.T) {
	fakeDocker(t, noHealthcheckJSON, nil)
	if v, _ := runProbe(t, "x", "health_status").Value.(string); v != "none" {
		t.Fatalf("want 'none' for unconfigured HEALTHCHECK, got %q", v)
	}
}

func TestUptimeSeconds_Positive(t *testing.T) {
	pastJSON := `[{"State":{"Running":true,"StartedAt":"` +
		time.Now().Add(-90*time.Second).Format(time.RFC3339Nano) +
		`"},"RestartCount":0}]`
	fakeDocker(t, pastJSON, nil)
	v, _ := runProbe(t, "x", "uptime_seconds").Value.(int)
	if v < 80 || v > 100 {
		t.Fatalf("want ~90s, got %v", v)
	}
}

func TestUptimeSeconds_NeverStartedReturnsZero(t *testing.T) {
	never := `[{"State":{"Running":false,"StartedAt":"0001-01-01T00:00:00Z"},"RestartCount":0}]`
	fakeDocker(t, never, nil)
	if v, _ := runProbe(t, "x", "uptime_seconds").Value.(int); v != 0 {
		t.Fatalf("want 0 for never-started container, got %v", v)
	}
}

// ----- Sentinel error mapping (smoke; full coverage in dockerclient_test) -----

func TestMissingContainerSurfaces_StatusNotFound(t *testing.T) {
	// SDK converts ErrNotFound to a Result with Status:not_found rather than
	// returning err — so a missing container shows up as a probe success
	// with status set, not as an error. The model's `unresolved` rules then
	// drive the engine.
	fakeDocker(t, `[]`, nil)
	res := runProbe(t, "missing", "running")
	if res.Status != "not_found" {
		t.Fatalf("want Status=not_found, got %q (value=%v)", res.Status, res.Value)
	}
}

func TestEmptyComponentName_ErrUsage(t *testing.T) {
	fakeDocker(t, runningContainerJSON, nil)
	err := runProbeErr(t, "", "running")
	if !errors.Is(err, provider.ErrUsage) {
		t.Fatalf("want ErrUsage, got %v", err)
	}
}

// ----- Registry wiring -----

func TestRegistryWiresContainer(t *testing.T) {
	r := provider.NewRegistry()
	Register(r)
	wantFacts := []string{"running", "restart_count", "health_status", "exit_code", "uptime_seconds"}
	got := r.Facts("container")
	gotSet := map[string]bool{}
	for _, f := range got {
		gotSet[f] = true
	}
	for _, w := range wantFacts {
		if !gotSet[w] {
			t.Errorf("registry missing container/%s", w)
		}
	}
}
