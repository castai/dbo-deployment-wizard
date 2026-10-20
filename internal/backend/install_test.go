package backend

import (
	"bytes"
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
		ReleaseName:        "castai-dbo",
		ChartName:          "castai-dbo",
		Namespace:          "castai-dbo",
		KubeContext:        "kind-test",
		ChartVersion:       "1.4.2",
		ChartRepo:          "https://charts.example.com/index.yaml",
		OrganizationID:     "org-123",
		DatabaseInstanceID: "db-123",
		ProxyID:            "proxy-123",
		Components:         []string{api.ComponentDBProxy, api.ComponentPooling},
		APIURL:             "https://api.cast.ai",
		APISecret:          "sec",
		DryRun:             true,
	}
	cfg.ChartRepoAlias = defaultChartRepoAlias

	// The strict mock fails on any shell run — a dry run must not execute.
	sh := backendmocks.NewMockProcessRunner(t)

	var out bytes.Buffer
	values := newHelmValues(cfg)
	r.NoError(Install(t.Context(), values, cfg, sh, &out))

	rendered := out.String()
	r.Contains(rendered, "Installing helm chart castai-helm/castai-dbo:1.4.2 as 'castai-dbo/castai-dbo'")
	r.Contains(rendered, "# Dry run: not executing. Repository command that would run:")
	r.Contains(rendered, "helm repo add castai-helm https://charts.example.com --force-update")
	r.Contains(rendered, "# Dry run: not executing. Values that would be applied:")
	r.Contains(rendered, "enabled: true")
	r.Contains(rendered, "organizationID: org-123")
	r.Contains(rendered, "databaseInstanceID: db-123")
	r.Contains(rendered, "proxyID: proxy-123")
	r.Contains(rendered, "# Exact command that would run:")
	r.Contains(rendered, "helm upgrade --install castai-dbo castai-helm/castai-dbo --version 1.4.2 --namespace castai-dbo --kube-context kind-test --create-namespace -f -") //nolint:dupword
}

func TestExecutorLiveRun_InvokesHelm(t *testing.T) {
	r := require.New(t)

	cfg := Config{
		ReleaseName:        "castai-dbo",
		ChartName:          "castai-dbo",
		Namespace:          "castai-dbo",
		KubeContext:        "kind-test",
		ChartVersion:       "1.4.2",
		ChartRepo:          "https://charts.example.com/index.yaml",
		OrganizationID:     "org-123",
		DatabaseInstanceID: "db-123",
		ProxyID:            "proxy-123",
		Components:         []string{api.ComponentDBAgent, api.ComponentDBProxy, api.ComponentPooling},
		AgentCreds:         api.Credentials{Username: "agent", Password: "agentpass"},
		PoolingCreds:       api.Credentials{Username: "pooler", Password: "poolpass"},
		APIURL:             "https://api.cast.ai",
		APISecret:          "sec",
	}
	cfg.ChartRepoAlias = defaultChartRepoAlias

	k := backendmocks.NewMockKubectl(t)
	k.EXPECT().ListSecrets(mock.Anything, "kind-test", "castai-dbo").Return(nil, nil)
	k.EXPECT().EnsureSecret(mock.Anything, "kind-test", "castai-dbo", "castai-dbo-agent-credentials",
		map[string]string{"DATABASE_USERNAME": "agent", "DATABASE_PASSWORD": "agentpass"}, mock.Anything, mock.Anything).Return(nil)
	k.EXPECT().EnsureSecret(mock.Anything, "kind-test", "castai-dbo", "castai-dbo-pooling-credentials",
		map[string]string{"DATABASE_USERNAME": "pooler", "DATABASE_PASSWORD": "poolpass"}, mock.Anything, mock.Anything).Return(nil)
	k.EXPECT().EnsureSecret(mock.Anything, "kind-test", "castai-dbo", "castai-dbo-api-key",
		map[string]string{"API_KEY": "sec"}, mock.Anything, mock.Anything).Return(nil)
	k.EXPECT().RolloutStatus(mock.Anything, "kind-test", "castai-dbo", "deployment.apps/db-agent", rolloutTimeout,
		mock.Anything, mock.Anything).Return(nil)

	var valuesBytes []byte
	sh := backendmocks.NewMockProcessRunner(t)
	sh.EXPECT().Run(mock.Anything, "helm", mock.Anything,
		mock.MatchedBy(func(args []string) bool { return args[0] == "repo" })).
		Return([]byte("\"castai-helm\" has been added to your repositories\n"), nil)
	sh.EXPECT().Run(mock.Anything, "helm", mock.Anything,
		mock.MatchedBy(func(args []string) bool { return args[0] == "upgrade" })).
		Run(func(_ context.Context, _ string, stdin []byte, _ []string) {
			valuesBytes = stdin
		}).
		Return([]byte("Release \"castai-dbo\" has been upgraded.\n"), nil)
	sh.EXPECT().Run(mock.Anything, "helm", mock.Anything,
		mock.MatchedBy(func(args []string) bool { return args[0] == "get" })).
		Return([]byte("---\napiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: db-agent\n"), nil)

	w := &Wizard{ctx: t.Context(), k: k, shell: sh, cfg: cfg}
	var out bytes.Buffer
	r.NoError(w.Install(&out))

	// The transcript logs every Secret write, the banner, helm's
	// output, and the rollout watch.
	rendered := out.String()
	r.Contains(rendered, "creating secret castai-dbo-agent-credentials")
	r.Contains(rendered, "creating secret castai-dbo-pooling-credentials")
	r.Contains(rendered, "creating secret castai-dbo-api-key")
	r.Contains(rendered, "Installing helm chart castai-helm/castai-dbo:1.4.2 as 'castai-dbo/castai-dbo'")
	r.Contains(rendered, "\"castai-helm\" has been added to your repositories")
	r.Contains(rendered, "Waiting for deployments of release 'castai-dbo' in 'castai-dbo' to be ready")
	r.Contains(rendered, "Monitoring: deployment.apps/db-agent")

	// Username/password pairs resolve into Secret refs, and every
	// enabled component points at the recreated API key Secret.
	var got helmValues
	r.NoError(yaml.Unmarshal(valuesBytes, &got))
	r.Equal(helmValues{
		DBAgent: dbAgentValues{
			Enabled:            true,
			APIURL:             "https://api.cast.ai",
			OrganizationID:     "org-123",
			DatabaseInstanceID: "db-123",
			APIKeySecretRef:    "castai-dbo-api-key",
			Database:           dbAgentDatabase{CredentialsSecretRef: "castai-dbo-agent-credentials"},
		},
		DBProxy: dbProxyValues{
			Enabled:         true,
			OrganizationID:  "org-123",
			ProxyID:         "proxy-123",
			APIKeySecretRef: "castai-dbo-api-key",
			Pooling: poolingValues{
				Enabled:  true,
				ProxySQL: proxySQLValues{UserSecretRef: "castai-dbo-pooling-credentials"},
			},
		},
	}, got)
}

