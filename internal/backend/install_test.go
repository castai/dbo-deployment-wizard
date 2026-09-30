package backend

import (
	"context"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/castai/dbo-deployment-wizard/internal/api"
	backendmocks "github.com/castai/dbo-deployment-wizard/mocks/backend"
)

func TestExecutorDryRun_DoesNotInvokeHelm(t *testing.T) {
	r := require.New(t)

	cfg := Config{
		ReleaseName:  "castai-dbo",
		ChartName:    "castai-dbo",
		Namespace:    "castai-dbo",
		KubeContext:  "kind-test",
		ChartVersion: "1.4.2",
		Components:   []string{api.ComponentDBProxy, api.ComponentPooling},
		APIURL:       "https://api.cast.ai",
		APISecret:    "sec",
		DryRun:       true,
	}

	// The strict mock fails on any shell run — a dry run must not execute.
	sh := backendmocks.NewMockProcessRunner(t)

	values := newHelmValues(cfg)
	r.NoError(Install(context.Background(), values, cfg, sh))
}

func TestExecutorLiveRun_InvokesHelm(t *testing.T) {
	r := require.New(t)

	cfg := Config{
		ReleaseName:  "castai-dbo",
		ChartName:    "castai-dbo",
		Namespace:    "castai-dbo",
		KubeContext:  "kind-test",
		ChartVersion: "1.4.2",
		Components:   []string{api.ComponentDBAgent, api.ComponentDBProxy, api.ComponentPooling},
		AgentCreds:   api.Credentials{Username: "agent", Password: "agentpass"},
		PoolingCreds: api.Credentials{Username: "pooler", Password: "poolpass"},
		APIURL:       "https://api.cast.ai",
		APISecret:    "sec",
	}

	k := backendmocks.NewMockKubectl(t)
	k.EXPECT().EnsureSecret(mock.Anything, "kind-test", "castai-dbo", "castai-dbo-agent-credentials",
		map[string]string{"DATABASE_USERNAME": "agent", "DATABASE_PASSWORD": "agentpass"}).Return(nil)
	k.EXPECT().EnsureSecret(mock.Anything, "kind-test", "castai-dbo", "castai-dbo-pooling-credentials",
		map[string]string{"DATABASE_USERNAME": "pooler", "DATABASE_PASSWORD": "poolpass"}).Return(nil)
	k.EXPECT().EnsureSecret(mock.Anything, "kind-test", "castai-dbo", "castai-dbo-api-key",
		map[string]string{"API_KEY": "sec"}).Return(nil)
	k.EXPECT().RolloutStatus(mock.Anything, "kind-test", "castai-dbo", "deployment.apps/db-agent", rolloutTimeout,
		mock.Anything, mock.Anything).Return(nil)

	var valuesBytes []byte
	sh := backendmocks.NewMockProcessRunner(t)
	sh.EXPECT().Run(mock.Anything, "helm", mock.Anything,
		mock.MatchedBy(func(args []string) bool { return args[0] == "upgrade" })).
		Run(func(_ context.Context, _ string, stdin []byte, _ []string) {
			valuesBytes = stdin
		}).
		Return([]byte("Release \"castai-dbo\" has been upgraded.\n"), nil)
	sh.EXPECT().Run(mock.Anything, "helm", mock.Anything,
		mock.MatchedBy(func(args []string) bool { return args[0] == "get" })).
		Return([]byte("---\napiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: db-agent\n"), nil)

	w := &Wizard{ctx: context.Background(), k: k, shell: sh, cfg: cfg}
	r.NoError(w.Install())

	// Username/password pairs resolve into Secret refs, and every
	// enabled component points at the recreated API key Secret.
	var got helmValues
	r.NoError(yaml.Unmarshal(valuesBytes, &got))
	r.Equal(helmValues{
		DBAgent: dbAgentValues{
			Enabled:         true,
			APIURL:          "https://api.cast.ai",
			APIKeySecretRef: "castai-dbo-api-key",
			Database:        dbAgentDatabase{CredentialsSecretRef: "castai-dbo-agent-credentials"},
		},
		DBProxy: dbProxyValues{
			Enabled:         true,
			APIKeySecretRef: "castai-dbo-api-key",
			Pooling: poolingValues{
				Enabled:  true,
				ProxySQL: proxySQLValues{UserSecretRef: "castai-dbo-pooling-credentials"},
			},
		},
	}, got)
}

