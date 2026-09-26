package tui

import (
	"testing"

	"charm.land/bubbles/v2/list"

	"github.com/castai/dbo-deployment-wizard/internal/api"
)

// fakeBackend is an api.Backend test double that answers only the
// getters reviewItems reads; any other call panics through the
// embedded nil interface.
type fakeBackend struct {
	api.Backend

	kubeContext  string
	namespace    string
	chartVersion string
	agentCreds   api.Credentials
	poolingCreds api.Credentials
	components   []string
	latest       string
}

func (f *fakeBackend) KubeContext() string  { return f.kubeContext }
func (f *fakeBackend) Namespace() string    { return f.namespace }
func (f *fakeBackend) ChartVersion() string { return f.chartVersion }

func (f *fakeBackend) AgentCredentials() api.Credentials   { return f.agentCreds }
func (f *fakeBackend) PoolingCredentials() api.Credentials { return f.poolingCreds }

func (f *fakeBackend) Components() []string       { return f.components }
func (f *fakeBackend) LatestChartVersion() string { return f.latest }

func findRow(items []list.Item, label string) (reviewItem, bool) {
	for _, it := range items {
		if ri, ok := it.(reviewItem); ok && ri.label == label {
			return ri, true
		}
	}

	return reviewItem{}, false
}

func TestReviewItemsChartVersionLatestSuffix(t *testing.T) {
	tests := map[string]struct {
		version string
		latest  string
		want    string
	}{
		"matching latest": {version: "1.4.2", latest: "1.4.2", want: "1.4.2 (latest)"},
		"older version":   {version: "1.4.1", latest: "1.4.2", want: "1.4.1"},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			b := &fakeBackend{chartVersion: tt.version, latest: tt.latest}
			row, ok := findRow(reviewItems(b), "Chart version:")
			if !ok {
				t.Fatal("chart version row not found")
			}
			if row.value != tt.want {
				t.Errorf("chart version value = %q, want %q", row.value, tt.want)
			}
		})
	}
}

func TestReviewItemsPoolingRow(t *testing.T) {
	pooling := []string{api.ComponentDBAgent, api.ComponentDBProxy, api.ComponentPooling}

	t.Run("hidden without pooling", func(t *testing.T) {
		b := &fakeBackend{components: []string{api.ComponentDBAgent}}
		if _, ok := findRow(reviewItems(b), "Pooling credentials:"); ok {
			t.Error("pooling credentials row present without pooling enabled")
		}
	})

	t.Run("secret source", func(t *testing.T) {
		b := &fakeBackend{
			components:   pooling,
			poolingCreds: api.Credentials{SecretName: "pooling-creds"},
		}
		row, ok := findRow(reviewItems(b), "Pooling credentials:")
		if !ok {
			t.Fatal("pooling credentials row not found")
		}
		if row.value != "existing secret" || row.sub != "Selected secret: pooling-creds" || row.subError {
			t.Errorf("pooling row = %+v, want existing-secret summary without error", row)
		}
	})

	t.Run("missing credentials", func(t *testing.T) {
		b := &fakeBackend{components: pooling}
		row, ok := findRow(reviewItems(b), "Pooling credentials:")
		if !ok {
			t.Fatal("pooling credentials row not found")
		}
		if !row.subError {
			t.Errorf("pooling row = %+v, want subError with missing credentials", row)
		}
	})
}

func TestReviewItemsAgentCredsSub(t *testing.T) {
	t.Run("secret source", func(t *testing.T) {
		b := &fakeBackend{
			agentCreds: api.Credentials{SecretName: "agent-secret"},
			components: []string{api.ComponentDBAgent},
		}
		row, ok := findRow(reviewItems(b), "Agent credentials:")
		if !ok {
			t.Fatal("agent credentials row not found")
		}
		if row.value != "existing secret" || row.sub != "Selected secret: agent-secret" {
			t.Errorf("agent row = %+v, want existing-secret summary", row)
		}
	})

	t.Run("username and password", func(t *testing.T) {
		b := &fakeBackend{
			agentCreds: api.Credentials{Username: "agent", Password: "secret"},
			components: []string{api.ComponentDBAgent},
		}
		row, ok := findRow(reviewItems(b), "Agent credentials:")
		if !ok {
			t.Fatal("agent credentials row not found")
		}
		if row.value != "username/password" || row.sub != "Username: agent, password: ••••••••••••••••" {
			t.Errorf("agent row = %+v, want username/password summary", row)
		}
	})

	t.Run("unset shows no sub", func(t *testing.T) {
		b := &fakeBackend{components: []string{api.ComponentDBAgent}}
		row, ok := findRow(reviewItems(b), "Agent credentials:")
		if !ok {
			t.Fatal("agent credentials row not found")
		}
		if row.value != unsetPlaceholder || row.sub != "" {
			t.Errorf("agent row = %+v, want unset placeholder without sub", row)
		}
	})
}

func TestReviewItemsContinueRow(t *testing.T) {
	items := reviewItems(&fakeBackend{components: []string{api.ComponentDBAgent}})
	last, ok := items[len(items)-1].(reviewItem)
	if !ok {
		t.Fatalf("last review item is %T, want reviewItem", items[len(items)-1])
	}
	if !last.action || last.label != "Continue" {
		t.Errorf("last row = %+v, want the Continue action", last)
	}
}
