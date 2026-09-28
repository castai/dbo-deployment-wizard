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

// stubKubectlDir puts a kubectl stub on PATH. Every invocation logs
// its arguments to the KUBECTL_LOG file. Namespace queries print
// KUBECTL_NS (empty: no such namespace); every other query prints
// KUBECTL_OUT. KUBECTL_FAIL fails each call with KUBECTL_ERR on
// stderr instead.
func stubKubectlDir(t *testing.T, logPath, out, ns string, fail bool) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the shell stub only runs on unix")
	}

	dir := t.TempDir()
	script := "#!/bin/sh\n" +
		"echo \"$@\" >> \"$KUBECTL_LOG\"\n" +
		"if [ -n \"$KUBECTL_FAIL\" ]; then\n" +
		"  echo \"$KUBECTL_ERR\" >&2\n" +
		"  exit 1\n" +
		"fi\n" +
		"case \"$*\" in\n" +
		"*\"get namespace\"*)\n" +
		"  if [ -n \"$KUBECTL_NS\" ]; then printf '%s\\n' \"$KUBECTL_NS\"; fi\n" +
		"  ;;\n" +
		"*)\n" +
		"  printf '%s\\n' \"$KUBECTL_OUT\"\n" +
		"  ;;\n" +
		"esac\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "kubectl"), []byte(script), 0o755))

	t.Setenv("PATH", dir)
	t.Setenv("KUBECTL_LOG", logPath)
	t.Setenv("KUBECTL_OUT", out)
	t.Setenv("KUBECTL_NS", ns)
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
	stubKubectlDir(t, logPath, "sh.helm.release.v1.castai-dbo.v1", "namespace/castai-db-optimizer", false)

	k := &RealKubectl{}
	ctx := context.Background()

	// The same coordinates hit the cache: two lookups, one kubectl
	// round-trip pair (namespace, then the release Secrets).
	for range 2 {
		exists, err := k.HelmReleaseExists(ctx, "kind-test", "castai-db-optimizer", "castai-dbo")
		require.NoError(t, err)
		require.True(t, exists)
	}
	require.Len(t, invocationLog(t, logPath), 2)

	// New coordinates mean a fresh lookup.
	exists, err := k.HelmReleaseExists(ctx, "kind-test", "other-ns", "castai-dbo")
	require.NoError(t, err)
	require.True(t, exists)

	lines := invocationLog(t, logPath)
	require.Len(t, lines, 4)
	require.Contains(t, lines[0], "get namespace castai-db-optimizer")
	require.Contains(t, lines[0], "--ignore-not-found")
	require.Contains(t, lines[1], "owner=helm,name=castai-dbo")
	require.Contains(t, lines[2], "get namespace other-ns")
	require.Contains(t, lines[3], "-n other-ns")
	require.Contains(t, lines[3], "owner=helm,name=castai-dbo")
}

func TestRealKubectlHelmReleaseExistsWithoutNamespace(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "calls.log")
	// The namespace is missing, yet the secret response is configured —
	// proving the lookup stops at the namespace check.
	stubKubectlDir(t, logPath, "sh.helm.release.v1.castai-dbo.v1", "", false)

	k := &RealKubectl{}
	exists, err := k.HelmReleaseExists(context.Background(), "kind-test", "castai-db-optimizer", "castai-dbo")
	require.NoError(t, err)
	require.False(t, exists)

	lines := invocationLog(t, logPath)
	require.Len(t, lines, 1)
	require.Contains(t, lines[0], "get namespace castai-db-optimizer")
}

func TestRealKubectlHelmReleaseExistsWithoutRelease(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "calls.log")
	stubKubectlDir(t, logPath, "", "namespace/castai-db-optimizer", false)

	k := &RealKubectl{}
	exists, err := k.HelmReleaseExists(context.Background(), "kind-test", "castai-db-optimizer", "castai-dbo")
	require.NoError(t, err)
	require.False(t, exists)
	require.Len(t, invocationLog(t, logPath), 2)
}

func TestRealKubectlHelmReleaseExistsCachesFailures(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "calls.log")
	stubKubectlDir(t, logPath, "", "", true)

	// A failed run's error is kubectl's stderr — the actual reason.
	const kubectlErr = `error: context "kind-test" does not exist`
	t.Setenv("KUBECTL_ERR", kubectlErr)

	k := &RealKubectl{}
	ctx := context.Background()

	for range 2 {
		exists, err := k.HelmReleaseExists(ctx, "kind-test", "castai-db-optimizer", "castai-dbo")
		require.ErrorContains(t, err, kubectlErr)
		require.False(t, exists)
	}
	require.Len(t, invocationLog(t, logPath), 1)
}
