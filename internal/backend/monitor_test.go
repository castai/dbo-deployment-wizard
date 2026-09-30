package backend

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	backendmocks "github.com/castai/dbo-deployment-wizard/mocks/backend"
)

func TestMonitorDeployments(t *testing.T) {
	cfg := Config{
		ReleaseName: "castai-dbo",
		Namespace:   "castai-db-optimizer",
		KubeContext: "kind-test",
	}

	// shellWithManifest stubs the get-manifest run with the manifest.
	shellWithManifest := func(t *testing.T, manifest string) *backendmocks.MockProcessRunner {
		sh := backendmocks.NewMockProcessRunner(t)
		sh.EXPECT().Run(mock.Anything, "helm", mock.Anything, []string{
			"get", "manifest", "--kube-context", "kind-test", "-n", "castai-db-optimizer", "castai-dbo",
		}).Return([]byte(manifest), nil)

		return sh
	}

	twoDeployments := "---\nkind: Deployment\nmetadata:\n  name: db-agent\n---\nkind: Deployment\nmetadata:\n  name: db-proxy\n"

	t.Run("watches every deployment", func(t *testing.T) {
		k := backendmocks.NewMockKubectl(t)
		k.EXPECT().RolloutStatus(mock.Anything, "kind-test", "castai-db-optimizer",
			"deployment.apps/db-agent", rolloutTimeout, mock.Anything, mock.Anything).Return(nil)
		k.EXPECT().RolloutStatus(mock.Anything, "kind-test", "castai-db-optimizer",
			"deployment.apps/db-proxy", rolloutTimeout, mock.Anything, mock.Anything).Return(nil)

		var out bytes.Buffer
		err := MonitorDeployments(context.Background(), k, shellWithManifest(t, twoDeployments), cfg, &out, &out)
		require.NoError(t, err)
		require.Contains(t, out.String(), "Waiting for deployments of release 'castai-dbo' in 'castai-db-optimizer' to be ready")
		require.Contains(t, out.String(), "Monitoring: deployment.apps/db-agent deployment.apps/db-proxy")
	})

	t.Run("no deployments", func(t *testing.T) {
		k := backendmocks.NewMockKubectl(t)

		var out bytes.Buffer
		err := MonitorDeployments(context.Background(), k,
			shellWithManifest(t, "---\nkind: ConfigMap\nmetadata:\n  name: cfg\n"), cfg, &out, &out)
		require.NoError(t, err)
		require.Contains(t, out.String(), "No deployments found for release 'castai-dbo'")
	})

	t.Run("collects failed deployments", func(t *testing.T) {
		k := backendmocks.NewMockKubectl(t)
		k.EXPECT().RolloutStatus(mock.Anything, "kind-test", "castai-db-optimizer",
			"deployment.apps/db-agent", rolloutTimeout, mock.Anything, mock.Anything).Return(nil)
		k.EXPECT().RolloutStatus(mock.Anything, "kind-test", "castai-db-optimizer",
			"deployment.apps/db-proxy", rolloutTimeout, mock.Anything, mock.Anything).
			Return(errors.New("error: timed out waiting for the condition"))

		var out bytes.Buffer
		err := MonitorDeployments(context.Background(), k, shellWithManifest(t, twoDeployments), cfg, &out, &out)
		require.EqualError(t, err, "deployment(s) not ready: deployment.apps/db-proxy")
	})

	t.Run("malformed manifest", func(t *testing.T) {
		k := backendmocks.NewMockKubectl(t)

		var out bytes.Buffer
		err := MonitorDeployments(context.Background(), k, shellWithManifest(t, "not yaml: ["), cfg, &out, &out)
		require.ErrorContains(t, err, "parse helm result")
	})
}
