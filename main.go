package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

type ProbeResult struct {
	Value any    `json:"value"`
	Raw   string `json:"raw"`
}

type ValidateResult struct {
	OK     bool   `json:"ok"`
	Auth   string `json:"auth"`
	Access string `json:"access"`
}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "usage: mgtt-provider-docker <command> [args]\n")
		fmt.Fprintf(os.Stderr, "commands: probe, validate, describe\n")
		os.Exit(1)
	}

	switch os.Args[1] {
	case "probe":
		if len(os.Args) < 4 {
			fmt.Fprintf(os.Stderr, "usage: mgtt-provider-docker probe <container> <fact> [flags]\n")
			os.Exit(1)
		}
		result, err := probe(os.Args[2], os.Args[3])
		if err != nil {
			fmt.Fprintf(os.Stderr, "probe error: %v\n", err)
			os.Exit(1)
		}
		json.NewEncoder(os.Stdout).Encode(result)

	case "validate":
		result := validate()
		json.NewEncoder(os.Stdout).Encode(result)
		if !result.OK {
			os.Exit(1)
		}

	case "describe":
		fmt.Println(`{"types": ["container"]}`)

	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", os.Args[1])
		os.Exit(1)
	}
}

func validate() *ValidateResult {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "docker", "info", "--format", "{{.ServerVersion}}")
	out, err := cmd.Output()
	if err != nil {
		return &ValidateResult{OK: false, Auth: "docker not reachable", Access: "none"}
	}
	version := strings.TrimSpace(string(out))
	return &ValidateResult{OK: true, Auth: fmt.Sprintf("docker %s", version), Access: "read-only"}
}

func probe(container, fact string) (*ProbeResult, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	data, err := dockerInspect(ctx, container)
	if err != nil {
		return nil, err
	}

	state, _ := data["State"].(map[string]any)
	if state == nil {
		return nil, fmt.Errorf("container %q: no State in inspect output", container)
	}

	switch fact {
	case "running":
		running, _ := state["Running"].(bool)
		return &ProbeResult{Value: running, Raw: fmt.Sprintf("%v", running)}, nil

	case "restart_count":
		rc := jsonInt(data, "RestartCount")
		return &ProbeResult{Value: rc, Raw: fmt.Sprintf("%d", rc)}, nil

	case "health_status":
		health, _ := state["Health"].(map[string]any)
		if health == nil {
			return &ProbeResult{Value: "none", Raw: "none"}, nil
		}
		status, _ := health["Status"].(string)
		return &ProbeResult{Value: status, Raw: status}, nil

	case "exit_code":
		ec := jsonInt(state, "ExitCode")
		return &ProbeResult{Value: ec, Raw: fmt.Sprintf("%d", ec)}, nil

	default:
		return nil, fmt.Errorf("unknown fact: %s", fact)
	}
}

func dockerInspect(ctx context.Context, container string) (map[string]any, error) {
	cmd := exec.CommandContext(ctx, "docker", "inspect", container)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("docker inspect %s: %w", container, err)
	}

	var results []map[string]any
	if err := json.Unmarshal(out, &results); err != nil {
		return nil, fmt.Errorf("parse docker inspect: %w", err)
	}
	if len(results) == 0 {
		return nil, fmt.Errorf("container %q not found", container)
	}
	return results[0], nil
}

func jsonInt(data map[string]any, key string) int {
	switch v := data[key].(type) {
	case float64:
		return int(v)
	case int:
		return v
	}
	return 0
}
