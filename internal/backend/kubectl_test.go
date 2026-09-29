package backend

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	backendmocks "github.com/castai/dbo-deployment-wizard/mocks/backend"
)

// nsCheckArgs is the namespace-existence check argv.
func nsCheckArgs(kubeContext, namespace string) []string {
	return []string{"--context", kubeContext, "get", "namespace", namespace, "--ignore-not-found", "-o", "name"}
}

// releaseQueryArgs is the release-revision Secret query argv.
func releaseQueryArgs(kubeContext, namespace, releaseName string) []string {
	return []string{
		"--context", kubeContext, "-n", namespace,
		"get", "secret", "-l", "owner=helm,name=" + releaseName, "-o", "json",
	}
}

const releaseSecretsJSON = `{"items":[{"metadata":{"name":"sh.helm.release.v1.castai-dbo.v1"}}]}`

func expectKubectlCall(sh *backendmocks.MockProcessRunner, args []string, out string, err error) {
	sh.EXPECT().Run(mock.Anything, "kubectl", []byte(nil), args).Return([]byte(out), err).Once()
}

func expectKubectlCallContains(sh *backendmocks.MockProcessRunner, args []string, out string, err error) {
	sh.EXPECT().Run(mock.Anything, "kubectl", []byte(nil), mock.MatchedBy(func(args []string) bool {
		cmdLine := strings.Join(args, " ")
		for _, arg := range args {
			if !strings.Contains(cmdLine, arg) {
				return false
			}
		}
		return true
	})).Return([]byte(out), err).Once()
}

func TestHelmReleaseExists(t *testing.T) {
	t.Run("caching works", func(t *testing.T) {
		sh := backendmocks.NewMockProcessRunner(t)
		expectKubectlCallContains(sh, []string{"get namespace castai-db-optimizer"}, "namespace/castai-db-optimizer\n", nil)
		expectKubectlCall(sh, releaseQueryArgs("kind-test", "castai-db-optimizer", "castai-dbo"), releaseSecretsJSON, nil)
		expectKubectlCall(sh, nsCheckArgs("kind-test", "other-ns"), "namespace/other-ns\n", nil)
		expectKubectlCall(sh, releaseQueryArgs("kind-test", "other-ns", "castai-dbo"), releaseSecretsJSON, nil)

		k := &RealKubectl{shell: sh}
		ctx := context.Background()

		// The same coordinates hit the cache: two lookups, one kubectl
		// round-trip pair (namespace, then the release Secrets).
		for range 2 {
			exists, err := k.HelmReleaseExists(ctx, "kind-test", "castai-db-optimizer", "castai-dbo")
			require.NoError(t, err)
			require.True(t, exists)
		}

		// New coordinates mean a fresh lookup.
		exists, err := k.HelmReleaseExists(ctx, "kind-test", "other-ns", "castai-dbo")
		require.NoError(t, err)
		require.True(t, exists)
	})

	t.Run("no namespace", func(t *testing.T) {
		// The namespace is missing: the secret query never runs.
		sh := backendmocks.NewMockProcessRunner(t)
		expectKubectlCall(sh, nsCheckArgs("kind-test", "castai-db-optimizer"), "", nil)

		k := &RealKubectl{shell: sh}
		exists, err := k.HelmReleaseExists(context.Background(), "kind-test", "castai-db-optimizer", "castai-dbo")
		require.NoError(t, err)
		require.False(t, exists)
	})

	t.Run("no release", func(t *testing.T) {
		sh := backendmocks.NewMockProcessRunner(t)
		expectKubectlCall(sh, nsCheckArgs("kind-test", "castai-db-optimizer"), "namespace/castai-db-optimizer\n", nil)
		expectKubectlCall(sh, releaseQueryArgs("kind-test", "castai-db-optimizer", "castai-dbo"), `{"items":[]}`, nil)

		k := &RealKubectl{shell: sh}
		exists, err := k.HelmReleaseExists(context.Background(), "kind-test", "castai-db-optimizer", "castai-dbo")
		require.NoError(t, err)
		require.False(t, exists)
	})

	t.Run("malformed json", func(t *testing.T) {
		sh := backendmocks.NewMockProcessRunner(t)
		expectKubectlCall(sh, nsCheckArgs("kind-test", "castai-db-optimizer"), "namespace/castai-db-optimizer\n", nil)
		expectKubectlCall(sh, releaseQueryArgs("kind-test", "castai-db-optimizer", "castai-dbo"), "not json", nil)

		k := &RealKubectl{shell: sh}
		exists, err := k.HelmReleaseExists(context.Background(), "kind-test", "castai-db-optimizer", "castai-dbo")
		require.ErrorContains(t, err, "parse kubectl result: invalid character")
		require.False(t, exists)
	})

	t.Run("cache after failure", func(t *testing.T) {
		const kubectlErr = `error: context "kind-test" does not exist`

		sh := backendmocks.NewMockProcessRunner(t)
		expectKubectlCall(sh, nsCheckArgs("kind-test", "castai-db-optimizer"), "", errors.New(kubectlErr))

		k := &RealKubectl{shell: sh}
		ctx := context.Background()

		for range 2 {
			exists, err := k.HelmReleaseExists(ctx, "kind-test", "castai-db-optimizer", "castai-dbo")
			require.ErrorContains(t, err, kubectlErr)
			require.False(t, exists)
		}
	})
}
