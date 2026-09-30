package backend

import (
	"context"
	"fmt"
	"io"

	"al.essio.dev/pkg/shellescape"
	"gopkg.in/yaml.v3"

	"github.com/castai/dbo-deployment-wizard/internal/api"
)

// Helm install generator: renders the Config into values, builds
// the deterministic argv, and executes helm. Chart parameters —
// including secrets — travel over stdin, never on the argv or disk.

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

// BuildHelmArgv produces the `helm upgrade --install` argv: only the
// release coordinates; every chart parameter travels over stdin
// (-f -). Deterministic so --dry-run output matches the live run.
func BuildHelmArgv(c Config) []string {
	return []string{
		"upgrade",
		"--install",
		c.ReleaseName,
		c.ChartName,
		"--version", c.ChartVersion,
		"--namespace", c.Namespace,
		"--kube-context", c.KubeContext, //nolint:goconst
		"--create-namespace",
		"-f", "-",
	}
}

// Install runs `helm upgrade --install` with the rendered values
// piped over stdin, so they never touch the argv or disk. stdout
// carries the CLI transcript: the handoff blank line and banner,
// helm's captured output, or the dry-run values and exact command.
func Install(ctx context.Context, values helmValues, cfg Config, shell ProcessRunner, stdout io.Writer) error {
	data, err := yaml.Marshal(values)
	if err != nil {
		return fmt.Errorf("render helm values: %w", err)
	}

	// A blank line hands off from the TUI's last frame.
	fmt.Fprintln(stdout)
	fmt.Fprintf(stdout, "Installing helm chart %s:%s as '%s/%s'\n",
		cfg.ChartName, cfg.ChartVersion, cfg.Namespace, cfg.ReleaseName)

	argv := BuildHelmArgv(cfg)

	if cfg.DryRun {
		fmt.Fprintln(stdout, "# Dry run: not executing. Values that would be applied:")
		fmt.Fprintln(stdout, string(data))
		fmt.Fprintln(stdout, "# Exact command that would run:")
		fmt.Fprintln(stdout, "helm "+shellescape.QuoteCommand(argv))

		return nil
	}

	out, err := shell.Run(ctx, "helm", data, argv)
	fmt.Fprint(stdout, string(out))
	if err != nil {
		return fmt.Errorf("helm upgrade: %w", err)
	}

	return nil
}
