package backend

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/castai/dbo-deployment-wizard/internal/api"
)

func TestNewDefaultConfig(t *testing.T) {
	r := require.New(t)
	cfg := NewDefaultConfig()

	r.Equal([]string{api.ComponentDBAgent, api.ComponentDBProxy}, cfg.Components)

	static := map[string]string{
		"APIURL":       cfg.APIURL,
		"ChartRepo":    cfg.ChartRepo,
		"ChartName":    cfg.ChartName,
		"ReleaseName":  cfg.ReleaseName,
		"Namespace":    cfg.Namespace,
		"GRPCEndpoint": cfg.GRPCEndpoint,
	}
	for name, v := range static {
		r.NotEmpty(v, "NewDefaultConfig must prefill %s", name)
	}

	// The discovery-based and CLI-required fields have no static
	// default.
	r.Empty(cfg.KubeContext)
	r.Empty(cfg.ChartVersion)
	r.Empty(cfg.Protocol)
	r.Empty(cfg.Endpoints)
}

// TestConfigSetEndpoints pins the endpoint argument grammar: host:port
// pairs, the readonly suffix, and the parse failures.
func TestConfigSetEndpoints(t *testing.T) {
	tests := map[string]struct {
		raw     []string
		want    []Endpoint
		wantErr string
	}{
		"none is valid":      {raw: nil, want: []Endpoint{}},
		"host and port":      {raw: []string{"db.example.com:5432"}, want: []Endpoint{{Host: "db.example.com", Port: 5432}}},
		"readonly endpoint":  {raw: []string{"db.example.com:5433,readonly"}, want: []Endpoint{{Host: "db.example.com", Port: 5433, Readonly: true}}},
		"multiple endpoints": {raw: []string{"db.example.com:5432", "ro.example.com:5433,readonly"}, want: []Endpoint{{Host: "db.example.com", Port: 5432}, {Host: "ro.example.com", Port: 5433, Readonly: true}}},
		"ipv6 host":          {raw: []string{"[::1]:5432"}, want: []Endpoint{{Host: "::1", Port: 5432}}},
		"missing port":       {raw: []string{"db.example.com"}, wantErr: `invalid endpoint "db.example.com": want host:port or host:port,readonly`},
		"non-numeric port":   {raw: []string{"db.example.com:abc"}, wantErr: `invalid endpoint "db.example.com:abc": port must be a number between 1 and 65535`},
		"port out of range":  {raw: []string{"db.example.com:0"}, wantErr: `invalid endpoint "db.example.com:0": port must be a number between 1 and 65535`},
		"unknown suffix":     {raw: []string{"db.example.com:5432,ro"}, wantErr: `invalid endpoint "db.example.com:5432,ro": want host:port or host:port,readonly`},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			r := require.New(t)

			c := NewDefaultConfig()
			err := c.SetEndpoints(tt.raw)
			if tt.wantErr != "" {
				r.EqualError(err, tt.wantErr)
				return
			}
			r.NoError(err)
			r.Equal(tt.want, c.Endpoints)
		})
	}
}

func TestConfigSetComponents(t *testing.T) {
	t.Run("unknown name errors", func(t *testing.T) {
		r := require.New(t)
		c := NewDefaultConfig()
		r.Error(c.SetComponents([]string{"db-agent", "bogus"}))
	})

	t.Run("dedups while preserving order", func(t *testing.T) {
		r := require.New(t)
		c := NewDefaultConfig()
		r.NoError(c.SetComponents([]string{"pooling", "db-proxy", "db-proxy", "db-agent"}))
		r.Equal([]string{"pooling", "db-proxy", "db-agent"}, c.Components)
	})

	t.Run("pooling without db-proxy errors", func(t *testing.T) {
		r := require.New(t)
		c := NewDefaultConfig()
		r.Error(c.SetComponents([]string{"pooling"}))
	})

	t.Run("deselecting pooling clears credentials", func(t *testing.T) {
		r := require.New(t)
		c := NewDefaultConfig()
		c.PoolingCreds = api.Credentials{Username: "u", Password: "p"}
		r.NoError(c.SetComponents([]string{"db-agent", "db-proxy"}))
		r.Empty(c.PoolingCreds)
	})
}

func TestConfigSetComponentEnabled(t *testing.T) {
	t.Run("enabling pooling without db-proxy errors", func(t *testing.T) {
		r := require.New(t)
		c := NewDefaultConfig()
		r.NoError(c.SetComponents([]string{api.ComponentDBAgent}))
		r.Error(c.SetComponentEnabled(api.ComponentPooling, true))
	})

	t.Run("enabling with db-proxy present works", func(t *testing.T) {
		r := require.New(t)
		c := NewDefaultConfig()
		r.NoError(c.SetComponentEnabled(api.ComponentPooling, true))
		r.True(c.HasComponent(api.ComponentPooling))
	})

	t.Run("disabling db-proxy while pooling is enabled errors", func(t *testing.T) {
		r := require.New(t)
		c := NewDefaultConfig()
		r.NoError(c.SetComponentEnabled(api.ComponentPooling, true))
		r.Error(c.SetComponentEnabled(api.ComponentDBProxy, false))
		// A rejected toggle leaves the selection untouched.
		r.True(c.HasComponent(api.ComponentDBProxy))
		r.True(c.HasComponent(api.ComponentPooling))
	})

	t.Run("disabling pooling clears credentials", func(t *testing.T) {
		r := require.New(t)
		c := NewDefaultConfig()
		r.NoError(c.SetComponentEnabled(api.ComponentPooling, true))
		c.PoolingCreds = api.Credentials{Username: "u", Password: "p"}

		r.NoError(c.SetComponentEnabled(api.ComponentPooling, false))
		r.False(c.HasComponent(api.ComponentPooling))
		r.Empty(c.PoolingCreds)
	})
}
