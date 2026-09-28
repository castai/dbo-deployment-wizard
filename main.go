// Package main implements the DBO deployment-wizard CLI: the cobra
// flag surface over backend.NewDefaultConfig, then NewWizard → tui.Run.
// The TUI requires a real terminal; running from a pipe errors out.
package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/mattn/go-isatty"
	"github.com/samber/lo"
	"github.com/spf13/cobra"

	"github.com/castai/dbo-deployment-wizard/internal/backend"
	"github.com/castai/dbo-deployment-wizard/internal/tui"
)

// errNotATTY is returned when the wizard is run without a terminal —
// the bubbletea TUI needs raw terminal input.
var errNotATTY = errors.New("this wizard requires a TTY; run it from a terminal or use a non-interactive installer instead")

// newRootCmd defines the CLI surface: every flag binds directly into
// the default config; RunE normalizes --components through
// SetComponents, and NewWizard validates the result.
func newRootCmd() *cobra.Command {
	cfg := backend.NewDefaultConfig()

	cmd := &cobra.Command{
		Use:   "deployment-wizard",
		Short: "Interactive installer for the castai-dbo Helm chart",
		Long: `deployment-wizard interactively collects every parameter required to
install the castai-dbo Helm chart (kubectl context, namespace, chart
version, secrets mode, components, pooling credentials), shows
a summary, and runs 'helm upgrade --install'.

Pass --dry-run to print the exact helm command instead of executing it.

The wizard requires a real terminal; it cannot be run from a pipe or CI.`,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			components := lo.FilterMap(cfg.Components, func(s string, _ int) (string, bool) {
				trimmed := strings.TrimSpace(s)

				return trimmed, trimmed != ""
			})
			if err := cfg.SetComponents(components); err != nil {
				return fmt.Errorf("--components: %w", err)
			}

			return run(cmd.Context(), cfg)
		},
	}

	cmd.Flags().StringVar(&cfg.APIURL, "api-url", cfg.APIURL,
		"CAST AI API base URL. Reserved for future use; not contacted in v1.")
	cmd.Flags().StringVar(&cfg.APISecret, "api-secret", "",
		"CAST AI API secret. Required.")
	_ = cmd.MarkFlagRequired("api-secret")
	cmd.Flags().StringVar(&cfg.ChartRepo, "chart-repo", cfg.ChartRepo,
		"URL of the Helm chart repository index.yaml used to list chart versions.")
	cmd.Flags().StringVar(&cfg.ChartName, "chart-name", cfg.ChartName,
		"Name of the Helm chart to install.")
	cmd.Flags().StringVar(&cfg.ChartVersion, "chart-version", cfg.ChartVersion,
		"Helm chart version to install.")
	cmd.Flags().StringVar(&cfg.ReleaseName, "release-name", cfg.ReleaseName,
		"Helm release name to use for the install.")
	cmd.Flags().StringVar(&cfg.KubeContext, "kubecontext", "",
		"Pre-fills the kubectl-context prompt. The wizard always confirms or overrides.")
	cmd.Flags().StringVar(&cfg.Namespace, "namespace", cfg.Namespace,
		"Pre-fills the namespace prompt. The wizard always confirms or overrides.")
	cmd.Flags().StringSliceVar(&cfg.Components, "components", cfg.Components,
		"Comma-separated list of chart components to install (e.g. \"db-agent,db-proxy,pooling\"). "+
			"Allowed values: db-agent, db-proxy, pooling. "+
			"pooling requires db-proxy to also be selected. "+
			"When unset, db-agent and db-proxy are preselected and the components screen is fully interactive.")
	cmd.Flags().BoolVar(&cfg.DryRun, "dry-run", false,
		"Print the exact 'helm upgrade --install' command instead of executing it.")
	cmd.Flags().BoolVar(&cfg.SimulateSlowNetwork, "slow-network", false,
		"Add a 2s delay to every networked call (kubectl and the chart repository) to simulate a slow network, for testing the UI.")

	return cmd
}

func main() {
	ctx := context.Background()
	if err := newRootCmd().ExecuteContext(ctx); err != nil {
		if errors.Is(err, tui.ErrAborted) {
			fmt.Fprintln(os.Stderr, "install aborted")
			os.Exit(0)
		}
		if errors.Is(err, errNotATTY) {
			fmt.Fprintln(os.Stderr, "Error:", err)
			os.Exit(2)
		}
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, cfg backend.Config) error {
	if !isatty.IsTerminal(os.Stdout.Fd()) && !isatty.IsCygwinTerminal(os.Stdout.Fd()) {
		return errNotATTY
	}

	repo := backend.NewHelmRepoClient(&http.Client{}, cfg.ChartRepo, cfg.SimulateSlowNetwork)
	k := backend.RealKubectl{SimulateSlowNetwork: cfg.SimulateSlowNetwork}
	helm := backend.DefaultHelmRunner{}

	w, err := backend.NewWizard(ctx, cfg, repo, &k, &helm)
	if err != nil {
		return err
	}
	defer w.Shutdown()

	return tui.Run(w)
}
