package backend

import (
	"context"
	"errors"
	"fmt"
	"io"
	"testing"
	"time"

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

	t.Run("retries a timed out lookup", func(t *testing.T) {
		sh := backendmocks.NewMockProcessRunner(t)
		for range 2 {
			expectHelmCall(sh, helmStatusArgs("kind-test", "castai-db-optimizer", "castai-dbo"), "", context.DeadlineExceeded)
		}

		k := &RealKubectl{shell: sh}

		for range 2 {
			exists, err := k.HelmReleaseExists(t.Context(), "kind-test", "castai-db-optimizer", "castai-dbo")
			require.ErrorIs(t, err, ErrClusterUnreachable)
			require.False(t, exists)
		}
	})

	t.Run("retries a lookup its context cut short", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		cancel()

		sh := backendmocks.NewMockProcessRunner(t)
		for range 2 {
			expectHelmCall(sh, helmStatusArgs("kind-test", "castai-db-optimizer", "castai-dbo"), "", ctx.Err())
		}

		k := &RealKubectl{shell: sh}

		for range 2 {
			exists, err := k.HelmReleaseExists(ctx, "kind-test", "castai-db-optimizer", "castai-dbo")
			require.ErrorIs(t, err, context.Canceled)
			require.False(t, exists)
		}
	})
}

// An unreachable cluster keeps kubectl and helm waiting far longer than
// the UI can; every captured cluster call must carry a deadline.
func TestClusterCalls_AreBounded(t *testing.T) {
	tests := map[string]func(ctx context.Context, k *RealKubectl){
		"list secrets": func(ctx context.Context, k *RealKubectl) {
			_, _ = k.ListSecrets(ctx, "kind-test", "castai-db-optimizer")
		},
		"helm release exists": func(ctx context.Context, k *RealKubectl) {
			_, _ = k.HelmReleaseExists(ctx, "kind-test", "castai-db-optimizer", "castai-dbo")
		},
		"ensure secret": func(ctx context.Context, k *RealKubectl) {
			_ = k.EnsureSecret(ctx, "kind-test", "castai-db-optimizer", "creds", nil, io.Discard, false)
		},
	}
	for name, call := range tests {
		t.Run(name, func(t *testing.T) {
			sh := backendmocks.NewMockProcessRunner(t)
			sh.EXPECT().Run(mock.Anything, mock.Anything, mock.Anything, mock.Anything).
				RunAndReturn(func(ctx context.Context, _ string, _ []byte, _ []string) ([]byte, error) {
					deadline, ok := ctx.Deadline()
					require.True(t, ok, "cluster call ran without a deadline")
					require.LessOrEqual(t, time.Until(deadline), clusterTimeout)

					return nil, context.DeadlineExceeded
				}).
				Once()

			call(t.Context(), &RealKubectl{shell: sh})
		})
	}
}

func TestListSecrets_TimeoutReportsClusterUnreachable(t *testing.T) {
	sh := backendmocks.NewMockProcessRunner(t)
	sh.EXPECT().Run(mock.Anything, "kubectl", mock.Anything, mock.Anything).
		Once().
		Return(nil, fmt.Errorf("kubectl get secret: %w", context.DeadlineExceeded))

	_, err := (&RealKubectl{shell: sh}).ListSecrets(t.Context(), "kind-test", "castai-db-optimizer")

	require.EqualError(t, err, "kubernetes cluster unreachable: no answer within 10s")
}
