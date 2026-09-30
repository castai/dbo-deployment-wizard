package backend

import (
	"context"
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"
)

// rolloutTimeout bounds each deployment's readiness watch — the
// original installer's 150s.
const rolloutTimeout = 150 * time.Second

// deploymentNameLine matches the metadata.name line directly under a
// top-level object — the old installer's /^  name:/ awk pattern.
var deploymentNameLine = regexp.MustCompile(`^  name: (.*)$`)

// MonitorDeployments watches the release's Deployments until they are
// ready, streaming each rollout's live progress — the old installer's
// post-upgrade step.
// TODO: redo with json responses
func MonitorDeployments(ctx context.Context, k Kubectl, shell ProcessRunner, c Config, stdout, stderr io.Writer) error {
	fmt.Fprintf(stdout, "Waiting for deployments of release '%s' in '%s' to be ready\n", c.ReleaseName, c.Namespace)

	manifest, err := shell.Run(ctx, "helm", nil, []string{
		"get", "manifest", "--kube-context", c.KubeContext, "-n", c.Namespace, c.ReleaseName,
	})
	if err != nil {
		return fmt.Errorf("get release manifest: %w", err)
	}

	deployments := deploymentNames(string(manifest))
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

// deploymentNames extracts the top-level Deployment object names from
// a rendered manifest.
func deploymentNames(manifest string) []string {
	var names []string
	found := false
	for _, line := range strings.Split(manifest, "\n") {
		if line == "kind: Deployment" {
			found = true

			continue
		}
		if m := deploymentNameLine.FindStringSubmatch(line); found && m != nil {
			names = append(names, "deployment.apps/"+strings.TrimSpace(m[1]))
			found = false
		}
	}

	return names
}
