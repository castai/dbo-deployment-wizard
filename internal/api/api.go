// Package api is the deployment wizard's client-facing API layer:
// the Backend contract frontends drive the service layer through
// without importing the service implementation itself.
package api

// Backend is the client-facing contract frontends drive the wizard
// through. Queries return read-only views of the committed state;
// commands are the only write path and enforce the domain
// invariants, so any state reachable through the contract is valid.
type Backend interface {
	// --- queries ---

	KubeContext() string
	Namespace() string
	ChartVersion() string
	AgentCredentials() Credentials
	PoolingCredentials() Credentials
	// Components returns the enabled components in selection order.
	Components() []string
	KubeContexts() []string
	ChartVersions() []HelmChartVersion
	// LatestChartVersion returns "" when the repo lookup failed.
	LatestChartVersion() string
	// Secrets returns the Secret names in the selected context and
	// namespace; a discovery failure returns none.
	Secrets() []string

	// --- commands ---

	SetKubeContext(name string)
	SetNamespace(ns string)
	SetChartVersion(version string)
	SetComponents(components []string) error
	// SetAgentCredentials sets the db-agent's database credentials;
	// the referenced Secret takes precedence over the username/password
	// pair. SetPoolingCredentials does the same for pooling.
	SetAgentCredentials(creds Credentials)
	SetPoolingCredentials(creds Credentials)

	// --- actions ---

	Install() error
}
