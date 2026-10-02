// Package backend is the deployment wizard's service layer: the single
// owner of the deployment configuration and the domain rules around
// it. It renders nothing and reads no keys.
package backend

import (
	"context"
	"fmt"
	"io"
	"os"
	"slices"

	"github.com/samber/lo"

	"github.com/castai/dbo-deployment-wizard/internal/api"
)

// Wizard is the domain facade owning the deployment configuration;
// it implements the Backend contract (internal/api). Commands
// enforce the invariants: pooling requires db-proxy, deselecting
// pooling drops it and clears its credentials, and a referenced
// Secret takes precedence over the username/password pair.
type Wizard struct {
	//nolint:containedctx // carries the cmd entry's context for discovery calls.
	ctx  context.Context
	repo *HelmRepoClient
	k    Kubectl
	// shell executes the helm install; tests inject a mock.
	shell ProcessRunner

	cfg Config

	latestVersion string
}

func (w *Wizard) ReleaseName() string {
	return w.cfg.ReleaseName
}

// Compile-time check: Wizard implements the full Backend contract.
var _ api.Backend = (*Wizard)(nil)

// NewWizard implements api.Backend
func NewWizard(ctx context.Context, cfg Config, repo *HelmRepoClient, k Kubectl, shell ProcessRunner) (*Wizard, error) {
	w := &Wizard{
		ctx:   ctx,
		repo:  repo,
		k:     k,
		shell: shell,
		cfg:   cfg,
	}

	if w.cfg.KubeContext == "" {
		if cur, err := k.CurrentKubeContext(ctx); err == nil && cur != "" {
			w.cfg.KubeContext = cur
		} else {
			return nil, fmt.Errorf("could not determine current kubernetes context: %w", err)
		}
	}

	if w.cfg.ChartVersion == "" && w.cfg.ChartName != "" {
		latest, err := repo.LatestVersion(ctx, cfg.ChartName)
		if err != nil {
			return nil, fmt.Errorf("fetch latest chart version: %w", err)
		}
		w.cfg.ChartVersion = latest
	}

	return w, nil
}

// --- queries ---

func (w *Wizard) KubeContext() string  { return w.cfg.KubeContext }
func (w *Wizard) Namespace() string    { return w.cfg.Namespace }
func (w *Wizard) ChartVersion() string { return w.cfg.ChartVersion }

func (w *Wizard) AgentCredentials() api.Credentials   { return w.cfg.AgentCreds }
func (w *Wizard) PoolingCredentials() api.Credentials { return w.cfg.PoolingCreds }

// Components returns the enabled components in selection order.
func (w *Wizard) Components() []string { return slices.Clone(w.cfg.Components) }

// KubeContexts returns the kubectl contexts discovered at construction.
func (w *Wizard) KubeContexts() ([]string, error) { return w.k.ListKubeContexts(w.ctx) }

// ChartVersions returns every published chart version, newest first.
func (w *Wizard) ChartVersions() ([]api.HelmChartVersion, error) {
	return w.repo.ListVersions(w.ctx, w.cfg.ChartName)
}

// LatestChartVersion returns the newest published version, or "" when
// the repo lookup failed.
func (w *Wizard) LatestChartVersion() string { return w.latestVersion }

// Secrets returns the names of the Secrets in the currently selected
// context and namespace; a discovery failure returns none (the UI
// falls back to free-text input).
func (w *Wizard) Secrets() []string {
	secrets, err := w.k.ListSecrets(w.ctx, w.cfg.KubeContext, w.cfg.Namespace)
	if err != nil {
		return nil
	}

	return secrets
}

// DeploymentExists reports whether the committed deployment
// coordinates — context, namespace, release name — already hold a
// helm release, i.e. whether Install would upgrade it rather than
// create a new installation. The kubectl layer memoizes lookups by
// coordinates.
func (w *Wizard) DeploymentExists() (bool, error) {
	return w.k.HelmReleaseExists(w.ctx, w.cfg.KubeContext, w.cfg.Namespace, w.cfg.ReleaseName)
}

// SummarizeInstallationImpact lists the actions the install will
// perform: the Helm deployment it creates or updates, and every
// Secret it creates or updates; the confirmation screen renders the
// lines verbatim.
func (w *Wizard) SummarizeInstallationImpact() ([]string, error) {
	exists, err := w.DeploymentExists()
	if err != nil {
		return nil, err
	}

	deployment := "New Helm deployment to be created:"
	if exists {
		deployment = "Existing Helm deployment will be updated:"
	}

	existingSecrets, _ := w.k.ListSecrets(w.ctx, w.cfg.KubeContext, w.cfg.Namespace)
	lines := []string{
		deployment + " " + w.cfg.Namespace + "/" + w.cfg.ReleaseName,
		secretLine("api key", w.cfg.ReleaseName+"-api-key", existingSecrets),
	}
	if w.cfg.AgentCreds.Username != "" && w.cfg.AgentCreds.Password != "" {
		lines = append(lines, secretLine("db agent", w.secretName("agent"), existingSecrets))
	}
	if w.cfg.PoolingCreds.Username != "" && w.cfg.PoolingCreds.Password != "" {
		lines = append(lines, secretLine("pooling", w.secretName("pooling"), existingSecrets))
	}

	return lines, nil
}

// secretLine names the credential-Secret action: updated when the
// Secret already exists, created otherwise.
func secretLine(component, name string, existing []string) string {
	if slices.Contains(existing, name) {
		return "Existing " + component + " secret will be updated: " + name
	}

	return "New " + component + " secret will be created: " + name
}

// --- commands ---

