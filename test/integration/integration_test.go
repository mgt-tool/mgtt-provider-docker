//go:build integration

// Package integration exercises mgtt-provider-docker end-to-end against a
// real Docker daemon. The host's docker is used directly (no DinD) — we
// just create real containers, probe them, and clean them up.
//
// Run with:
//
//	go test -tags=integration ./test/integration/...
//
// Requirements on the host: docker, go. Tests are skipped when docker is
// unavailable.
package integration

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// We use alpine pinned by digest so the test image is reproducible (the
// :latest tag rolls and would silently shift behavior).
const alpineImage = "alpine:3.20@sha256:beefdbd8a1da6d2915566fde36db9db0b524eb737fc57cd1367effd16dc0d06d"

// ---------------------------------------------------------------------------
// Test lifecycle
// ---------------------------------------------------------------------------

func TestMain(m *testing.M) {
	if _, err := exec.LookPath("docker"); err != nil {
		fmt.Fprintln(os.Stderr, "docker not on PATH; skipping docker integration tests")
		os.Exit(0)
	}
	os.Exit(m.Run())
}

// ---------------------------------------------------------------------------
// Container fixtures — each test creates uniquely-named containers and
// cleans them up via t.Cleanup so a failed run doesn't leak state.
// ---------------------------------------------------------------------------

func uniqueName(t *testing.T, prefix string) string {
	t.Helper()
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	return "mgtt-it-" + prefix + "-" + hex.EncodeToString(b)
}

// startRunning spins up a long-running container and registers cleanup.
func startRunning(t *testing.T, name string) {
	t.Helper()
	out, err := exec.Command("docker", "run", "-d", "--name", name,
		alpineImage, "sleep", "3600").CombinedOutput()
	if err != nil {
		t.Fatalf("docker run %s: %v\n%s", name, err, out)
	}
	t.Cleanup(func() { _ = exec.Command("docker", "rm", "-f", name).Run() })
}

// startStopped runs a container that exits immediately with the given code.
func startStopped(t *testing.T, name string, exitCode int) {
	t.Helper()
	out, err := exec.Command("docker", "run", "-d", "--name", name,
		alpineImage, "sh", "-c", fmt.Sprintf("exit %d", exitCode)).CombinedOutput()
	if err != nil {
		t.Fatalf("docker run %s: %v\n%s", name, err, out)
	}
	t.Cleanup(func() { _ = exec.Command("docker", "rm", "-f", name).Run() })
	// Give docker a moment to record the exit.
	time.Sleep(500 * time.Millisecond)
}

// startCrashLooping runs a container with --restart=on-failure that exits
// non-zero, so the daemon restarts it repeatedly. After a few seconds
// RestartCount climbs.
func startCrashLooping(t *testing.T, name string) {
	t.Helper()
	out, err := exec.Command("docker", "run", "-d", "--name", name,
		"--restart", "on-failure", alpineImage, "sh", "-c", "exit 1").CombinedOutput()
	if err != nil {
		t.Fatalf("docker run %s: %v\n%s", name, err, out)
	}
	t.Cleanup(func() { _ = exec.Command("docker", "rm", "-f", name).Run() })
	// Wait until we've accumulated a few restarts (default backoff is 100ms,
	// 200ms, 400ms, ...). 5 seconds is enough for ~5 restarts.
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		out, _ := exec.Command("docker", "inspect", "--format", "{{.RestartCount}}", name).Output()
		rc := strings.TrimSpace(string(out))
		if rc != "0" && rc != "" {
			return
		}
		time.Sleep(500 * time.Millisecond)
	}
}

// ---------------------------------------------------------------------------
// Provider binary harness
// ---------------------------------------------------------------------------

func buildProviderBinary(t *testing.T) string {
	t.Helper()
	root := repoRoot(t)
	bin := filepath.Join(t.TempDir(), "mgtt-provider-docker")
	if runtime.GOOS == "windows" {
		bin += ".exe"
	}
	cmd := exec.Command("go", "build", "-o", bin, ".")
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go build provider: %v\n%s", err, out)
	}
	return bin
}

type probeResult struct {
	Value  any    `json:"value"`
	Raw    string `json:"raw"`
	Status string `json:"status"`
}

func probe(t *testing.T, binary, name, fact string) probeResult {
	t.Helper()
	cmd := exec.Command(binary, "probe", name, fact, "--type", "container")
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("probe %s/%s: %v\nstderr: %s", name, fact, err, stderr.String())
	}
	var r probeResult
	if err := json.Unmarshal(out, &r); err != nil {
		t.Fatalf("decode probe output: %v (raw=%q)", err, out)
	}
	return r
}