func TestWizardDryRun_PrintsSecretCommands(t *testing.T) {
	r := require.New(t)

	cfg := Config{
		ReleaseName:  "castai-dbo",
		ChartName:    "castai-dbo",
		Namespace:    "castai-dbo",
		KubeContext:  "kind-test",
		ChartVersion: "1.4.2",
		ChartRepo:    "https://charts.example.com/index.yaml",
		Components:   []string{api.ComponentDBAgent, api.ComponentDBProxy, api.ComponentPooling},
		AgentCreds:   api.Credentials{Username: "agent", Password: "agentpass"},
		PoolingCreds: api.Credentials{Username: "pooler", Password: "poolpass"},
		APIURL:       "https://api.cast.ai",
		APISecret:    "sec",
		DryRun:       true,
	}
	cfg.ChartRepoAlias = defaultChartRepoAlias

	// A dry run must not touch the cluster: the zero-expectation shell
	// fails on any run; RealKubectl prints the secret commands.
	sh := backendmocks.NewMockProcessRunner(t)
	k := &RealKubectl{shell: sh}

	w := &Wizard{ctx: t.Context(), k: k, shell: sh, cfg: cfg}
	var out bytes.Buffer
	r.NoError(w.Install(&out))

	// Each Secret write prints the apply command it would run.
	apply := "kubectl --context kind-test -n castai-dbo apply -f -  # "
	r.Contains(out.String(), apply+"castai-dbo-agent-credentials")
	r.Contains(out.String(), apply+"castai-dbo-pooling-credentials")
	r.Contains(out.String(), apply+"castai-dbo-api-key")
	r.Contains(out.String(), "# Dry run: not executing. Values that would be applied:")
}

