package servercreate

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"github.com/larkly/lazystack/internal/compute"
	img "github.com/larkly/lazystack/internal/image"
	"github.com/larkly/lazystack/internal/network"
	"github.com/larkly/lazystack/internal/testutil"
)

// createRecorder is a fake Nova endpoint recording POST /servers bodies.
type createRecorder struct {
	mu     sync.Mutex
	bodies []map[string]any
	failAt int // 1-based request number to fail; 0 = never
}

func (r *createRecorder) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost || req.URL.Path != "/servers" {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	raw, _ := io.ReadAll(req.Body)
	var body struct {
		Server map[string]any `json:"server"`
	}
	_ = json.Unmarshal(raw, &body)
	r.mu.Lock()
	r.bodies = append(r.bodies, body.Server)
	n := len(r.bodies)
	failAt := r.failAt
	r.mu.Unlock()
	if n == failAt {
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte(`{"forbidden":{"message":"Quota exceeded","code":403}}`))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	fmt.Fprintf(w, `{"server":{"id":"srv-%d","adminPass":"x"}}`, n)
}

func (r *createRecorder) got() []map[string]any {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]map[string]any(nil), r.bodies...)
}

// run executes cmd (expanding batches) and returns every non-spinner message.
func run(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	msg := cmd()
	if batch, ok := msg.(tea.BatchMsg); ok {
		var out []tea.Msg
		for _, c := range batch {
			out = append(out, run(c)...)
		}
		return out
	}
	if _, ok := msg.(spinner.TickMsg); ok || msg == nil {
		return nil
	}
	return []tea.Msg{msg}
}

func loadedForm(t *testing.T, images []img.Image, networks []network.Network) (Model, *createRecorder) {
	t.Helper()
	rec := &createRecorder{}
	client, cleanup := testutil.FakeServiceClient(rec)
	t.Cleanup(cleanup)
	m := New(client, nil, nil)
	m, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 50})
	m, _ = m.Update(imagesLoadedMsg{images: images})
	m, _ = m.Update(flavorsLoadedMsg{flavors: []compute.Flavor{{ID: "flv-1", Name: "m1.small", VCPUs: 1, RAM: 2048, Disk: 20}}})
	m, _ = m.Update(networksLoadedMsg{networks: networks})
	m, _ = m.Update(keypairsLoadedMsg{})
	m, _ = m.Update(secGroupsLoadedMsg{})
	return m, rec
}

func defaultImages() []img.Image {
	return []img.Image{{ID: "11111111-2222-3333-4444-555555555555", Name: "ubuntu"}}
}

func readyForm(t *testing.T) (Model, *createRecorder) {
	t.Helper()
	m, rec := loadedForm(t, defaultImages(), nil)
	m.selectedImage = 0
	m.selectedFlavor = 0
	return m, rec
}

func names(bodies []map[string]any) []string {
	var out []string
	for _, b := range bodies {
		out = append(out, fmt.Sprint(b["name"]))
	}
	return out
}

func TestTemplateCountCreatesPreviewedNames(t *testing.T) {
	m, rec := readyForm(t)
	m.nameInput.SetValue("typed-name")
	m.templateInput.SetValue("web-{n:02d}")
	m.countInput.SetValue("3")
	if !strings.Contains(m.View(), "web-00, web-01, web-02") {
		t.Fatalf("preview missing:\n%s", m.View())
	}

	m, cmd := m.submit()
	if !m.submitting {
		t.Fatalf("not submitting, err=%q", m.err)
	}
	msgs := run(cmd)
	bodies := rec.got()
	if got := strings.Join(names(bodies), ","); got != "web-00,web-01,web-02" {
		t.Fatalf("created names=%s", got)
	}
	for _, b := range bodies {
		if _, ok := b["min_count"]; ok {
			t.Fatalf("templated create must not use multi-create: %v", b)
		}
		if _, ok := b["max_count"]; ok {
			t.Fatalf("templated create must not use multi-create: %v", b)
		}
	}
	if len(msgs) != 1 {
		t.Fatalf("msgs=%v", msgs)
	}
	if _, ok := msgs[0].(serverCreatedMsg); !ok {
		t.Fatalf("completion=%#v", msgs[0])
	}
}

func TestTemplateTakesPrecedenceOverTypedName(t *testing.T) {
	// A populated template names the server; the Server Name field is then optional.
	m, rec := readyForm(t)
	m.templateInput.SetValue("api-{n}")
	m, cmd := m.submit()
	if m.err != "" {
		t.Fatalf("template without typed name rejected: %q", m.err)
	}
	run(cmd)
	if got := names(rec.got()); len(got) != 1 || got[0] != "api-0" {
		t.Fatalf("names=%v", got)
	}
}

func TestPlainNameCountUsesSingleMultiCreate(t *testing.T) {
	m, rec := readyForm(t)
	m.nameInput.SetValue("db")
	m.countInput.SetValue("3")
	_, cmd := m.submit()
	run(cmd)
	bodies := rec.got()
	if len(bodies) != 1 || bodies[0]["name"] != "db" || bodies[0]["min_count"] != float64(3) || bodies[0]["max_count"] != float64(3) {
		t.Fatalf("bodies=%v", bodies)
	}
}

