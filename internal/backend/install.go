package backend

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"strings"

	"al.essio.dev/pkg/shellescape"
	"github.com/samber/lo"
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
	// OrganizationID associates the deployment with its CAST AI
	// organization; the chart requires it on every component.
	OrganizationID string `yaml:"organizationID,omitempty"`
	// DatabaseInstanceID identifies the database instance the agent
	// manages; the db-agent chart requires it.
	DatabaseInstanceID string `yaml:"databaseInstanceID,omitempty"`
	// GRPCEndpoint is the CAST AI gRPC endpoint the agent reports
	// through; the db-agent chart requires it.
	GRPCEndpoint string `yaml:"gRPCEndpoint,omitempty"`
	// APIKeySecretRef names the Secret carrying the CAST AI API key
	// (API_KEY); the wizard recreates it on every install.
	APIKeySecretRef string          `yaml:"apiKeySecretRef,omitempty"`
	Database        dbAgentDatabase `yaml:"database,omitempty"`
}

type dbAgentDatabase struct {
	Host     string `yaml:"host,omitempty"`
	Port     int    `yaml:"port,omitempty"`
	Protocol string `yaml:"protocol,omitempty"`
	// CredentialsSecretRef names the Secret holding the agent's
	// database user (DATABASE_USERNAME / DATABASE_PASSWORD keys).
	CredentialsSecretRef string `yaml:"credentialsSecretRef,omitempty"`
}

type dbProxyValues struct {
	Enabled        bool   `yaml:"enabled"`
	OrganizationID string `yaml:"organizationID,omitempty"`
	// APIURL is the scheme-less CAST AI endpoint the proxy reports
	// through; the chart configmap prepends the https:// scheme.
	APIURL string `yaml:"apiURL,omitempty"`
	// ProxyID identifies this proxy instance; the db-proxy chart
	// requires it.
	ProxyID         string                  `yaml:"proxyID,omitempty"`
	Protocol        string                  `yaml:"protocol,omitempty"`
	APIKeySecretRef string                  `yaml:"apiKeySecretRef,omitempty"`
	Endpoints       []dbProxyEndpointValues `yaml:"endpoints,omitempty"`
	Pooling         poolingValues           `yaml:"pooling"`
}

// dbProxyEndpointValues is one upstream database endpoint the proxy
// forwards to, as host:port.
type dbProxyEndpointValues struct {
	Address  string `yaml:"address"`
	Readonly bool   `yaml:"readonly"`
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
	v.DBAgent.OrganizationID = c.OrganizationID
	v.DBAgent.DatabaseInstanceID = c.DatabaseInstanceID
	v.DBAgent.GRPCEndpoint = c.GRPCEndpoint
	v.DBAgent.Database.Protocol = c.Protocol
	if len(c.Endpoints) > 0 {
		v.DBAgent.Database.Host = c.Endpoints[0].Host
		v.DBAgent.Database.Port = c.Endpoints[0].Port
	}
	v.DBAgent.Database.CredentialsSecretRef = c.agentCreds().SecretName
	v.DBProxy.Enabled = c.HasComponent(api.ComponentDBProxy)
	v.DBProxy.APIURL = grpcAPIURL(c.GRPCEndpoint)
	v.DBProxy.OrganizationID = c.OrganizationID
	v.DBProxy.ProxyID = c.ProxyID
	v.DBProxy.Protocol = proxyChartProtocol(c.Protocol)
	if len(c.Endpoints) > 0 {
		v.DBProxy.Endpoints = lo.Map(c.Endpoints, func(e Endpoint, _ int) dbProxyEndpointValues {
			return dbProxyEndpointValues{Address: fmt.Sprintf("%s:%d", e.Host, e.Port), Readonly: e.Readonly}
		})
	}
	v.DBProxy.Pooling.Enabled = c.HasComponent(api.ComponentPooling)
	v.DBProxy.Pooling.ProxySQL.UserSecretRef = c.poolingCreds().SecretName

	return v
}

// chartRef is the helm chart reference in the repo_name/chart form
// helm requires for repository charts; the repo is registered under
// the alias by ensureRepo before the install runs.
func chartRef(c Config) string {
	return c.ChartRepoAlias + "/" + c.ChartName
}

// grpcAPIURL adapts the gRPC endpoint for db-proxy's apiURL value,
// which must be scheme-less — the chart configmap prepends https://.
func grpcAPIURL(endpoint string) string {
	return strings.TrimPrefix(strings.TrimPrefix(endpoint, "https://"), "http://")
}

// proxyChartProtocol maps the wizard's protocol to db-proxy's
// capitalized form; db-agent consumes the lowercase form directly.
// An unset protocol maps to an unset value.
func proxyChartProtocol(protocol string) string {
	switch protocol {
	case protocolMySQL:
		return "MySQL"
	case protocolPostgres:
		return "PostgreSQL"
	default:
		return ""
	}
}

// repoBaseURL derives the helm repository base URL from the index
// URL the wizard lists versions from, by dropping a trailing
// index.yaml document along with any query and fragment.
func repoBaseURL(indexURL string) (string, error) {
	u, err := url.Parse(indexURL)
	if err != nil {
		return "", fmt.Errorf("parse chart repository URL %q: %w", indexURL, err)
	}
	if !u.IsAbs() || u.Host == "" {
		return "", fmt.Errorf("chart repository URL %q must be an absolute URL with a host", indexURL)
	}

	u.Path = strings.TrimSuffix(u.Path, "/")
	u.Path, _ = strings.CutSuffix(u.Path, "/index.yaml")
	u.RawPath = ""
	u.RawQuery = ""
	u.Fragment, u.RawFragment = "", ""

	return u.String(), nil
}

// BuildHelmArgv produces the `helm upgrade --install` argv: only the
// release coordinates; every chart parameter travels over stdin
// (-f -). Deterministic so --dry-run output matches the live run.
func BuildHelmArgv(c Config) []string {
	return []string{
		"upgrade",
		"--install",
		c.ReleaseName,
		chartRef(c),
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

	// A blank line separates the transcript from whatever preceded it
	// on the terminal.
	fmt.Fprintln(stdout)
	fmt.Fprintf(stdout, "Installing helm chart %s:%s as '%s/%s'\n",
		chartRef(cfg), cfg.ChartVersion, cfg.Namespace, cfg.ReleaseName)

	if err := ensureRepo(ctx, cfg, shell, stdout); err != nil {
		return err
	}

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

// ensureRepo registers the chart repository under the config's alias
// so the repo_name/chart reference resolves; --force-update keeps an
// existing registration fresh. A dry run prints the command instead.
func ensureRepo(ctx context.Context, cfg Config, shell ProcessRunner, stdout io.Writer) error {
	base, err := repoBaseURL(cfg.ChartRepo)
	if err != nil {
		return err
	}

	argv := []string{"repo", "add", cfg.ChartRepoAlias, base, "--force-update"}

	if cfg.DryRun {
		fmt.Fprintln(stdout, "# Dry run: not executing. Repository command that would run:")
		fmt.Fprintln(stdout, "helm "+shellescape.QuoteCommand(argv))

		return nil
	}

	out, err := shell.Run(ctx, "helm", nil, argv)
	fmt.Fprint(stdout, string(out))
	if err != nil {
		return fmt.Errorf("helm repo add: %w", err)
	}

	return nil
}
