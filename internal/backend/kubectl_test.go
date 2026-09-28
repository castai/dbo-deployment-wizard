package backend

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// stubKubectlDir puts a kubectl stub on PATH: every invocation logs
// its arguments to the KUBECTL_LOG file, then prints KUBECTL_OUT — or,
// when KUBECTL_FAIL is set, prints KUBECTL_ERR to stderr and exits 1.
func stubKubectlDir(t *testing.T, logPath, out string, fail bool) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the shell stub only runs on unix")
	}

	dir := t.TempDir()
	script := "#!/bin/sh\necho \"$@\" >> \"$KUBECTL_LOG\"\nif [ -n \"$KUBECTL_FAIL\" ]; then\n  echo \"$KUBECTL_ERR\" >&2\n  exit 1\nfi\nprintf '%s\\n' \"$KUBECTL_OUT\"\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "kubectl"), []byte(script), 0o755))

	t.Setenv("PATH", dir)
	t.Setenv("KUBECTL_LOG", logPath)
	t.Setenv("KUBECTL_OUT", out)
	if fail {
		t.Setenv("KUBECTL_FAIL", "1")
	}
}

// invocationLog returns the stub's logged invocations, one per line.
func invocationLog(t *testing.T, path string) []string {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)

	var lines []string
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if line != "" {
			lines = append(lines, line)
		}
	}

	return lines
}

func TestRealKubectlHelmReleaseExistsCachesByCoordinates(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "calls.log")
	stubKubectlDir(t, logPath, "sh.helm.release.v1.castai-dbo.v1", false)

	k := &RealKubectl{}
	ctx := context.Background()

	// The same coordinates hit the cache: two lookups, one kubectl call.
	for range 2 {
		exists, err := k.HelmReleaseExists(ctx, "kind-test", "castai-db-optimizer", "castai-dbo")
		require.NoError(t, err)
		require.True(t, exists)
	}
	require.Len(t, invocationLog(t, logPath), 1)

	// New coordinates mean a fresh lookup.
	exists, err := k.HelmReleaseExists(ctx, "kind-test", "other-ns", "castai-dbo")
	require.NoError(t, err)
	require.True(t, exists)

	lines := invocationLog(t, logPath)
	require.Len(t, lines, 2)
	require.Contains(t, lines[0], "--context kind-test")
	require.Contains(t, lines[0], "-n castai-db-optimizer")
	require.Contains(t, lines[0], "owner=helm,name=castai-dbo")
	require.Contains(t, lines[1], "-n other-ns")
}

func TestRealKubectlHelmReleaseExistsCachesFailures(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "calls.log")
	stubKubectlDir(t, logPath, "", true)

	// A failed run's error carries kubectl's stderr — the actual
	// reason — ahead of the wrapped exit status.
	const kubectlErr = `error: context "kind-test" does not exist`
	t.Setenv("KUBECTL_ERR", kubectlErr)

	k := &RealKubectl{}
	ctx := context.Background()

	for range 2 {
		exists, err := k.HelmReleaseExists(ctx, "kind-test", "castai-db-optimizer", "castai-dbo")
		require.ErrorContains(t, err, kubectlErr)
		require.ErrorContains(t, err, "exit status 1")
		require.False(t, exists)
	}
	require.Len(t, invocationLog(t, logPath), 1)
}
