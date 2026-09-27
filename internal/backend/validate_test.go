package backend

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/castai/dbo-deployment-wizard/internal/api"
)

func TestConfigValidate(t *testing.T) {
	// complete returns a config with every prerequisite met.
	complete := func(mutate func(*Config)) Config {
		cfg := NewDefaultConfig()
		cfg.KubeContext = "kind-test"
		cfg.ChartVersion = "1.4.2"
		cfg.AgentCreds = api.Credentials{Username: "agent", Password: "agentpass"}
		if mutate != nil {
			mutate(&cfg)
		}

		return cfg
	}

	tests := map[string]struct {
		mutate  func(*Config)
		wantErr string
	}{
		"complete config": {mutate: nil, wantErr: ""},
		"missing namespace": {
			mutate:  func(c *Config) { c.Namespace = "" },
			wantErr: "namespace is required",
		},
		"missing agent credentials": {
			mutate:  func(c *Config) { c.AgentCreds = api.Credentials{} },
			wantErr: "agent credentials are required",
		},
		"agent credentials not required without db-agent": {
			mutate: func(c *Config) {
				c.AgentCreds = api.Credentials{}
				require.NoError(t, c.SetComponents([]string{api.ComponentDBProxy}))
			},
		},
		"missing pooling credentials": {
			mutate: func(c *Config) {
				require.NoError(t, c.SetComponents([]string{api.ComponentDBAgent, api.ComponentDBProxy, api.ComponentPooling}))
			},
			wantErr: "pooling credentials are required",
		},
		"pooling credentials not required without pooling": {
			mutate: func(c *Config) {
				c.PoolingCreds = api.Credentials{}
			},
		},
		"every missing prerequisite joins into one message": {
			mutate: func(c *Config) {
				c.Namespace = ""
				c.AgentCreds = api.Credentials{}
				require.NoError(t, c.SetComponents([]string{api.ComponentDBAgent, api.ComponentDBProxy, api.ComponentPooling}))
			},
			wantErr: "namespace is required; agent credentials are required; pooling credentials are required",
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			r := require.New(t)

			cfg := complete(tt.mutate)
			err := cfg.validate()
			if tt.wantErr == "" {
				r.NoError(err)

				return
			}
			r.EqualError(err, tt.wantErr)
		})
	}
}

func TestWizardValidate(t *testing.T) {
	r := require.New(t)

	// The default config carries no agent credentials, so the
	// facade's validation reports exactly that.
	w := &Wizard{cfg: NewDefaultConfig()}
	r.EqualError(w.Validate(), "agent credentials are required")

	w.cfg.AgentCreds = api.Credentials{SecretName: "agent-secret"}
	r.NoError(w.Validate())
}
