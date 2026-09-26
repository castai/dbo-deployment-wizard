package backend

import (
	"context"
	_ "embed"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/castai/dbo-deployment-wizard/internal/api"
)

// testHelmIndex is the Helm repository index the test servers serve,
// embedded at compile time.
//
//go:embed testdata/index.yaml
var testHelmIndex []byte

// newTestClient spins up an httptest server serving body and returns a
// HelmRepoClient pointed at it; the server's teardown is queued on
// t.Cleanup so the test never leaks a listener.
func newTestClient(t *testing.T, body []byte) *HelmRepoClient {
	t.Helper()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/yaml")
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)

	return NewHelmRepoClient(srv.Client(), srv.URL)
}

func TestListVersions(t *testing.T) {
	r := require.New(t)

	c := newTestClient(t, testHelmIndex)
	got, err := c.ListVersions(context.Background(), "castai-dbo")
	r.NoError(err)
	r.Equal([]api.HelmChartVersion{
		{Number: "1.4.2", Created: time.Date(2026, 1, 15, 10, 0, 0, 0, time.UTC)},
		{Number: "1.4.1", Created: time.Date(2025, 12, 20, 10, 0, 0, 0, time.UTC)},
		{Number: "1.4.0", Created: time.Date(2025, 12, 1, 10, 0, 0, 0, time.UTC)},
		{Number: "1.3.7", Created: time.Date(2025, 11, 10, 10, 0, 0, 0, time.UTC)},
	}, got)
}

func TestLatestVersion(t *testing.T) {
	r := require.New(t)

	c := newTestClient(t, testHelmIndex)
	got, err := c.LatestVersion(context.Background(), "castai-dbo")
	r.NoError(err)
	r.Equal("1.4.2", got)
}

func TestListVersions_ChartNotFound(t *testing.T) {
	r := require.New(t)

	c := newTestClient(t, testHelmIndex)
	_, err := c.ListVersions(context.Background(), "does-not-exist")
	r.Error(err)
}

func TestListVersions_StripsEmptyVersions(t *testing.T) {
	r := require.New(t)

	c := newTestClient(t, []byte(`entries:
  castai-dbo:
    - version: 2.0.0
    - version: ""
    - version: 1.9.0
`))
	got, err := c.ListVersions(context.Background(), "castai-dbo")
	r.NoError(err)
	r.Equal([]api.HelmChartVersion{{Number: "2.0.0"}, {Number: "1.9.0"}}, got)
}