func probeAllowFail(t *testing.T, binary string, args ...string) (string, string, int) {
	t.Helper()
	cmd := exec.Command(binary, args...)
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	code := 0
	if ee, ok := err.(*exec.ExitError); ok {
		code = ee.ExitCode()
	} else if err != nil {
		t.Fatalf("run provider: %v", err)
	}
	return strings.TrimSpace(stdout.String()), strings.TrimSpace(stderr.String()), code
}

func repoRoot(t *testing.T) string {
	t.Helper()
	out, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		wd, _ := os.Getwd()
		return filepath.Join(wd, "..", "..")
	}
	return strings.TrimSpace(string(out))
}

// ---------------------------------------------------------------------------
// Scenario 1 — running container: positive facts on a healthy container
// ---------------------------------------------------------------------------

func TestScenario_RunningContainer(t *testing.T) {
	name := uniqueName(t, "running")
	startRunning(t, name)

	binary := buildProviderBinary(t)

	t.Run("running == true", func(t *testing.T) {
		r := probe(t, binary, name, "running")
		if v, _ := r.Value.(bool); !v {
			t.Fatalf("want running=true, got %v", r.Value)
		}
	})

	t.Run("restart_count == 0", func(t *testing.T) {
		r := probe(t, binary, name, "restart_count")
		v, _ := r.Value.(float64)
		if int(v) != 0 {
			t.Fatalf("want 0, got %v", r.Value)
		}
	})

	t.Run("exit_code == 0", func(t *testing.T) {
		r := probe(t, binary, name, "exit_code")
		v, _ := r.Value.(float64)
		if int(v) != 0 {
			t.Fatalf("want 0, got %v", r.Value)
		}
	})

	t.Run("uptime_seconds > 0", func(t *testing.T) {
		r := probe(t, binary, name, "uptime_seconds")
		v, _ := r.Value.(float64)
		if int(v) < 0 {
			t.Fatalf("want >= 0, got %v", r.Value)
		}
	})
}

// ---------------------------------------------------------------------------
// Scenario 2 — exited container with non-zero exit code
// ---------------------------------------------------------------------------

func TestScenario_ExitedNonZero(t *testing.T) {
	name := uniqueName(t, "crashed")
	startStopped(t, name, 137)

	binary := buildProviderBinary(t)

	t.Run("running == false", func(t *testing.T) {
		r := probe(t, binary, name, "running")
		if v, _ := r.Value.(bool); v {
			t.Fatalf("want running=false, got %v", r.Value)
		}
	})

	t.Run("exit_code == 137", func(t *testing.T) {
		r := probe(t, binary, name, "exit_code")
		v, _ := r.Value.(float64)
		if int(v) != 137 {
			t.Fatalf("want 137 (SIGKILL), got %v", r.Value)
		}
	})
}

// ---------------------------------------------------------------------------
// Scenario 3 — crash-looping container: restart_count climbs
// ---------------------------------------------------------------------------

func TestScenario_CrashLooping(t *testing.T) {
	name := uniqueName(t, "loop")
	startCrashLooping(t, name)

	binary := buildProviderBinary(t)
	r := probe(t, binary, name, "restart_count")
	v, _ := r.Value.(float64)
	if int(v) < 1 {
		t.Fatalf("want restart_count >= 1 after crash-loop window, got %v", r.Value)
	}
}

// ---------------------------------------------------------------------------
// Scenario 4 — missing container: status:not_found in JSON output
// ---------------------------------------------------------------------------

func TestScenario_MissingContainer_StatusNotFound(t *testing.T) {
	binary := buildProviderBinary(t)
	r := probe(t, binary, "container-that-was-never-created-"+uniqueName(t, "x"), "running")
	if r.Status != "not_found" {
		t.Fatalf("missing container should yield status:not_found, got %q (value=%v)", r.Status, r.Value)
	}
}

// ---------------------------------------------------------------------------
// Scenario 5 — usage errors must surface as exit 1
// ---------------------------------------------------------------------------

func TestScenario_UnknownFact_ErrUsage(t *testing.T) {
	binary := buildProviderBinary(t)
	_, _, code := probeAllowFail(t, binary,
		"probe", "anything", "no_such_fact",
		"--type", "container",
	)
	if code != 1 {
		t.Fatalf("unknown fact: want exit 1, got %d", code)
	}
}

func TestScenario_UnknownType_ErrUsage(t *testing.T) {
	binary := buildProviderBinary(t)
	_, _, code := probeAllowFail(t, binary,
		"probe", "anything", "running",
		"--type", "no_such_type",
	)
	if code != 1 {
		t.Fatalf("unknown type: want exit 1, got %d", code)
	}
}

var _ = context.Background // keep imports