// SetKubeContext selects the kubectl context to install into.
func (w *Wizard) SetKubeContext(name string) { w.cfg.KubeContext = name }

// SetNamespace sets the target namespace.
func (w *Wizard) SetNamespace(ns string) { w.cfg.Namespace = ns }

// SetChartVersion sets the chart version to install.
func (w *Wizard) SetChartVersion(v string) { w.cfg.ChartVersion = v }

// SetComponents replaces the enabled component set, enforcing the
// curated-list and pooling⇒db-proxy rules; dropping pooling clears
// its credentials.
func (w *Wizard) SetComponents(components []string) error { return w.cfg.SetComponents(components) }

// SetAgentCredentials sets the db-agent's database credentials; the
// referenced Secret takes precedence over the username/password pair.
func (w *Wizard) SetAgentCredentials(c api.Credentials) {
	w.cfg.AgentCreds = credentialsWithPrecedence(c)
}

// SetPoolingCredentials sets the pooling credentials; the referenced
// Secret takes precedence over the username/password pair.
func (w *Wizard) SetPoolingCredentials(c api.Credentials) {
	w.cfg.PoolingCreds = credentialsWithPrecedence(c)
}

// credentialsWithPrecedence drops the username/password pair when a
// Secret is referenced.
func credentialsWithPrecedence(c api.Credentials) api.Credentials {
	if c.SecretName != "" {
		c.Username, c.Password = "", ""
	}

	return c
}

// Validate reports every unmet install prerequisite; the review
// screen gates its Continue action on it.
func (w *Wizard) Validate() error { return w.cfg.validate() }

// Install runs the helm chart install and then watches the release's
// Deployments until ready
func (w *Wizard) Install(stdout io.Writer) error {
	fmt.Fprintf(stdout, "\n\n\nApplying...%s\n", lo.Ternary(w.cfg.DryRun, "(dry run mode)", ""))
	cfg := w.cfg

	// The secret-write log names creating vs updating; the dry run
	// prints the apply commands instead and skips the discovery call.
	var existingSecrets []string
	if !cfg.DryRun {
		var err error
		existingSecrets, err = w.k.ListSecrets(w.ctx, cfg.KubeContext, cfg.Namespace)
		if err != nil {
			return fmt.Errorf("listing existing secrets: %w", err)
		}
	}
	if err := w.resolveCredentials(stdout, existingSecrets, &cfg.AgentCreds, w.secretName("agent")); err != nil {
		return fmt.Errorf("resolving agent credentials: %w", err)
	}
	if err := w.resolveCredentials(stdout, existingSecrets, &cfg.PoolingCreds, w.secretName("pooling")); err != nil {
		return fmt.Errorf("resolving pooling credentials: %w", err)
	}

	// The API key always rides its own recreated Secret — the raw value
	// never travels in the values file.
	apiSecretName := cfg.ReleaseName + "-api-key"
	apiData := map[string]string{"API_KEY": cfg.APISecret}
	if err := w.writeSecret(stdout, existingSecrets, apiSecretName, apiData); err != nil {
		return fmt.Errorf("creating api secret: %w", err)
	}

	values := newHelmValues(cfg)
	if values.DBAgent.Enabled {
		values.DBAgent.APIKeySecretRef = apiSecretName
	}
	if values.DBProxy.Enabled {
		values.DBProxy.APIKeySecretRef = apiSecretName
	}

	if err := Install(w.ctx, values, cfg, w.shell, stdout); err != nil {
		return fmt.Errorf("helm install: %w", err)
	}
	if cfg.DryRun {
		return nil
	}

	return MonitorDeployments(w.ctx, w.k, w.shell, cfg, stdout, os.Stderr)
}

// writeSecret creates or updates the named Secret with data, logging
// the write to the install's transcript; the kubectl layer prints
// the apply command on dry runs.
func (w *Wizard) writeSecret(stdout io.Writer, existing []string, name string, data map[string]string) error {
	logSecretWrite(stdout, existing, name)
	if err := w.k.EnsureSecret(w.ctx, w.cfg.KubeContext, w.cfg.Namespace, name, data, stdout, w.cfg.DryRun); err != nil {
		return fmt.Errorf("ensure secret %s: %w", name, err)
	}

	return nil
}

// resolveCredentials replaces a username/password pair with the ref of
// the Secret created for it; an existing Secret ref or empty
// credentials pass through.
func (w *Wizard) resolveCredentials(stdout io.Writer, existing []string, c *api.Credentials, secretName string) error {
	if c.SecretName != "" || (c.Username == "" && c.Password == "") {
		return nil
	}
	data := map[string]string{
		"DATABASE_USERNAME": c.Username,
		"DATABASE_PASSWORD": c.Password,
	}
	if err := w.writeSecret(stdout, existing, secretName, data); err != nil {
		return err
	}
	c.SecretName = secretName
	c.Username, c.Password = "", ""

	return nil
}

// logSecretWrite announces a Secret write to the install's
// transcript; the existing-Secret check names creating vs updating.
func logSecretWrite(stdout io.Writer, existing []string, name string) {
	action := "creating"
	if slices.Contains(existing, name) {
		action = "updating"
	}
	fmt.Fprintln(stdout, action+" secret "+name)
}

// secretName is the deterministic name of the wizard-created Secret
// holding a component's credentials.
func (w *Wizard) secretName(component string) string {
	return w.cfg.ReleaseName + "-" + component + "-credentials"
}

// Shutdown releases anything the wizard holds; the cmd entry calls it
// after the UI returns. The v1 wizard holds nothing.
func (w *Wizard) Shutdown() {}
