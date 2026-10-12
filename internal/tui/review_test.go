package tui

import (
	"errors"
	"testing"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"
	"github.com/stretchr/testify/require"

	"github.com/castai/dbo-deployment-wizard/internal/api"
)

// stubBackend answers the review screen's queries and counts the
// release lookups; any other Backend call panics on the nil embed.
type stubBackend struct {
	api.Backend

	exists  bool
	err     error
	lookups int
}

func (s *stubBackend) KubeContext() string                 { return "kind-test" }
func (s *stubBackend) Namespace() string                   { return "castai-db-optimizer" }
func (s *stubBackend) ChartVersion() string                { return "1.4.2" }
func (s *stubBackend) LatestChartVersion() string          { return "1.4.2" }
func (s *stubBackend) ReleaseName() string                 { return "castai-dbo" }
func (s *stubBackend) Components() []string                { return []string{api.ComponentDBAgent} }
func (s *stubBackend) AgentCredentials() api.Credentials   { return api.Credentials{} }
func (s *stubBackend) PoolingCredentials() api.Credentials { return api.Credentials{} }

func (s *stubBackend) SummarizeInstallationImpact() ([]string, error) {
	return []string{"New Helm deployment will be created"}, nil
}

func (s *stubBackend) DeploymentExists() (bool, error) {
	s.lookups++

	return s.exists, s.err
}

func newTestReviewModel(b api.Backend) *reviewModel {
	return newReviewModel(b, huh.ThemeFunc(huh.ThemeCharm).Theme(true), true)
}

// runLoad starts a lookup and runs it to its answer, returning the
// spinner's first tick alongside.
func runLoad(t testing.TB, r *reviewModel) (reviewTitleMsg, spinner.TickMsg) {
	t.Helper()

	batch, ok := r.load()().(tea.BatchMsg)
	require.True(t, ok)

	var title reviewTitleMsg
	var tick spinner.TickMsg
	for _, cmd := range batch {
		switch msg := cmd().(type) {
		case reviewTitleMsg:
			title = msg
		case spinner.TickMsg:
			tick = msg
		}
	}

	return title, tick
}

// The release lookup shells out to helm, which blocks for as long as
// an unreachable cluster keeps it waiting; rendering must never run it.
func TestReviewView_DoesNotLookUpTheRelease(t *testing.T) {
	b := &stubBackend{}
	r := newTestReviewModel(b)

	require.Contains(t, r.view(), reviewTitleLoading)

	cmd := r.load()
	require.Contains(t, r.view(), reviewTitleLoading)
	require.Zero(t, b.lookups)
	require.NotNil(t, cmd)

	msg, _ := runLoad(t, r)
	r.setTitle(msg)

	require.Contains(t, r.view(), "Installing new Helm deployment: castai-dbo")
	require.Equal(t, 1, b.lookups)
}

func TestReviewLoad_Title(t *testing.T) {
	tests := map[string]struct {
		exists bool
		err    error
		want   string
	}{
		"new installation":    {want: "Installing new Helm deployment: castai-dbo"},
		"existing release":    {exists: true, want: "Updating Helm deployment: castai-dbo"},
		"unreachable cluster": {err: errors.New("kubernetes cluster unreachable"), want: "kubernetes cluster unreachable"},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			r := newTestReviewModel(&stubBackend{exists: tt.exists, err: tt.err})

			msg, _ := runLoad(t, r)
			r.setTitle(msg)

			require.Equal(t, tt.want, r.title)
		})
	}
}

func TestReviewSetTitle_DropsSupersededLookups(t *testing.T) {
	b := &stubBackend{}
	r := newTestReviewModel(b)

	stale, _ := runLoad(t, r)

	b.exists = true
	fresh, _ := runLoad(t, r)

	r.setTitle(fresh)
	r.setTitle(stale)

	require.Equal(t, "Updating Helm deployment: castai-dbo", r.title)
}

func TestReviewTick_SpinsOnlyWhileLoading(t *testing.T) {
	r := newTestReviewModel(&stubBackend{})

	msg, tick := runLoad(t, r)
	frame := r.view()
	require.NotNil(t, r.tick(tick))
	require.NotEqual(t, frame, r.view())

	r.setTitle(msg)
	require.Nil(t, r.tick(tick))
}
