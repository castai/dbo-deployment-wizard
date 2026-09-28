package backend

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"os/exec"
	"strings"

	"gopkg.in/yaml.v3"
)

// Kubectl abstracts the kubectl shell-outs
type Kubectl interface {
	ListKubeContexts(ctx context.Context) ([]string, error)
	CurrentKubeContext(ctx context.Context) (string, error)
	ListSecrets(ctx context.Context, kubeContext, namespace string) ([]string, error)
	HelmReleaseExists(ctx context.Context, kubeContext, namespace, releaseName string) (bool, error)
	EnsureSecret(ctx context.Context, kubeContext, namespace, name string, data map[string]string) error
}

// RealKubectl invokes the kubectl binary on PATH. SimulateSlowNetwork
// delays every call by simulateSlowNetworkDelay — the --slow-network
// UI testing aid.
type RealKubectl struct {
	SimulateSlowNetwork bool

	// releaseCache memoizes HelmReleaseExists results by release
	// coordinates; the UI re-checks on every review render.
	releaseCache map[releaseKey]releaseLookup
}

// releaseKey identifies a helm release by its coordinates: the
// namespace on a kubectl context.
type releaseKey struct {
	kubeContext string
	namespace   string
	releaseName string
}

// releaseLookup is one memoized HelmReleaseExists result; err is set
// when the lookup failed.
type releaseLookup struct {
	exists bool
	err    error
}

// run executes `kubectl <args>`
func (k *RealKubectl) run(ctx context.Context, args ...string) ([]byte, error) {
	if k.SimulateSlowNetwork {
		if err := sleepCtx(ctx, simulateSlowNetworkDelay); err != nil {
			return nil, err
		}
	}

	cmd := exec.CommandContext(ctx, "kubectl", args...)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		// Context cancellation/timeout looks different from a real kubectl error —
		// surface it distinctly rather than "signal: killed".
		if ctx.Err() != nil {
			return stdout.Bytes(), fmt.Errorf("kubectl %s: %w", strings.Join(args, " "), ctx.Err())
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

// ListKubeContexts returns every context name; an empty result is an
// error.
func (k *RealKubectl) ListKubeContexts(ctx context.Context) ([]string, error) {
	out, err := k.run(ctx, "config", "get-contexts", "-o", "name")
	if err != nil {
		return nil, err
	}
	var contexts []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			contexts = append(contexts, line)
		}
	}
	if len(contexts) == 0 {
		return nil, ErrNoKubeContexts
	}

	return contexts, nil
}

// CurrentKubeContext returns the current context; an empty result
// with no error is possible.
func (k *RealKubectl) CurrentKubeContext(ctx context.Context) (string, error) {
	out, err := k.run(ctx, "config", "current-context")
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(string(out)), nil
}

// ListSecrets returns the names of every Secret in namespace on
// kubeContext.
func (k *RealKubectl) ListSecrets(ctx context.Context, kubeContext, namespace string) ([]string, error) {
	out, err := k.run(ctx, "--context", kubeContext, "-n", namespace,
		"get", "secret", "-o", "jsonpath={range .items[*]}{.metadata.name}{\"\\n\"}{end}")
	if err != nil {
		return nil, err
	}
	var secrets []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			secrets = append(secrets, line)
		}
	}

	return secrets, nil
}

// HelmReleaseExists reports whether namespace on kubeContext holds a
// helm release named releaseName: a missing namespace answers "no"
// outright — helm would create it on install — otherwise any
// revision Secret in any status counts, since helm upgrade targets
// failed releases too. Results are memoized by release coordinates —
// failures included, so a broken lookup doesn't re-shell-out on
// every render.
func (k *RealKubectl) HelmReleaseExists(ctx context.Context, kubeContext, namespace, releaseName string) (bool, error) {
	key := releaseKey{kubeContext: kubeContext, namespace: namespace, releaseName: releaseName}
	if cached, ok := k.releaseCache[key]; ok {
		return cached.exists, cached.err
	}

	result := k.lookupHelmRelease(ctx, kubeContext, namespace, releaseName)
	if k.releaseCache == nil {
		k.releaseCache = map[releaseKey]releaseLookup{}
	}
	k.releaseCache[key] = result

	return result.exists, result.err
}

// lookupHelmRelease is the uncached release lookup.
func (k *RealKubectl) lookupHelmRelease(ctx context.Context, kubeContext, namespace, releaseName string) releaseLookup {
	nsOut, err := k.run(ctx, "--context", kubeContext,
		"get", "namespace", namespace, "--ignore-not-found", "-o", "name")
	if err != nil {
		return releaseLookup{err: err}
	}
	if strings.TrimSpace(string(nsOut)) == "" {
		return releaseLookup{}
	}

	out, err := k.run(ctx, "--context", kubeContext, "-n", namespace,
		"get", "secret", "-l", "owner=helm,name="+releaseName,
		"-o", "jsonpath={.items[*].metadata.name}")
	if err != nil {
		return releaseLookup{err: err}
	}

	return releaseLookup{exists: strings.TrimSpace(string(out)) != ""}
}

// secretManifest is the kubectl-applied Secret schema.
type secretManifest struct {
	APIVersion string            `yaml:"apiVersion"`
	Kind       string            `yaml:"kind"`
	Metadata   secretMetadata    `yaml:"metadata"`
	Type       string            `yaml:"type"`
	Data       map[string]string `yaml:"data"`
}

type secretMetadata struct {
	Name string `yaml:"name"`
}

// EnsureSecret idempotently creates or updates a Secret holding the
// given data. The manifest travels via stdin, so the values never
// appear on the argv.
func (k *RealKubectl) EnsureSecret(ctx context.Context, kubeContext, namespace, name string, data map[string]string) error {
	if k.SimulateSlowNetwork {
		if err := sleepCtx(ctx, simulateSlowNetworkDelay); err != nil {
			return err
		}
	}
	encoded := make(map[string]string, len(data))
	for key, value := range data {
		encoded[key] = base64.StdEncoding.EncodeToString([]byte(value))
	}

	manifest, err := yaml.Marshal(secretManifest{
		APIVersion: "v1",
		Kind:       "Secret",
		Metadata:   secretMetadata{Name: name},
		Type:       "Opaque",
		Data:       encoded,
	})
	if err != nil {
		return fmt.Errorf("render secret manifest: %w", err)
	}

	cmd := exec.CommandContext(ctx, "kubectl", "--context", kubeContext, "-n", namespace, "apply", "-f", "-")
	cmd.Stdin = bytes.NewReader(manifest)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("kubectl apply secret %s: %w: %s", name, err, out)
	}

	return nil
}

// ErrNoKubeContexts is returned by ListKubeContexts when kubectl
// reports no configured contexts.
var ErrNoKubeContexts = errors.New("kubectl reported no contexts")
