package backend

import (
	"context"
	"errors"
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

// probeArgs is the reachability probe's argv.
func probeArgs(kubeContext string) []string {
	return []string{"--context", kubeContext, "get", "--raw", "/version"}
}

// reachableKubectl is a RealKubectl whose probe already found
// kubeContext reachable, so a test stubs only the call under test.
func reachableKubectl(sh ProcessRunner, kubeContext string) *RealKubectl {
	return &RealKubectl{shell: sh, probe: contextProbe{probed: true, kubeContext: kubeContext}}
}

func TestHelmReleaseExists(t *testing.T) {
	t.Run("caches by coordinates", func(t *testing.T) {
		sh := backendmocks.NewMockProcessRunner(t)
		expectHelmCall(sh, helmStatusArgs("kind-test", "castai-db-optimizer", "castai-dbo"), helmStatusJSON, nil)
		expectHelmCall(sh, helmStatusArgs("kind-test", "other-ns", "castai-dbo"), helmStatusJSON, nil)

		k := reachableKubectl(sh, "kind-test")
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

		k := reachableKubectl(sh, "kind-test")
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

		k := reachableKubectl(sh, "kind-test")
		exists, err := k.HelmReleaseExists(t.Context(), "kind-test", "castai-db-optimizer", "castai-dbo")
		require.ErrorContains(t, err, "parse helm result")
		require.False(t, exists)
	})

	t.Run("answers a different release", func(t *testing.T) {
		sh := backendmocks.NewMockProcessRunner(t)
		expectHelmCall(sh, helmStatusArgs("kind-test", "castai-db-optimizer", "castai-dbo"),
			`{"name":"castai-dbo-2"}`, nil)

		k := reachableKubectl(sh, "kind-test")
		exists, err := k.HelmReleaseExists(t.Context(), "kind-test", "castai-db-optimizer", "castai-dbo")
		require.ErrorContains(t, err, `answered release "castai-dbo-2"`)
		require.False(t, exists)
	})

	t.Run("caches failures", func(t *testing.T) {
		const helmErr = `Error: Kubernetes cluster unreachable`

		sh := backendmocks.NewMockProcessRunner(t)
		expectHelmCall(sh, helmStatusArgs("kind-test", "castai-db-optimizer", "castai-dbo"), "", errors.New(helmErr))

		k := reachableKubectl(sh, "kind-test")
		ctx := t.Context()

		for range 2 {
			exists, err := k.HelmReleaseExists(ctx, "kind-test", "castai-db-optimizer", "castai-dbo")
			require.ErrorContains(t, err, helmErr)
			require.False(t, exists)
		}
	})

	t.Run("retries a timed out lookup", func(t *testing.T) {
		r := require.New(t)

		sh := backendmocks.NewMockProcessRunner(t)
		for range 2 {
			expectHelmCall(sh, helmStatusArgs("kind-test", "castai-db-optimizer", "castai-dbo"), "", context.DeadlineExceeded)
		}

		k := reachableKubectl(sh, "kind-test")

		for range 2 {
			exists, err := k.HelmReleaseExists(t.Context(), "kind-test", "castai-db-optimizer", "castai-dbo")
			r.ErrorIs(err, ErrClusterUnreachable)
			r.False(exists)
		}
	})

	t.Run("retries a lookup its context cut short", func(t *testing.T) {
		r := require.New(t)

		ctx, cancel := context.WithCancel(t.Context())
		cancel()

		sh := backendmocks.NewMockProcessRunner(t)
		for range 2 {
			expectHelmCall(sh, helmStatusArgs("kind-test", "castai-db-optimizer", "castai-dbo"), "", ctx.Err())
		}

		k := reachableKubectl(sh, "kind-test")

		for range 2 {
			exists, err := k.HelmReleaseExists(ctx, "kind-test", "castai-db-optimizer", "castai-dbo")
			r.ErrorIs(err, context.Canceled)
			r.False(exists)
		}
	})
}

// clusterCalls is every captured call that needs the cluster.
var clusterCalls = map[string]func(ctx context.Context, k *RealKubectl) error{
	"list secrets": func(ctx context.Context, k *RealKubectl) error {
		_, err := k.ListSecrets(ctx, "kind-test", "castai-db-optimizer")

		return err
	},
	"helm release exists": func(ctx context.Context, k *RealKubectl) error {
		_, err := k.HelmReleaseExists(ctx, "kind-test", "castai-db-optimizer", "castai-dbo")

		return err
	},
	"ensure secret": func(ctx context.Context, k *RealKubectl) error {
		return k.EnsureSecret(ctx, "kind-test", "castai-db-optimizer", "creds", nil, io.Discard, false)
	},
}

// An unreachable cluster keeps kubectl and helm waiting far longer than
// the UI can; the probe and every call after it must carry a deadline.
func TestClusterCalls_AreBounded(t *testing.T) {
	requireBounded := func(t testing.TB, ctx context.Context, limit time.Duration) {
		t.Helper()

		r := require.New(t)

		deadline, ok := ctx.Deadline()
		r.True(ok, "cluster call ran without a deadline")
		r.LessOrEqual(time.Until(deadline), limit)
	}

	for name, call := range clusterCalls {
		t.Run(name, func(t *testing.T) {
			r := require.New(t)

			sh := backendmocks.NewMockProcessRunner(t)
			sh.EXPECT().Run(mock.Anything, "kubectl", []byte(nil), probeArgs("kind-test")).
				RunAndReturn(func(ctx context.Context, _ string, _ []byte, _ []string) ([]byte, error) {
					requireBounded(t, ctx, connectTimeout)

					return nil, nil
				}).
				Once()
			sh.EXPECT().Run(mock.Anything, mock.Anything, mock.Anything, mock.Anything).
				RunAndReturn(func(ctx context.Context, _ string, _ []byte, _ []string) ([]byte, error) {
					requireBounded(t, ctx, replyTimeout)

					return nil, context.DeadlineExceeded
				}).
				Once()

			err := call(t.Context(), &RealKubectl{shell: sh})
			r.ErrorIs(err, ErrClusterUnreachable)
			r.ErrorContains(err, "no answer within 30s")
		})
	}
}

func TestReachable(t *testing.T) {
	expectProbe := func(sh *backendmocks.MockProcessRunner, kubeContext string, err error) {
		sh.EXPECT().Run(mock.Anything, "kubectl", []byte(nil), probeArgs(kubeContext)).Once().Return(nil, err)
	}

	t.Run("an unreachable context is probed once and never called", func(t *testing.T) {
		r := require.New(t)

		sh := backendmocks.NewMockProcessRunner(t)
		expectProbe(sh, "kind-test", context.DeadlineExceeded)

		k := &RealKubectl{shell: sh}
		for _, call := range clusterCalls {
			r.ErrorContains(call(t.Context(), k), "kubernetes cluster unreachable: no answer within 5s")
		}
	})

	t.Run("a failed probe carries kubectl's reason", func(t *testing.T) {
		r := require.New(t)

		sh := backendmocks.NewMockProcessRunner(t)
		expectProbe(sh, "kind-test", errors.New("Unable to connect to the server: no such host"))

		_, err := (&RealKubectl{shell: sh}).ListSecrets(t.Context(), "kind-test", "castai-db-optimizer")
		r.ErrorIs(err, ErrClusterUnreachable)
		r.EqualError(err, "kubernetes cluster unreachable: Unable to connect to the server: no such host")
	})

	t.Run("only the last context's verdict is kept", func(t *testing.T) {
		r := require.New(t)

		sh := backendmocks.NewMockProcessRunner(t)
		// Switching away and back probes kind-test again.
		expectProbe(sh, "kind-test", context.DeadlineExceeded)
		expectProbe(sh, "kind-other", context.DeadlineExceeded)
		expectProbe(sh, "kind-test", context.DeadlineExceeded)

		k := &RealKubectl{shell: sh}
		for _, kubeContext := range []string{"kind-test", "kind-test", "kind-other", "kind-test"} {
			_, err := k.ListSecrets(t.Context(), kubeContext, "castai-db-optimizer")
			r.ErrorIs(err, ErrClusterUnreachable)
		}
	})

	t.Run("a probe the caller cut short is no verdict", func(t *testing.T) {
		r := require.New(t)

		ctx, cancel := context.WithCancel(t.Context())
		cancel()

		sh := backendmocks.NewMockProcessRunner(t)
		expectProbe(sh, "kind-test", ctx.Err())
		expectProbe(sh, "kind-test", ctx.Err())

		k := &RealKubectl{shell: sh}
		for range 2 {
			_, err := k.ListSecrets(ctx, "kind-test", "castai-db-optimizer")
			r.ErrorIs(err, context.Canceled)
		}
	})
}
