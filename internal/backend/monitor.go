package backend

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"
)

// rolloutTimeout bounds each deployment's readiness watch — the
// original installer's 150s.
const rolloutTimeout = 150 * time.Second

// MonitorDeployments watches the release's Deployments until they are
// ready, streaming each rollout's live progress.
func MonitorDeployments(ctx context.Context, k Kubectl, shell ProcessRunner, c Config, stdout, stderr io.Writer) error {
	fmt.Fprintf(stdout, "Waiting for deployments of release '%s' in '%s' to be ready\n", c.ReleaseName, c.Namespace)

	type manifestObject struct {
		Kind     string `yaml:"kind"`
		Metadata struct {
			Name string `yaml:"name"`
		} `yaml:"metadata"`
	}
	objects, err := runHelmYAML[[]manifestObject](ctx, shell, []string{
		"get", "manifest", "--kube-context", c.KubeContext, "-n", c.Namespace, c.ReleaseName, //nolint:goconst
	})
	if err != nil {
		return err
	}

	// TODO: lo.FilterMap here
	var deployments []string
	for _, obj := range objects {
		if obj.Kind == "Deployment" {
			deployments = append(deployments, "deployment.apps/"+obj.Metadata.Name)
		}
	}
	if len(deployments) == 0 {
		fmt.Fprintf(stdout, "No deployments found for release '%s'\n", c.ReleaseName)

		return nil
	}

	fmt.Fprintf(stdout, "Monitoring: %s\n", strings.Join(deployments, " "))

	var failed []string
	for _, dep := range deployments {
		if err := k.RolloutStatus(ctx, c.KubeContext, c.Namespace, dep, rolloutTimeout, stdout, stderr); err != nil {
			failed = append(failed, dep)
		}
	}
	if len(failed) > 0 {
		return fmt.Errorf("deployment(s) not ready: %s", strings.Join(failed, ", "))
	}

	return nil
}
