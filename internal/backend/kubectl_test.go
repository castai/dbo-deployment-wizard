package backend

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	backendmocks "github.com/castai/dbo-deployment-wizard/mocks/backend"
)

// helmStatusArgs is the release-status argv.
func helmStatusArgs(kubeContext, namespace, releaseName string) []string {
	return []string{"status", releaseName, "-n", namespace, "--kube-context", kubeContext, "-o", "json"}
}

// helmStatusJSON is `helm status -o json`'s answer for the release.
const helmStatusJSON = `{"name":"castai-dbo","info":{"status":"deployed"}}`

// expectHelmCall stubs one helm run: it fires once with the argv and
// answers with stdout or the error.
func expectHelmCall(sh *backendmocks.MockProcessRunner, args []string, out string, err error) {
	// stdin arrives as a typed []byte(nil); an untyped nil would not
	// match in testify.
	sh.EXPECT().Run(mock.Anything, "helm", []byte(nil), args).
		Once().
		Return([]byte(out), err)
}

func TestHelmReleaseExists(t *testing.T) {
	t.Run("caches by coordinates", func(t *testing.T) {
		sh := backendmocks.NewMockProcessRunner(t)
		expectHelmCall(sh, helmStatusArgs("kind-test", "castai-db-optimizer", "castai-dbo"), helmStatusJSON, nil)
		expectHelmCall(sh, helmStatusArgs("kind-test", "other-ns", "castai-dbo"), helmStatusJSON, nil)

		k := &RealKubectl{shell: sh}
		ctx := t.Context()

		// The same coordinates hit the cache: two lookups, one helm run.
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

	t.Run("not found is a new installation", func(t *testing.T) {
		// helm answers its not-found line for a missing release and a
		// missing namespace alike.
		sh := backendmocks.NewMockProcessRunner(t)
		expectHelmCall(sh, helmStatusArgs("kind-test", "castai-db-optimizer", "castai-dbo"), "", ErrHelmReleaseNotFound)

		k := &RealKubectl{shell: sh}
		ctx := t.Context()

		for range 2 {
			exists, err := k.HelmReleaseExists(ctx, "kind-test", "castai-db-optimizer", "castai-dbo")
			require.NoError(t, err)
			require.False(t, exists)
		}
	})

	t.Run("malformed json", func(t *testing.T) {
		sh := backendmocks.NewMockProcessRunner(t)
		expectHelmCall(sh, helmStatusArgs("kind-test", "castai-db-optimizer", "castai-dbo"), "not json", nil)

		k := &RealKubectl{shell: sh}
		exists, err := k.HelmReleaseExists(t.Context(), "kind-test", "castai-db-optimizer", "castai-dbo")
		require.ErrorContains(t, err, "parse helm result")
		require.False(t, exists)
	})

	t.Run("answers a different release", func(t *testing.T) {
		sh := backendmocks.NewMockProcessRunner(t)
		expectHelmCall(sh, helmStatusArgs("kind-test", "castai-db-optimizer", "castai-dbo"),
			`{"name":"castai-dbo-2"}`, nil)

		k := &RealKubectl{shell: sh}
		exists, err := k.HelmReleaseExists(t.Context(), "kind-test", "castai-db-optimizer", "castai-dbo")
		require.ErrorContains(t, err, `answered release "castai-dbo-2"`)
		require.False(t, exists)
	})

	t.Run("caches failures", func(t *testing.T) {
		const helmErr = `Error: Kubernetes cluster unreachable`

		sh := backendmocks.NewMockProcessRunner(t)
		expectHelmCall(sh, helmStatusArgs("kind-test", "castai-db-optimizer", "castai-dbo"), "", errors.New(helmErr))

		k := &RealKubectl{shell: sh}
		ctx := t.Context()

		for range 2 {
			exists, err := k.HelmReleaseExists(ctx, "kind-test", "castai-db-optimizer", "castai-dbo")
			require.ErrorContains(t, err, helmErr)
			require.False(t, exists)
		}
	})
}
