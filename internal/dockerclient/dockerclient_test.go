package dockerclient

import (
	"context"
	"errors"
	"os/exec"
	"strings"
	"testing"

	"github.com/mgt-tool/mgtt/sdk/provider"
)

// fakeExitErr returns an *exec.ExitError-shaped error with the given stderr,
// because that's what exec.CommandContext.Output() returns on non-zero exit.
func fakeExitErr(stderr string) error {
	return &exec.ExitError{Stderr: []byte(stderr)}
}

func TestInspect_HappyPath(t *testing.T) {
	c := New()
	c.Run = func(ctx context.Context, args ...string) ([]byte, error) {
		return []byte(`[{"State":{"Running":true,"ExitCode":0}}]`), nil
	}
	data, err := c.Inspect(context.Background(), "x")
	if err != nil {
		t.Fatal(err)
	}
	state := data["State"].(map[string]any)
	if state["Running"] != true {
		t.Fatalf("want Running=true, got %v", state["Running"])
	}
}

func TestInspect_EmptyArrayIsNotFound(t *testing.T) {
	c := New()
	c.Run = func(ctx context.Context, args ...string) ([]byte, error) {
		return []byte(`[]`), nil
	}
	_, err := c.Inspect(context.Background(), "missing")
	if !errors.Is(err, provider.ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}

func TestInspect_BadJSONIsProtocol(t *testing.T) {
	c := New()
	c.Run = func(ctx context.Context, args ...string) ([]byte, error) {
		return []byte(`not json`), nil
	}
	_, err := c.Inspect(context.Background(), "x")
	if !errors.Is(err, provider.ErrProtocol) {
		t.Fatalf("want ErrProtocol, got %v", err)
	}
}

func TestClassify_NoSuchContainer_NotFound(t *testing.T) {
	cases := []string{
		"Error: No such container: foo",
		"Error: No such object: bar",
		"Error response from daemon: No such container: baz",
	}
	for _, msg := range cases {
		c := New()
		c.Run = func(ctx context.Context, args ...string) ([]byte, error) {
			return nil, fakeExitErr(msg)
		}
		_, err := c.Inspect(context.Background(), "x")
		if !errors.Is(err, provider.ErrNotFound) {
			t.Errorf("stderr %q: want ErrNotFound, got %v", msg, err)
		}
	}
}

func TestClassify_DaemonDown_Transient(t *testing.T) {
	cases := []string{
		"Cannot connect to the Docker daemon at unix:///var/run/docker.sock. Is the docker daemon running?",
		"error during connect: ... Is the docker daemon running?",
	}
	for _, msg := range cases {
		c := New()
		c.Run = func(ctx context.Context, args ...string) ([]byte, error) {
			return nil, fakeExitErr(msg)
		}
		_, err := c.Inspect(context.Background(), "x")
		if !errors.Is(err, provider.ErrTransient) {
			t.Errorf("stderr %q: want ErrTransient, got %v", msg, err)
		}
	}
}

func TestClassify_PermissionDenied_Forbidden(t *testing.T) {
	c := New()
	c.Run = func(ctx context.Context, args ...string) ([]byte, error) {
		return nil, fakeExitErr("permission denied while trying to connect to the Docker daemon socket")
	}
	_, err := c.Inspect(context.Background(), "x")
	if !errors.Is(err, provider.ErrForbidden) {
		t.Fatalf("want ErrForbidden, got %v", err)
	}
}

func TestClassify_TimeoutTransient(t *testing.T) {
	c := New()
	c.Run = func(ctx context.Context, args ...string) ([]byte, error) {
		return nil, errors.New("context deadline exceeded")
	}
	_, err := c.Inspect(context.Background(), "x")
	if !errors.Is(err, provider.ErrTransient) {
		t.Fatalf("want ErrTransient, got %v", err)
	}
}

func TestVersion(t *testing.T) {
	c := New()
	c.Run = func(ctx context.Context, args ...string) ([]byte, error) {
		// docker info --format prints just the version
		if strings.Contains(strings.Join(args, " "), "info") {
			return []byte("25.0.5\n"), nil
		}
		return nil, errors.New("unexpected args")
	}
	v, err := c.Version(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if v != "25.0.5" {
		t.Fatalf("want 25.0.5, got %q", v)
	}
}
