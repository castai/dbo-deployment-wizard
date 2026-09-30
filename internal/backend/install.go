package backend

import (
	"context"
	"fmt"
	"os"

	"al.essio.dev/pkg/shellescape"
	"gopkg.in/yaml.v3"

	"github.com/castai/dbo-deployment-wizard/internal/api"
)

// Helm install generator: renders the Config into a temporary values
// file, builds the deterministic argv, and executes helm. Chart
// parameters — including secrets — travel in the values file, never
// on the argv, and the file is deleted right after it is applied.

// helmValues is the values.yaml schema the castai-dbo chart consumes
// (umbrella aliases: db-agent, db-proxy). All credentials travel as
// secret refs — the wizard creates the Secrets before rendering.
type helmValues struct {
	DBAgent dbAgentValues `yaml:"db-agent"`
	DBProxy dbProxyValues `yaml:"db-proxy"`
}

type dbAgentValues struct {
	Enabled bool   `yaml:"enabled"`
	APIURL  string `yaml:"apiURL,omitempty"`
	// APIKeySecretRef names the Secret carrying the CAST AI API key
	// (API_KEY); the wizard recreates it on every install.
	APIKeySecretRef string          `yaml:"apiKeySecretRef,omitempty"`
	Database        dbAgentDatabase `yaml:"database,omitempty"`
}

type dbAgentDatabase struct {
	// CredentialsSecretRef names the Secret holding the agent's
	// database user (DATABASE_USERNAME / DATABASE_PASSWORD keys).
	CredentialsSecretRef string `yaml:"credentialsSecretRef,omitempty"`
}

type dbProxyValues struct {
	Enabled         bool          `yaml:"enabled"`
	APIKeySecretRef string        `yaml:"apiKeySecretRef,omitempty"`
	Pooling         poolingValues `yaml:"pooling"`
}

type poolingValues struct {
	Enabled  bool           `yaml:"enabled"`
	ProxySQL proxySQLValues `yaml:"proxySql,omitempty"`
}

type proxySQLValues struct {
	// UserSecretRef names the Secret holding the pooler's upstream DB
	// user (DATABASE_USERNAME / DATABASE_PASSWORD keys); ProxySQL
	// (MySQL) only — pgdog authenticates pass-through.
	UserSecretRef string `yaml:"userSecretRef,omitempty"`
}

func newHelmValues(c Config) helmValues {
	var v helmValues
	v.DBAgent.Enabled = c.HasComponent(api.ComponentDBAgent)
	v.DBAgent.APIURL = c.APIURL
	v.DBAgent.Database.CredentialsSecretRef = c.AgentCreds.SecretName
	v.DBProxy.Enabled = c.HasComponent(api.ComponentDBProxy)
	v.DBProxy.Pooling.Enabled = c.HasComponent(api.ComponentPooling)
	v.DBProxy.Pooling.ProxySQL.UserSecretRef = c.PoolingCreds.SecretName

	return v
}

// writeTempValues writes data to a fresh temporary values file and
// returns its path; the caller owns the removal. 0600 — it carries
// secrets.
func writeTempValues(data []byte) (string, error) {
	f, err := os.CreateTemp("", "deployment-wizard-values-*.yaml")
	if err != nil {
		return "", fmt.Errorf("create temp values file: %w", err)
	}
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		_ = os.Remove(f.Name())

		return "", fmt.Errorf("write temp values file: %w", err)
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(f.Name())

		return "", fmt.Errorf("close temp values file: %w", err)
	}

	return f.Name(), nil
}

// BuildHelmArgv produces the `helm upgrade --install` argv: only the
// release coordinates; every chart parameter travels in the values
// file at valuesPath. Deterministic so --dry-run output matches the
// live run.
func BuildHelmArgv(c Config, valuesPath string) []string {
	return []string{
		"upgrade",
		"--install",
		c.ReleaseName,
		c.ChartName,
		"--version", c.ChartVersion,
		"--namespace", c.Namespace,
		"--kube-context", c.KubeContext,
		"--create-namespace",
		"-f", valuesPath,
	}
}

// Install renders cfg into a temporary values file and runs `helm
// upgrade --install`
func Install(ctx context.Context, values helmValues, cfg Config, shell ProcessRunner) error {
	data, err := yaml.Marshal(values)
	if err != nil {
		return fmt.Errorf("render helm values: %w", err)
	}

	path, err := writeTempValues(data)
	if err != nil {
		return err
	}
	defer os.Remove(path)

	argv := BuildHelmArgv(cfg, path)

	if cfg.DryRun {
		fmt.Fprintln(os.Stdout, "# Dry run: not executing. Values that would be applied:")
		fmt.Fprintln(os.Stdout, string(data))
		fmt.Fprintln(os.Stdout, "# Exact command that would run:")
		fmt.Fprintln(os.Stdout, "helm "+shellescape.QuoteCommand(argv))

		return nil
	}

	out, err := shell.Run(ctx, "helm", nil, argv)
	fmt.Fprint(os.Stdout, string(out))
	if err != nil {
		return fmt.Errorf("helm upgrade: %w", err)
	}

	return nil
}
