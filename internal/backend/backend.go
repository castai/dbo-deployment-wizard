// Package backend is the deployment wizard's service layer: the single
// owner of the deployment configuration and the domain rules around
// it. It renders nothing and reads no keys.
package backend

import (
	"context"
	"fmt"
	"slices"

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
	// helm executes the helm install; tests inject a mock.
	helm HelmRunner

	cfg Config

	latestVersion string
}

func (w *Wizard) ReleaseName() string {
	return w.cfg.ReleaseName
}

// Compile-time check: Wizard implements the full Backend contract.
var _ api.Backend = (*Wizard)(nil)

// NewWizard implements api.Backend
func NewWizard(ctx context.Context, cfg Config, repo *HelmRepoClient, k Kubectl, helm HelmRunner) (*Wizard, error) {
	w := &Wizard{
		ctx:  ctx,
		repo: repo,
		k:    k,
		helm: helm,
		cfg:  cfg,
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

// Install runs the helm chart install; DryRun prints the values
// and the exact command instead of executing. Credentials given as a
// username/password pair are turned into Secrets in the target
// namespace, and the API key always rides its own recreated Secret —
// their refs, never the raw values, reach helm.
func (w *Wizard) Install() error {
	cfg := w.cfg
	if err := w.resolveCredentials(&cfg.AgentCreds, w.secretName("agent")); err != nil {
		return err
	}
	if err := w.resolveCredentials(&cfg.PoolingCreds, w.secretName("pooling")); err != nil {
		return err
	}

	apiSecretName := cfg.ReleaseName + "-api-key"
	if err := w.createAPISecret(apiSecretName); err != nil {
		return err
	}

	values := newHelmValues(cfg)
	if values.DBAgent.Enabled {
		values.DBAgent.APIKeySecretRef = apiSecretName
	}
	if values.DBProxy.Enabled {
		values.DBProxy.APIKeySecretRef = apiSecretName
	}

	return Install(w.ctx, values, cfg, w.helm)
}

// createAPISecret recreates the Secret carrying the CAST AI API key on
// every install — there is no existing-secret path for it — so the key
// never travels raw in the values file. DryRun skips the cluster call;
// the values still reference the Secret by name.
func (w *Wizard) createAPISecret(secretName string) error {
	if w.cfg.DryRun {
		return nil
	}
	data := map[string]string{"API_KEY": w.cfg.APISecret}
	if err := w.k.EnsureSecret(w.ctx, w.cfg.KubeContext, w.cfg.Namespace, secretName, data); err != nil {
		return fmt.Errorf("ensure api key secret: %w", err)
	}

	return nil
}

// resolveCredentials replaces a username/password pair with the ref of
// the Secret created for it; an existing Secret ref or empty
// credentials pass through. DryRun skips the cluster call and only
// resolves the name the Secret would get.
func (w *Wizard) resolveCredentials(c *api.Credentials, secretName string) error {
	if c.SecretName != "" || (c.Username == "" && c.Password == "") {
		return nil
	}
	if !w.cfg.DryRun {
		data := map[string]string{
			"DATABASE_USERNAME": c.Username,
			"DATABASE_PASSWORD": c.Password,
		}
		if err := w.k.EnsureSecret(w.ctx, w.cfg.KubeContext, w.cfg.Namespace, secretName, data); err != nil {
			return fmt.Errorf("ensure credentials secret %s: %w", secretName, err)
		}
	}
	c.SecretName = secretName
	c.Username, c.Password = "", ""

	return nil
}

// secretName is the deterministic name of the wizard-created Secret
// holding a component's credentials.
func (w *Wizard) secretName(component string) string {
	return w.cfg.ReleaseName + "-" + component + "-credentials"
}

// Shutdown releases anything the wizard holds; the cmd entry calls it
// after the UI returns. The v1 wizard holds nothing.
func (w *Wizard) Shutdown() {}
