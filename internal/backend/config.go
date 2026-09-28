package backend

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/samber/lo"

	"github.com/castai/dbo-deployment-wizard/internal/api"
)

// Config is every user-supplied deployment parameter, from the CLI's
// flags to the TUI's commits; the helm install generator is one of
// its consumers.
type Config struct {
	ReleaseName  string
	ChartName    string
	Namespace    string
	KubeContext  string
	ChartVersion string

	// Chart sourcing config, not a helm value.
	ChartRepo string

	// AgentCreds are the db-agent's database credentials; PoolingCreds
	// are db-proxy's (pooling). Each either references an existing
	// Secret or carries a username/password pair, which the install
	// turns into a Secret and passes its ref to helm instead.
	AgentCreds   api.Credentials
	PoolingCreds api.Credentials

	Components []string

	APIURL    string
	APISecret string

	// DryRun prints the exact helm command instead of executing it.
	DryRun bool

	// SimulateSlowNetwork delays every networked call by a fixed 2s —
	// a local UI-testing aid, not a deployment parameter.
	SimulateSlowNetwork bool
}

// Static defaults; the discovery-based fields (KubeContext,
// ChartVersion) are filled by NewWizard.
const (
	defaultAPIBaseURL  = "https://api.cast.ai"
	defaultChartName   = "castai-dbo"
	defaultReleaseName = "castai-dbo"
	defaultNamespace   = "castai-db-optimizer"
)

// NewDefaultConfig returns the initial configuration with every
// static field prefilled.
func NewDefaultConfig() Config {
	return Config{
		APIURL:      defaultAPIBaseURL,
		ChartRepo:   DefaultIndexURL,
		ChartName:   defaultChartName,
		ReleaseName: defaultReleaseName,
		Namespace:   defaultNamespace,

		Components: []string{api.ComponentDBAgent, api.ComponentDBProxy},
	}
}

// HasComponent reports whether the given subchart is enabled.
func (c *Config) HasComponent(name string) bool {
	for _, comp := range c.Components {
		if comp == name {
			return true
		}
	}

	return false
}

// SetComponents replaces the enabled component set. Names must be in
// AllComponents (unknown names error), duplicates drop, and pooling
// requires db-proxy. Dropping pooling clears its credentials.
func (c *Config) SetComponents(components []string) error {
	components = lo.Uniq(components)
	for _, c := range components {
		if !slices.Contains(api.AllComponents, c) {
			return fmt.Errorf("unknown component %q (allowed: %s)",
				c, strings.Join(api.AllComponents, ", "))
		}
	}

	if slices.Contains(components, api.ComponentPooling) && !slices.Contains(components, api.ComponentDBProxy) {
		return fmt.Errorf("%s requires %s to also be selected", api.ComponentPooling, api.ComponentDBProxy)
	}

	c.Components = components
	c.reconcilePoolingCreds()

	return nil
}

// reconcilePoolingCreds clears the pooling credentials whenever
// pooling is no longer enabled.
func (c *Config) reconcilePoolingCreds() {
	if !c.HasComponent(api.ComponentPooling) {
		c.PoolingCreds = api.Credentials{}
	}
}

// SetComponentEnabled toggles a single component; validity is
// enforced by SetComponents.
func (c *Config) SetComponentEnabled(name string, enabled bool) error {
	sel := slices.Clone(c.Components)
	if enabled {
		return c.SetComponents(append(sel, name))
	}

	sel = slices.DeleteFunc(sel, func(x string) bool { return x == name })

	return c.SetComponents(sel)
}

// validate reports the unmet install prerequisites as one joined
// error: the enabled components' credentials and the target
// namespace. Kube context and chart version can't be emptied
// through the contract (NewWizard fills them, the screens only
// select), so they are not rechecked.
func (c *Config) validate() error {
	var errs []string
	if c.Namespace == "" {
		errs = append(errs, "namespace is required")
	}
	if c.APISecret == "" {
		return errors.New("api secret is required")
	}
	if c.ReleaseName == "" {
		return errors.New("release name is required")
	}
	if c.ChartName == "" {
		return errors.New("chart name is required")
	}

	if c.HasComponent(api.ComponentDBAgent) && !c.AgentCreds.Provided() {
		errs = append(errs, "agent credentials are required")
	}
	if c.HasComponent(api.ComponentPooling) && !c.PoolingCreds.Provided() {
		errs = append(errs, "pooling credentials are required")
	}
	if len(errs) == 0 {
		return nil
	}

	return errors.New(strings.Join(errs, "; "))
}
