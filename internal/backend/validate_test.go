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
		cfg.APISecret = "secret"
		cfg.DatabaseInstanceID = "db-123"
		cfg.ProxyID = "proxy-123"
		cfg.Protocol = protocolPostgres
		cfg.Endpoints = []Endpoint{{Host: "db.example.com", Port: 5432}}
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
		"missing api secret": {
			mutate:  func(c *Config) { c.APISecret = "" },
			wantErr: "api secret is required",
		},
		"missing release name": {
			mutate:  func(c *Config) { c.ReleaseName = "" },
			wantErr: "release name is required",
		},
		"missing chart name": {
			mutate:  func(c *Config) { c.ChartName = "" },
			wantErr: "chart name is required",
		},
		"missing kube context": {
			mutate:  func(c *Config) { c.KubeContext = "" },
			wantErr: "kubectl context is required",
		},
		"missing chart version": {
			mutate:  func(c *Config) { c.ChartVersion = "" },
			wantErr: "chart version is required",
		},
		"no components": {
			mutate:  func(c *Config) { require.NoError(t, c.SetComponents(nil)) },
			wantErr: "at least one component is required",
		},
		"missing database instance ID": {
			mutate:  func(c *Config) { c.DatabaseInstanceID = "" },
			wantErr: "database instance ID is required when db-agent is selected",
		},
		"database instance ID not required without db-agent": {
			mutate: func(c *Config) {
				c.DatabaseInstanceID = ""
				require.NoError(t, c.SetComponents([]string{api.ComponentDBProxy}))
			},
		},
		"missing proxy ID": {
			mutate:  func(c *Config) { c.ProxyID = "" },
			wantErr: "proxy ID is required when db-proxy is selected",
		},
		"proxy ID not required without db-proxy": {
			mutate: func(c *Config) {
				c.ProxyID = ""
				require.NoError(t, c.SetComponents([]string{api.ComponentDBAgent}))
			},
		},
		"invalid protocol": {
			mutate:  func(c *Config) { c.Protocol = "oracle" },
			wantErr: "protocol must be postgres or mysql",
		},
		"missing endpoints": {
			mutate:  func(c *Config) { c.Endpoints = nil },
			wantErr: "at least one endpoint is required",
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
		"every missing requirement joins into one message": {
			mutate: func(c *Config) {
				c.Namespace = ""
				c.APISecret = ""
				c.ReleaseName = ""
				c.ChartName = ""
				c.AgentCreds = api.Credentials{}
				require.NoError(t, c.SetComponents([]string{api.ComponentDBAgent, api.ComponentDBProxy, api.ComponentPooling}))
			},
			wantErr: "namespace is required\napi secret is required\nrelease name is required\nchart name is required\nagent credentials are required\npooling credentials are required",
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