func TestValidationPrecedesRequests(t *testing.T) {
	cases := []struct {
		name  string
		setup func(Model) Model
		want  string
	}{
		{"no name or template", func(m Model) Model { return m }, "Server name is required"},
		{"bad count", func(m Model) Model {
			m.templateInput.SetValue("w-{n}")
			m.countInput.SetValue("0")
			return m
		}, "Count must be a positive number"},
		{"count too high", func(m Model) Model {
			m.templateInput.SetValue("w-{n}")
			m.countInput.SetValue("101")
			return m
		}, "100 or less"},
		{"no image", func(m Model) Model {
			m.nameInput.SetValue("x")
			m.selectedImage = -1
			return m
		}, "Image is required"},
		{"no flavor", func(m Model) Model {
			m.nameInput.SetValue("x")
			m.selectedFlavor = -1
			return m
		}, "Flavor is required"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m, rec := readyForm(t)
			m = tc.setup(m)
			m, cmd := m.submit()
			if cmd != nil || m.submitting || !strings.Contains(m.err, tc.want) {
				t.Fatalf("cmd=%v submitting=%v err=%q", cmd != nil, m.submitting, m.err)
			}
			if n := len(rec.got()); n != 0 {
				t.Fatalf("requests=%d", n)
			}
		})
	}
}

func TestPartialTemplatedCreateReportsCreatedIDs(t *testing.T) {
	m, rec := readyForm(t)
	rec.failAt = 3
	m.templateInput.SetValue("web-{n}")
	m.countInput.SetValue("4")
	m, cmd := m.submit()
	msgs := run(cmd)
	if n := len(rec.got()); n != 3 {
		t.Fatalf("requests=%d; creation should stop at the first failure", n)
	}
	m, _ = m.Update(msgs[0])
	for _, want := range []string{"web-0 (srv-1)", "web-1 (srv-2)", "web-2", "2 of 4"} {
		if !strings.Contains(m.err, want) {
			t.Fatalf("error %q missing %q", m.err, want)
		}
	}
	if m.submitting {
		t.Fatal("still submitting after partial failure")
	}
}

func TestSubmitIgnoredWhileSubmitting(t *testing.T) {
	m, rec := readyForm(t)
	m.nameInput.SetValue("x")
	m, first := m.submit()
	m, again := m.submit()
	if again != nil {
		t.Fatal("second submit while in flight produced a command")
	}
	run(first)
	if n := len(rec.got()); n != 1 {
		t.Fatalf("requests=%d", n)
	}
}

func TestShortIDsInPickers(t *testing.T) {
	ids := []string{"", "a", "abcdefg", "abcdefgh", "0b6c3f3e-7a0e-4d7c-9a8a-3c1d1f0c9e11"}
	var images []img.Image
	var nets []network.Network
	for i, id := range ids {
		images = append(images, img.Image{ID: id, Name: fmt.Sprintf("image-%d", i)})
		nets = append(nets, network.Network{ID: id, Name: fmt.Sprintf("net-%d", i)})
	}
	for _, field := range []int{fieldImage, fieldNetwork} {
		m, rec := loadedForm(t, images, nets)
		m.focusField = field
		m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
		if !m.pickerOpen {
			t.Fatal("picker did not open")
		}
		_ = m.View()
		for _, it := range m.pickerItems() {
			if len([]rune(it.desc)) > 8 {
				t.Fatalf("field %d: abbreviation %q exceeds 8 runes", field, it.desc)
			}
		}
		// Select the last (full UUID) item and verify the full ID is submitted.
		for i := 0; i < len(ids)-1; i++ {
			m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
		}
		m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
		m.nameInput.SetValue("x")
		if field == fieldNetwork {
			m.selectedImage = 0
		} else if m.selectedImage != len(ids)-1 {
			t.Fatalf("selectedImage=%d", m.selectedImage)
		}
		m.selectedFlavor = 0
		_, cmd := m.submit()
		run(cmd)
		body := rec.got()[0]
		if field == fieldImage && body["imageRef"] != ids[len(ids)-1] {
			t.Fatalf("imageRef=%v", body["imageRef"])
		}
		if field == fieldNetwork {
			nw, _ := body["networks"].([]any)
			if len(nw) != 1 || nw[0].(map[string]any)["uuid"] != ids[len(ids)-1] {
				t.Fatalf("networks=%v", body["networks"])
			}
		}
	}
}

func TestPickerFilterAcceptsJK(t *testing.T) {
	images := []img.Image{
		{ID: "11111111-aaaa", Name: "alpine"},
		{ID: "22222222-bbbb", Name: "jack-image"},
		{ID: "33333333-cccc", Name: "kube-node"},
		{ID: "44444444-dddd", Name: "kube-master"},
	}
	m, _ := loadedForm(t, images, nil)
	m.focusField = fieldImage
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	for _, r := range "jack" {
		m, _ = m.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	if got := m.pickerFilter.Value(); got != "jack" {
		t.Fatalf("filter=%q", got)
	}
	m.pickerFilter.SetValue("")
	for _, r := range "kube" {
		m, _ = m.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	if got := m.pickerFilter.Value(); got != "kube" {
		t.Fatalf("filter=%q", got)
	}
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	if m.pickerCursor != 1 {
		t.Fatalf("down arrow cursor=%d", m.pickerCursor)
	}
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.pickerOpen || m.selectedImage != 3 {
		t.Fatalf("enter selected %d (open=%v), want kube-master", m.selectedImage, m.pickerOpen)
	}

	m.focusField = fieldImage
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m, _ = m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if m.pickerOpen || m.selectedImage != 3 {
		t.Fatalf("escape changed selection: open=%v selected=%d", m.pickerOpen, m.selectedImage)
	}
}
