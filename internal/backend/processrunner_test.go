package backend

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	backendmocks "github.com/castai/dbo-deployment-wizard/mocks/backend"
)

// A killed program's orphaned child keeps the output pipes open; Run
// must still return once the deadline passes.
func TestRealProcessRunnerRun_ReturnsOnDeadline(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, err := (&RealProcessRunner{}).Run(ctx, "sh", nil, []string{"-c", "sleep 20 & wait"})

	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.Less(t, time.Since(start), 10*time.Second)
}

func TestRunHelmYAML(t *testing.T) {
	type sampleResult struct {
		Name string `yaml:"name"`
	}

	// shellReturning stubs one helm run answering with out.
	shellReturning := func(t testing.TB, out []byte) *backendmocks.MockProcessRunner {
		sh := backendmocks.NewMockProcessRunner(t)
		sh.EXPECT().Run(mock.Anything, "helm", mock.Anything, []string{"get", "manifest"}).
			Return(out, nil)

		return sh
	}

	t.Run("decodes each document", func(t *testing.T) {
		sh := shellReturning(t, []byte("---\nname: a\n---\nname: b\n"))

		got, err := runHelmYAML[[]sampleResult](t.Context(), sh, []string{"get", "manifest"})
		require.NoError(t, err)
		require.Equal(t, []sampleResult{{Name: "a"}, {Name: "b"}}, got)
	})

	t.Run("empty output decodes to none", func(t *testing.T) {
		sh := shellReturning(t, nil)

		got, err := runHelmYAML[[]sampleResult](t.Context(), sh, []string{"get", "manifest"})
		require.NoError(t, err)
		require.Empty(t, got)
	})

	t.Run("malformed yaml errors", func(t *testing.T) {
		sh := shellReturning(t, []byte("not yaml: ["))

		_, err := runHelmYAML[[]sampleResult](t.Context(), sh, []string{"get", "manifest"})
		require.ErrorContains(t, err, "parse helm result")
	})
}
