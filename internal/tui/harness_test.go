package tui

import (
	"io"
	"reflect"
	"testing"
	"time"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"github.com/samber/lo"
	"github.com/stretchr/testify/require"

	"github.com/castai/dbo-deployment-wizard/internal/api"
)

// fakeBackend is a hand-written api.Backend: queries return the fields,
// commands record into them.
type fakeBackend struct {
	kubeContext  string
	namespace    string
	chartVersion string
	agentCreds   api.Credentials
	poolingCreds api.Credentials
	components   []string

	kubeContexts    []string
	kubeContextsErr error
	versions        []api.HelmChartVersion
	versionsErr     error
	secrets         []string
	secretsErr      error
	exists          bool
	existsErr       error
	summary         []string
	summaryErr      error

	setComponentsErr error
	validateErr      error
	installed        bool
}

var _ api.Backend = (*fakeBackend)(nil)

func newFakeBackend() *fakeBackend {
	return &fakeBackend{
		kubeContext:  "kind-a",
		namespace:    "castai-dbo",
		chartVersion: "1.0.0",
		components:   []string{api.ComponentDBAgent, api.ComponentDBProxy, api.ComponentPooling},
		kubeContexts: []string{"kind-a", "kind-b", "prod"},
		versions:     []api.HelmChartVersion{{Number: "1.1.0"}, {Number: "1.0.0"}},
		summary:      []string{"install castai-dbo"},
	}
}

func (f *fakeBackend) KubeContext() string                 { return f.kubeContext }
func (f *fakeBackend) Namespace() string                   { return f.namespace }
func (f *fakeBackend) ChartVersion() string                { return f.chartVersion }
func (f *fakeBackend) ReleaseName() string                 { return "castai-dbo" }
func (f *fakeBackend) AgentCredentials() api.Credentials   { return f.agentCreds }
func (f *fakeBackend) PoolingCredentials() api.Credentials { return f.poolingCreds }
func (f *fakeBackend) Components() []string                { return f.components }
func (f *fakeBackend) KubeContexts() ([]string, error)     { return f.kubeContexts, f.kubeContextsErr }

func (f *fakeBackend) ChartVersions() ([]api.HelmChartVersion, error) {
	return f.versions, f.versionsErr
}

func (f *fakeBackend) LatestChartVersion() string { return f.versions[0].Number }

func (f *fakeBackend) Secrets() ([]string, error)              { return f.secrets, f.secretsErr }
func (f *fakeBackend) DeploymentExists() (bool, error)         { return f.exists, f.existsErr }
func (f *fakeBackend) SetKubeContext(name string)              { f.kubeContext = name }
func (f *fakeBackend) SetNamespace(ns string)                  { f.namespace = ns }
func (f *fakeBackend) SetChartVersion(version string)          { f.chartVersion = version }
func (f *fakeBackend) SetAgentCredentials(c api.Credentials)   { f.agentCreds = c }
func (f *fakeBackend) SetPoolingCredentials(c api.Credentials) { f.poolingCreds = c }
func (f *fakeBackend) Validate() error                         { return f.validateErr }

func (f *fakeBackend) SummarizeInstallationImpact() ([]string, error) {
	return f.summary, f.summaryErr
}

func (f *fakeBackend) SetComponents(components []string) error {
	if f.setComponentsErr != nil {
		return f.setComponentsErr
	}
	f.components = components

	return nil
}

func (f *fakeBackend) Install(io.Writer) error {
	f.installed = true

	return nil
}

// cmdTimeout bounds how long a command may take to answer; timers
// (spinner ticks, the repaint loop) never answer in time and are
// dropped, which keeps the harness from looping on them.
const cmdTimeout = 30 * time.Millisecond

// harness drives the root model the way a tea.Program would, without
// a terminal.
type harness struct {
	tb   testing.TB
	m    *Model
	quit bool
}

func newHarness(tb testing.TB, b *fakeBackend) *harness {
	tb.Helper()
	h := &harness{tb: tb, m: newModelWithBackground(b, true)}
	h.run(h.m.Init())

	return h
}

func (h *harness) send(msgs ...tea.Msg) {
	h.tb.Helper()
	for _, msg := range msgs {
		_, cmd := h.m.Update(msg)
		h.run(cmd)
	}
}

func (h *harness) press(code rune) {
	h.tb.Helper()
	h.send(tea.KeyPressMsg{Code: code})
}

func (h *harness) ctrl(code rune) {
	h.tb.Helper()
	h.send(tea.KeyPressMsg{Code: code, Mod: tea.ModCtrl})
}

func (h *harness) typeText(s string) {
	h.tb.Helper()
	for _, c := range s {
		h.send(tea.KeyPressMsg{Code: c, Text: string(c)})
	}
}

// enterSection moves the review cursor onto the row targeting to (or
// Continue, for screenConfirm) and presses Enter.
func (h *harness) enterSection(to screenID) {
	h.tb.Helper()
	r := require.New(h.tb)
	r.Same(h.m.review, h.m.active, "not on review")

	_, i, found := lo.FindIndexOf(h.m.review.list.Items(), func(it list.Item) bool {
		ri, ok := it.(reviewItem)

		return ok && lo.Ternary(to == screenConfirm, ri.action, !ri.action && ri.target == to)
	})
	r.True(found, "no review row for screen %d", to)
	h.m.review.list.Select(i)
	h.press(tea.KeyEnter)
}

func (h *harness) onReview() bool {
	return h.m.active == screen(h.m.review)
}

// run executes cmd and feeds what it produces back into the model
// until nothing answers within cmdTimeout.
func (h *harness) run(cmd tea.Cmd) {
	h.tb.Helper()
	pending := []tea.Cmd{cmd}
	for range 100 {
		pending = lo.Filter(pending, func(c tea.Cmd, _ int) bool { return c != nil })
		if len(pending) == 0 {
			return
		}
		msgs := collect(pending)
		pending = nil
		for _, msg := range msgs {
			if sub, ok := cmdList(msg); ok {
				pending = append(pending, sub...)

				continue
			}
			if _, ok := msg.(tea.QuitMsg); ok {
				h.quit = true

				continue
			}
			_, next := h.m.Update(msg)
			pending = append(pending, next)
		}
	}
	h.tb.Fatal("commands never settled")
}

// collect runs cmds concurrently and returns, in order, the messages of
// those that answer within cmdTimeout.
func collect(cmds []tea.Cmd) []tea.Msg {
	results := lo.Map(cmds, func(c tea.Cmd, _ int) chan tea.Msg {
		ch := make(chan tea.Msg, 1)
		go func() { ch <- c() }()

		return ch
	})

	timer := time.NewTimer(cmdTimeout)
	defer timer.Stop()
	expired := false

	return lo.FilterMap(results, func(ch chan tea.Msg, _ int) (tea.Msg, bool) {
		if !expired {
			select {
			case msg := <-ch:
				return msg, msg != nil
			case <-timer.C:
				expired = true
			}
		}
		select {
		case msg := <-ch:
			return msg, msg != nil
		default:
			return nil, false
		}
	})
}

// cmdList unpacks tea.Batch and tea.Sequence messages; the latter's
// type is unexported, hence the reflection.
func cmdList(msg tea.Msg) ([]tea.Cmd, bool) {
	v := reflect.ValueOf(msg)
	if v.Kind() != reflect.Slice || v.Type().Elem() != reflect.TypeFor[tea.Cmd]() {
		return nil, false
	}

	return lo.Times(v.Len(), func(i int) tea.Cmd {
		cmd, _ := v.Index(i).Interface().(tea.Cmd)

		return cmd
	}), true
}
