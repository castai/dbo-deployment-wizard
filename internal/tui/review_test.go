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
func runLoad(t testing.TB, review *reviewModel) (reviewTitleMsg, spinner.TickMsg) {
	t.Helper()

	r := require.New(t)

	batch, ok := review.load()().(tea.BatchMsg)
	r.True(ok)

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
	r := require.New(t)

	b := &stubBackend{}
	review := newTestReviewModel(b)

	r.Contains(review.view(), reviewTitleLoading)

	cmd := review.load()
	r.Contains(review.view(), reviewTitleLoading)
	r.Zero(b.lookups)
	r.NotNil(cmd)

	msg, _ := runLoad(t, review)
	review.setTitle(msg)

	r.Contains(review.view(), "Installing new Helm deployment: castai-dbo")
	r.Equal(1, b.lookups)
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
			r := require.New(t)
			review := newTestReviewModel(&stubBackend{exists: tt.exists, err: tt.err})

			msg, _ := runLoad(t, review)
			review.setTitle(msg)

			r.Equal(tt.want, review.title)
		})
	}
}

func TestReviewSetTitle_DropsSupersededLookups(t *testing.T) {
	r := require.New(t)

	b := &stubBackend{}
	review := newTestReviewModel(b)

	stale, _ := runLoad(t, review)

	b.exists = true
	fresh, _ := runLoad(t, review)

	review.setTitle(fresh)
	review.setTitle(stale)

	r.Equal("Updating Helm deployment: castai-dbo", review.title)
}

func TestReviewTick_SpinsOnlyWhileLoading(t *testing.T) {
	r := require.New(t)
	review := newTestReviewModel(&stubBackend{})

	msg, tick := runLoad(t, review)
	frame := review.view()
	r.NotNil(review.tick(tick))
	r.NotEqual(frame, review.view())

	review.setTitle(msg)
	r.Nil(review.tick(tick))
}
