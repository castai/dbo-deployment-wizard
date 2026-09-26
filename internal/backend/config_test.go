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
		"APIURL":      cfg.APIURL,
		"ChartRepo":   cfg.ChartRepo,
		"ChartName":   cfg.ChartName,
		"ReleaseName": cfg.ReleaseName,
		"Namespace":   cfg.Namespace,
	}
	for name, v := range static {
		r.NotEmpty(v, "NewDefaultConfig must prefill %s", name)
	}

	// The discovery-based fields have no static default.
	r.Empty(cfg.KubeContext)
	r.Empty(cfg.ChartVersion)
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
