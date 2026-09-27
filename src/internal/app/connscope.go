package app

import (
	"reflect"
	"strings"
	"time"

	"charm.land/bubbletea/v2"
	"github.com/gophercloud/gophercloud/v2"
	"github.com/gophercloud/gophercloud/v2/openstack/identity/v3/tokens"
	"github.com/larkly/lazystack/internal/shared"
	"github.com/larkly/lazystack/internal/ui/cloneprogress"
	"github.com/larkly/lazystack/internal/ui/imagecreate"
	"github.com/larkly/lazystack/internal/ui/imagedownload"
)

// Connection scoping
//
// View models receive command results by message type, with no notion of
// which cloud/project connection issued the request. After a cloud or
// project switch, a slow response from the previous connection would
// otherwise land in the fresh views (foreign rows, errors, action results).
//
// Every command returned by Model.Update is therefore wrapped by scopeCmd:
// its result is delivered as a connScopedMsg carrying the connection
// generation that was current when the command was issued. Update unwraps
// the envelope and drops it if a newer connection has been established.

// connScopedMsg carries a command result together with the connection
// generation that issued the command.
type connScopedMsg struct {
	gen uint64
	msg tea.Msg
}

// connectResultMsg carries the outcome of a connect attempt. Only the most
// recent attempt is applied, and identity (cloud name, project) is
// committed only when that attempt succeeds.
type connectResultMsg struct {
	seq         uint64
	cloudName   string
	projectID   string // requested project, empty for the cloud default
	projectName string
	msg         tea.Msg // shared.CloudConnectedMsg or shared.CloudConnectErrMsg
}

// projectsLoadErrMsg reports that the accessible-project list could not be
// fetched. It is not fatal: the connection and its token scope stay valid.
type projectsLoadErrMsg struct {
	err error
}

// refreshTickMsg drives the periodic refresh chain. Only the chain with the
// current tick generation is live; ticks from a chain superseded by a
// reconnect or an idle resume are ignored instead of rescheduled.
type refreshTickMsg struct {
	gen uint64
}

// refreshTickCmd returns a single tea.Tick that fires a refresh tick after
// the refresh interval. This is the ONLY tick source in the app — views
// must not create their own tick timers.
func (m Model) refreshTickCmd() tea.Cmd {
	gen := m.tickGen
	return tea.Tick(m.refreshInterval, func(time.Time) tea.Msg {
		return refreshTickMsg{gen: gen}
	})
}

// scopeCmd stamps the result of cmd with the connection generation gen.
func scopeCmd(gen uint64, cmd tea.Cmd) tea.Cmd {
	if cmd == nil {
		return nil
	}
	return func() tea.Msg {
		msg := cmd()
		if batch, ok := msg.(tea.BatchMsg); ok {
			scoped := make(tea.BatchMsg, len(batch))
			for i, c := range batch {
				scoped[i] = scopeCmd(gen, c)
			}
			return scoped
		}
		if !connScoped(msg) {
			return msg
		}
		return connScopedMsg{gen: gen, msg: msg}
	}
}

var (
	teaPkgPrefix      = "charm.land/"
	imageCreatePkg    = reflect.TypeOf(imagecreate.Model{}).PkgPath()
	imageDownloadPkg  = reflect.TypeOf(imagedownload.Model{}).PkgPath()
	connectResultType = reflect.TypeOf(connectResultMsg{})
)

// connScoped reports whether msg belongs to the connection that issued it.
// Framework messages (quit, batches, spinner frames), connect results, and
// long-running operations that own their clients and identity (volume
// clones, image uploads/downloads) are delivered regardless of connection.
func connScoped(msg tea.Msg) bool {
	if msg == nil {
		return false
	}
	if _, ok := msg.(connScopedMsg); ok {
		return false // already scoped by an inner Update
	}
	if _, ok := cloneprogress.OpID(msg); ok {
		return false
	}
	t := reflect.TypeOf(msg)
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t == connectResultType {
		return false
	}
	pkg := t.PkgPath()
	switch {
	case strings.HasPrefix(pkg, teaPkgPrefix):
		return false
	case pkg == imageCreatePkg, pkg == imageDownloadPkg:
		return false
	}
	return true
}

// tokenProject returns the project the provider's auth token is scoped
// to, if the auth result carries one. It does no I/O.
func tokenProject(pc *gophercloud.ProviderClient) (id, name string) {
	if pc == nil {
		return "", ""
	}
	ar, ok := pc.GetAuthResult().(interface {
		ExtractProject() (*tokens.Project, error)
	})
	if !ok {
		return "", ""
	}
	proj, err := ar.ExtractProject()
	if err != nil || proj == nil {
		return "", ""
	}
	return proj.ID, proj.Name
}

// connectCmd connects to cloudName (optionally scoped to projectID) and
// reports the outcome as a connectResultMsg for attempt seq.
func connectCmd(seq uint64, cloudName, projectID, projectName string, connect func() tea.Msg) tea.Cmd {
	return func() tea.Msg {
		return connectResultMsg{
			seq:         seq,
			cloudName:   cloudName,
			projectID:   projectID,
			projectName: projectName,
			msg:         connect(),
		}
	}
}

// applyConnectResult commits the most recent connect attempt. Stale
// attempts are dropped; a failed attempt keeps the previous connection and
// its identity.
func (m Model) applyConnectResult(res connectResultMsg) (tea.Model, tea.Cmd) {
	if res.seq != m.connectSeq {
		shared.Debugf("[app] dropping stale connect result (attempt %d, current %d)", res.seq, m.connectSeq)
		return m, nil
	}
	if _, ok := res.msg.(shared.CloudConnectedMsg); !ok {
		return m.update(res.msg)
	}
	m.cloudName = res.cloudName
	next, cmd := m.update(res.msg)
	nm := next.(Model)
	if res.projectID != "" {
		if nm.currentProjectID == "" {
			nm.currentProjectID = res.projectID
		}
		if nm.currentProjectID == res.projectID && nm.statusBar.ProjectName == "" {
			nm.statusBar.ProjectName = res.projectName
		}
		nm.quotaView.SetProjectID(nm.currentProjectID)
	}
	return nm, cmd
}
