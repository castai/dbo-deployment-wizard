package tui

import (
	"errors"
	"strings"
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
			row, ok := findRow(reviewItems(b, ""), "Chart version:")
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
		if _, ok := findRow(reviewItems(b, ""), "Pooling credentials:"); ok {
			t.Error("pooling credentials row present without pooling enabled")
		}
	})

	t.Run("secret source", func(t *testing.T) {
		b := &fakeBackend{
			components:   pooling,
			poolingCreds: api.Credentials{SecretName: "pooling-creds"},
		}
		row, ok := findRow(reviewItems(b, ""), "Pooling credentials:")
		if !ok {
			t.Fatal("pooling credentials row not found")
		}
		if row.value != "existing secret" || row.sub != "Selected secret: pooling-creds" || row.subWarn {
			t.Errorf("pooling row = %+v, want existing-secret summary without warning", row)
		}
	})

	t.Run("missing credentials", func(t *testing.T) {
		b := &fakeBackend{components: pooling}
		row, ok := findRow(reviewItems(b, ""), "Pooling credentials:")
		if !ok {
			t.Fatal("pooling credentials row not found")
		}
		if row.sub != "⚠ required" || !row.subWarn {
			t.Errorf("pooling row = %+v, want the yellow required marker", row)
		}
	})
}

func TestReviewItemsAgentCredsSub(t *testing.T) {
	t.Run("secret source", func(t *testing.T) {
		b := &fakeBackend{
			agentCreds: api.Credentials{SecretName: "agent-secret"},
			components: []string{api.ComponentDBAgent},
		}
		row, ok := findRow(reviewItems(b, ""), "Agent credentials:")
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
		row, ok := findRow(reviewItems(b, ""), "Agent credentials:")
		if !ok {
			t.Fatal("agent credentials row not found")
		}
		if row.value != "username/password" || row.sub != "Username: agent, password: ••••••••••••••••" {
			t.Errorf("agent row = %+v, want username/password summary", row)
		}
	})

	t.Run("unset with db-agent shows the required marker", func(t *testing.T) {
		b := &fakeBackend{components: []string{api.ComponentDBAgent}}
		row, ok := findRow(reviewItems(b, ""), "Agent credentials:")
		if !ok {
			t.Fatal("agent credentials row not found")
		}
		if row.value != unsetPlaceholder || row.sub != "⚠ required" || !row.subWarn {
			t.Errorf("agent row = %+v, want unset placeholder with the yellow required marker", row)
		}
	})

	t.Run("unset without db-agent is not required", func(t *testing.T) {
		b := &fakeBackend{components: []string{api.ComponentDBProxy}}
		row, ok := findRow(reviewItems(b, ""), "Agent credentials:")
		if !ok {
			t.Fatal("agent credentials row not found")
		}
		if row.sub != "" || row.subWarn {
			t.Errorf("agent row = %+v, want no required marker without db-agent", row)
		}
	})
}

func TestReviewItemsContinueRow(t *testing.T) {
	items := reviewItems(&fakeBackend{components: []string{api.ComponentDBAgent}}, "")
	last, ok := items[len(items)-1].(reviewItem)
	if !ok {
		t.Fatalf("last review item is %T, want reviewItem", items[len(items)-1])
	}
	if !last.action || last.label != "Continue" || last.err != "" {
		t.Errorf("last row = %+v, want the Continue action without a message", last)
	}
}

func TestReviewItemsContinueRowCarriesValidationMessage(t *testing.T) {
	const msg = "agent credentials are required\npooling credentials are required"
	last, ok := findRow(reviewItems(&fakeBackend{components: []string{api.ComponentDBAgent, api.ComponentDBProxy, api.ComponentPooling}}, msg), "Continue")
	if !ok {
		t.Fatal("continue row not found")
	}
	if last.err != msg {
		t.Errorf("continue row err = %q, want %q", last.err, msg)
	}
}

func TestReviewTitle(t *testing.T) {
	tests := map[string]struct {
		exists bool
		err    error
		want   string
	}{
		"new installation": {
			exists: false,
			want:   "Installing castai-dbo helm chart — review configuration (will create a new installation)",
		},
		"existing installation": {
			exists: true,
			want:   "Installing castai-dbo helm chart — review configuration (will update the existing installation)",
		},
		"lookup failure shows the error": {
			err:  errors.New("context unreachable"),
			want: "Installing castai-dbo helm chart — review configuration (could not determine: context unreachable)",
		},
		"multi-line errors clip to the first line": {
			err:  errors.New("first line\nsecond line"),
			want: "Installing castai-dbo helm chart — review configuration (could not determine: first line)",
		},
		"long errors truncate": {
			err:  errors.New(strings.Repeat("a", 60)),
			want: "Installing castai-dbo helm chart — review configuration (could not determine: " + strings.Repeat("a", 47) + "…)",
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			if got := reviewTitle(tt.exists, tt.err); got != tt.want {
				t.Errorf("reviewTitle() = %q, want %q", got, tt.want)
			}
		})
	}
}