// TestNewHelmValues_Cartesian asserts the exact values the renderer
// emits across the (component-set × credentials) combinations the
// wizard supports.
func TestNewHelmValues_Cartesian(t *testing.T) {
	type tc struct {
		name   string
		config Config
		want   helmValues
	}

	allComponents := []string{
		api.ComponentDBProxy,
		api.ComponentDBAgent,
		api.ComponentPooling,
	}

	cases := []tc{
		{
			name: "agent secret ref",
			config: Config{
				AgentCreds: api.Credentials{SecretName: "my-agent-secret"},
			},
			want: helmValues{
				DBAgent: dbAgentValues{Database: dbAgentDatabase{CredentialsSecretRef: "my-agent-secret"}},
			},
		},
		{
			name: "all components, pooling secret ref",
			config: Config{
				Components:   allComponents,
				PoolingCreds: api.Credentials{SecretName: "pooling-secret"},
			},
			want: helmValues{
				DBAgent: dbAgentValues{Enabled: true},
				DBProxy: dbProxyValues{
					Enabled: true,
					Pooling: poolingValues{
						Enabled:  true,
						ProxySQL: proxySQLValues{UserSecretRef: "pooling-secret"},
					},
				},
			},
		},
		{
			name: "pooling enabled without creds still renders pooling.enabled",
			config: Config{
				Components: []string{api.ComponentDBProxy, api.ComponentPooling},
			},
			want: helmValues{
				DBProxy: dbProxyValues{Enabled: true, Pooling: poolingValues{Enabled: true}},
			},
		},
	}

	base := Config{
		APIURL: "https://api.cast.ai",
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := require.New(t)

			cfg := c.config
			if cfg.APIURL == "" {
				cfg.APIURL = base.APIURL
			}

			c.want.DBAgent.APIURL = cfg.APIURL
			r.Equal(c.want, newHelmValues(cfg))
		})
	}
}

// TestBuildHelmArgv_Deterministic locks down the exact argv shape:
// only the release coordinates, every chart parameter over stdin.
func TestBuildHelmArgv_Deterministic(t *testing.T) {
	r := require.New(t)

	cfg := Config{
		ReleaseName:  "castai-dbo",
		ChartName:    "castai-dbo",
		Namespace:    "castai-dbo",
		KubeContext:  "kind-test",
		ChartVersion: "1.4.2",
	}

	want := []string{
		"upgrade", "--install",
		"castai-dbo",
		"castai-dbo",
		"--version", "1.4.2",
		"--namespace", "castai-dbo",
		"--kube-context", "kind-test",
		"--create-namespace",
		"-f", "-",
	}

	r.Equal(want, BuildHelmArgv(cfg))
}

// TestHelmValuesMarshal_OmitsEmptySections pins the omitempty
// behavior: empty sections must not render, or they would override
// chart defaults with empty values.
func TestHelmValuesMarshal_OmitsEmptySections(t *testing.T) {
	r := require.New(t)

	out, err := yaml.Marshal(helmValues{})
	r.NoError(err)

	rendered := string(out)
	r.NotContains(rendered, "apiURL:")
	r.NotContains(rendered, "apiKeySecretRef:")
	r.NotContains(rendered, "database:")
	r.NotContains(rendered, "userSecretRef:")
	r.Contains(rendered, "db-agent:")
	r.Contains(rendered, "db-proxy:")
	r.Contains(rendered, "pooling:")
	// The component toggles always render, true or false.
	r.Contains(rendered, "enabled: false")
}