// Credentials entered for a component that was later deselected must
// not reach the cluster: the strict mock fails on any Secret but the
// API key's.
func TestWizardInstall_SkipsDisabledComponentsSecrets(t *testing.T) {
	r := require.New(t)

	cfg := Config{
		ReleaseName:  "castai-dbo",
		ChartName:    "castai-dbo",
		Namespace:    "castai-dbo",
		KubeContext:  "kind-test",
		ChartVersion: "1.4.2",
		ChartRepo:    "https://charts.example.com/index.yaml",
		Components:   []string{api.ComponentDBProxy},
		AgentCreds:   api.Credentials{Username: "agent", Password: "agentpass"},
		PoolingCreds: api.Credentials{Username: "pooler", Password: "poolpass"},
		APIURL:       "https://api.cast.ai",
		APISecret:    "sec",
		DryRun:       true,
	}
	cfg.ChartRepoAlias = defaultChartRepoAlias

	k := backendmocks.NewMockKubectl(t)
	k.EXPECT().EnsureSecret(mock.Anything, "kind-test", "castai-dbo", "castai-dbo-api-key",
		map[string]string{"API_KEY": "sec"}, mock.Anything, true).Return(nil)

	w := &Wizard{ctx: t.Context(), k: k, shell: backendmocks.NewMockProcessRunner(t), cfg: cfg}
	var out bytes.Buffer
	r.NoError(w.Install(&out))

	r.NotContains(out.String(), "credentials")
	r.NotContains(out.String(), "SecretRef: castai-dbo-agent")
	r.NotContains(out.String(), "userSecretRef")
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
				Components: []string{api.ComponentDBAgent},
				AgentCreds: api.Credentials{SecretName: "my-agent-secret"},
			},
			want: helmValues{
				DBAgent: dbAgentValues{Enabled: true, Database: dbAgentDatabase{CredentialsSecretRef: "my-agent-secret"}},
			},
		},
		{
			name: "disabled components' secret refs are left out",
			config: Config{
				Components:   []string{api.ComponentDBProxy},
				AgentCreds:   api.Credentials{SecretName: "my-agent-secret"},
				PoolingCreds: api.Credentials{SecretName: "pooling-secret"},
			},
			want: helmValues{
				DBProxy: dbProxyValues{Enabled: true},
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
			name: "organization ID reaches every component",
			config: Config{
				Components:     []string{api.ComponentDBAgent, api.ComponentDBProxy},
				OrganizationID: "org-123",
			},
			want: helmValues{
				DBAgent: dbAgentValues{Enabled: true, OrganizationID: "org-123"},
				DBProxy: dbProxyValues{Enabled: true, OrganizationID: "org-123"},
			},
		},
		{
			name: "database instance ID reaches db-agent, proxy ID reaches db-proxy",
			config: Config{
				Components:         []string{api.ComponentDBAgent, api.ComponentDBProxy},
				DatabaseInstanceID: "db-123",
				ProxyID:            "proxy-123",
			},
			want: helmValues{
				DBAgent: dbAgentValues{Enabled: true, DatabaseInstanceID: "db-123"},
				DBProxy: dbProxyValues{Enabled: true, ProxyID: "proxy-123"},
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

// TestRepoBaseURL pins how the repository base URL is derived from
// the index URL: the trailing index.yaml document, query and
// fragment drop, and anything that is not an absolute URL errors.
func TestRepoBaseURL(t *testing.T) {
	tests := map[string]struct {
		indexURL string
		want     string
		wantErr  bool
	}{
		"default castai index":             {indexURL: "https://castai.github.io/helm-charts/index.yaml", want: "https://castai.github.io/helm-charts"},
		"root index":                       {indexURL: "https://charts.example.com/index.yaml", want: "https://charts.example.com"},
		"nested path":                      {indexURL: "https://charts.example.com/charts/index.yaml", want: "https://charts.example.com/charts"},
		"trailing slash after index.yaml":  {indexURL: "https://charts.example.com/index.yaml/", want: "https://charts.example.com"},
		"already a base URL":               {indexURL: "https://charts.example.com/charts/", want: "https://charts.example.com/charts"},
		"host-only URL":                    {indexURL: "https://charts.example.com", want: "https://charts.example.com"},
		"port is preserved":                {indexURL: "https://charts.example.com:8080/index.yaml", want: "https://charts.example.com:8080"},
		"query and fragment are dropped":   {indexURL: "https://charts.example.com/charts/index.yaml?token=abc#frag", want: "https://charts.example.com/charts"},
		"percent-encoded path round-trips": {indexURL: "https://charts.example.com/my%20charts/index.yaml", want: "https://charts.example.com/my%20charts"},
		"relative URL is rejected":         {indexURL: "charts.example.com/index.yaml", wantErr: true},
		"schemeless URL is rejected":       {indexURL: "//charts.example.com/index.yaml", wantErr: true},
		"whitespace in URL is rejected":    {indexURL: "https://exa mple.com/index.yaml", wantErr: true},
		"empty URL is rejected":            {indexURL: "", wantErr: true},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			r := require.New(t)

			got, err := repoBaseURL(tt.indexURL)
			if tt.wantErr {
				r.Error(err)
				return
			}
			r.NoError(err)
			r.Equal(tt.want, got)
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
	cfg.ChartRepoAlias = defaultChartRepoAlias

	want := []string{
		"upgrade", "--install",
		"castai-dbo",
		"castai-helm/castai-dbo",
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
	r.NotContains(rendered, "organizationID:")
	r.NotContains(rendered, "databaseInstanceID:")
	r.NotContains(rendered, "proxyID:")
	r.Contains(rendered, "db-agent:")
	r.Contains(rendered, "db-proxy:")
	r.Contains(rendered, "pooling:")
	// The component toggles always render, true or false.
	r.Contains(rendered, "enabled: false")
}
