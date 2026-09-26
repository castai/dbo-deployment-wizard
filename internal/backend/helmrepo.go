// The Helm chart repository client: fetches and parses index.yaml,
// returning the versions of a given chart in newest-first order.
// Results are cached in memory for cacheTTL.
package backend

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/castai/dbo-deployment-wizard/internal/api"
)

const (
	DefaultIndexURL = "https://castai.github.io/helm-charts/index.yaml"

	cacheTTL = 10 * time.Minute

	defaultHTTPTimeout = 30 * time.Second
)

// HelmRepoClient fetches and caches a Helm chart repository index.
// slowNetwork delays every index fetch by slowNetworkDelay — the
// --slow-network UI testing aid.
type HelmRepoClient struct {
	httpClient  *http.Client
	indexURL    string
	slowNetwork bool

	mu        sync.Mutex
	index     map[string][]api.HelmChartVersion
	fetchedAt time.Time
}

// NewHelmRepoClient returns a client reading index.yaml from indexURL;
// nil httpClient and empty indexURL fall back to defaults. slowNetwork
// delays index fetches by slowNetworkDelay.
func NewHelmRepoClient(httpClient *http.Client, indexURL string, slowNetwork bool) *HelmRepoClient {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: defaultHTTPTimeout}
	}
	if indexURL == "" {
		indexURL = DefaultIndexURL
	}

	return &HelmRepoClient{httpClient: httpClient, indexURL: indexURL, slowNetwork: slowNetwork}
}

// ListVersions returns every published version of chartName, newest
// first — the helm CLI publishes entries newest-first per chart, and
// downstream code relies on that order.
func (c *HelmRepoClient) ListVersions(ctx context.Context, chartName string) ([]api.HelmChartVersion, error) {
	index, err := c.getIndex(ctx)
	if err != nil {
		return nil, err
	}
	versions, ok := index[chartName]
	if !ok {
		return nil, fmt.Errorf("chart %q not found in helm repository", chartName)
	}
	// Defensive copy so callers can't mutate the cache.
	out := make([]api.HelmChartVersion, len(versions))
	copy(out, versions)

	return out, nil
}

// LatestVersion returns the newest published version of chartName.
func (c *HelmRepoClient) LatestVersion(ctx context.Context, chartName string) (string, error) {
	versions, err := c.ListVersions(ctx, chartName)
	if err != nil {
		return "", err
	}
	if len(versions) == 0 {
		return "", fmt.Errorf("chart %q has no published versions", chartName)
	}

	return versions[0].Number, nil
}

func (c *HelmRepoClient) getIndex(ctx context.Context) (map[string][]api.HelmChartVersion, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.index != nil && time.Since(c.fetchedAt) < cacheTTL {
		return c.index, nil
	}

	index, err := c.fetchIndex(ctx)
	if err != nil {
		return nil, err
	}
	c.index = index
	c.fetchedAt = time.Now()

	return c.index, nil
}

// helmIndex is a minimal subset of the Helm repo index schema.
type helmIndex struct {
	Entries map[string][]helmEntry `yaml:"entries"`
}

type helmEntry struct {
	Version string    `yaml:"version"`
	Created time.Time `yaml:"created"`
}

func (c *HelmRepoClient) fetchIndex(ctx context.Context) (map[string][]api.HelmChartVersion, error) {
	if c.slowNetwork {
		if err := sleepCtx(ctx, slowNetworkDelay); err != nil {
			return nil, err
		}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.indexURL, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching helm repo index: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("helm repo index returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading helm repo index: %w", err)
	}

	var parsed helmIndex
	if err := yaml.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("parsing helm repo index: %w", err)
	}

	versions := make(map[string][]api.HelmChartVersion, len(parsed.Entries))
	for name, entries := range parsed.Entries {
		out := make([]api.HelmChartVersion, 0, len(entries))
		for _, e := range entries {
			if e.Version == "" {
				continue
			}
			out = append(out, api.HelmChartVersion{Number: e.Version, Created: e.Created})
		}
		versions[name] = out
	}

	return versions, nil
}
