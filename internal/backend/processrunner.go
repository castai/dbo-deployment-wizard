package backend

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"
)

// ProcessRunner is the thin exec layer: run command,
// get back stdout or an error.
type ProcessRunner interface {
	Run(ctx context.Context, program string, stdin []byte, args []string) ([]byte, error)
}

type RealProcessRunner struct {
	SimulateSlowNetwork bool
}

func (s *RealProcessRunner) Run(ctx context.Context, program string, stdin []byte, args []string) ([]byte, error) {
	if s.SimulateSlowNetwork {
		if err := sleepCtx(ctx, simulateSlowNetworkDelay); err != nil {
			return nil, err
		}
	}

	cmd := exec.CommandContext(ctx, program, args...)
	if stdin != nil {
		cmd.Stdin = bytes.NewReader(stdin)
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		// Context cancellation/timeout looks different from a real command error —
		// surface it distinctly rather than "signal: killed".
		if ctx.Err() != nil {
			return stdout.Bytes(), fmt.Errorf("%s %s: %w", program, strings.Join(args, " "), ctx.Err())
		}

		msg := strings.TrimSpace(lastLine(stderr.String()))
		if msg == "" {
			msg = strings.TrimSpace(lastLine(stdout.String()))
		}
		if msg != "" {
			return stdout.Bytes(), errors.New(msg)
		}

		return stdout.Bytes(), err
	}

	return stdout.Bytes(), nil
}

func lastLine(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")

	return lines[len(lines)-1]
}

// ErrHelmReleaseNotFound is helm's not-found answer — a missing
// release and a missing namespace both produce it.
var ErrHelmReleaseNotFound = errors.New("release: not found")

// runHelm runs helm through the shell, translating its not-found
// answer into ErrHelmReleaseNotFound.
func runHelm(ctx context.Context, shell ProcessRunner, stdin []byte, args []string) ([]byte, error) {
	out, err := shell.Run(ctx, "helm", stdin, args)
	if err != nil && strings.Contains(err.Error(), ErrHelmReleaseNotFound.Error()) {
		return out, ErrHelmReleaseNotFound
	}

	return out, err
}

// runHelmJSON runs helm and unmarshals its JSON output; the caller's
// T decides the shape — a struct or a list.
func runHelmJSON[T any](ctx context.Context, shell ProcessRunner, args []string) (T, error) {
	var result T
	out, err := runHelm(ctx, shell, nil, slices.Concat(args, []string{"-o", "json"}))
	if err != nil {
		return result, err
	}

	if err := json.Unmarshal(out, &result); err != nil {
		return result, fmt.Errorf("parse helm result: %w", err)
	}

	return result, nil
}

// runHelmYAML runs helm and decodes its multi-document YAML output
// into the caller's list type, one element per document.
func runHelmYAML[T ~[]E, E any](ctx context.Context, shell ProcessRunner, args []string) (T, error) {
	var result T
	out, err := runHelm(ctx, shell, nil, args)
	if err != nil {
		return result, err
	}

	decoder := yaml.NewDecoder(bytes.NewReader(out))
	for {
		var obj E
		err := decoder.Decode(&obj)
		if errors.Is(err, io.EOF) {
			return result, nil
		}
		if err != nil {
			return result, fmt.Errorf("parse helm result: %w", err)
		}
		result = append(result, obj)
	}
}
