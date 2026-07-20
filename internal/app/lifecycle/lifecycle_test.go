package lifecycle_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	gws "github.com/gorilla/websocket"

	"github.com/teraerp/tera-agent/internal/adapters/communication/websocket"
	"github.com/teraerp/tera-agent/internal/adapters/logger"
	"github.com/teraerp/tera-agent/internal/adapters/printing/profile"
	"github.com/teraerp/tera-agent/internal/adapters/store"
	"github.com/teraerp/tera-agent/internal/app/dispatcher"
	"github.com/teraerp/tera-agent/internal/app/lifecycle"
	"github.com/teraerp/tera-agent/internal/app/ports"
	"github.com/teraerp/tera-agent/internal/domain/agent"
	"github.com/teraerp/tera-agent/internal/domain/comms"
	"github.com/teraerp/tera-agent/internal/domain/job"
	dp "github.com/teraerp/tera-agent/internal/domain/printing"
)

// fakeHandler records the print job instead of really printing.
type fakeHandler struct{ got chan job.Job }

func (h fakeHandler) Handle(_ context.Context, j job.Job) error {
	h.got <- j
	return nil
}

type fakeDiscovery struct{}

func (fakeDiscovery) List(context.Context) ([]dp.Printer, error) {
	return []dp.Printer{{ID: "KL200", Name: "KL200", Driver: "cups", Status: dp.StatusReady}}, nil
}
func (fakeDiscovery) StatusOf(context.Context, string) (dp.Status, error) {
	return dp.StatusReady, nil
}

// expect reads frames, skipping heartbeats, until it finds wantType.
func expect(t *testing.T, c *gws.Conn, wantType string) []byte {
	t.Helper()
	c.SetReadDeadline(time.Now().Add(3 * time.Second))
	for {
		_, raw, err := c.ReadMessage()
		if err != nil {
			t.Fatalf("read %s: %v", wantType, err)
		}
		var x comms.Typed
		_ = json.Unmarshal(raw, &x)
		if x.Type == comms.TypeHeartbeat {
			continue
		}
		if x.Type != wantType {
			t.Fatalf("got message %q, want %q", x.Type, wantType)
		}
		return raw
	}
}

func writeJSON(t *testing.T, c *gws.Conn, v any) {
	t.Helper()
	b, _ := json.Marshal(v)
	if err := c.WriteMessage(gws.TextMessage, b); err != nil {
		t.Fatalf("write: %v", err)
	}
}

func TestLifecycle_FullSessionRoundTrip(t *testing.T) {
	upgrader := gws.Upgrader{}
	completed := make(chan comms.JobCompleted, 1)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer c.Close()

		expect(t, c, comms.TypeHello)
		expect(t, c, comms.TypeAuthenticate)
		writeJSON(t, c, comms.Authenticated{Type: comms.TypeAuthenticated, AgentID: "a1", CompanyID: "co", BranchID: "br"})

		expect(t, c, comms.TypeRegister)
		caps := expect(t, c, comms.TypeCapabilities)
		if !strings.Contains(string(caps), "KL200") {
			t.Errorf("capabilities missing printer: %s", caps)
		}

		writeJSON(t, c, comms.ProfilesSync{Type: comms.TypeProfilesSync, Version: 1, Profiles: []comms.ProfileDTO{{
			PrinterID: "KL200", NativeFormats: []string{"escpos"}, WidthDots: 576, DPI: 203, SupportsCut: true,
		}}})
		expect(t, c, comms.TypeProfilesAck)

		writeJSON(t, c, comms.Job{
			Type: comms.TypeJob, ID: "job-1", JobType: "print",
			Payload: json.RawMessage(`{"printer_id":"KL200","format":"application/pdf","content":""}`),
		})
		expect(t, c, comms.TypeJobReceived)
		raw := expect(t, c, comms.TypeJobCompleted)
		var jc comms.JobCompleted
		_ = json.Unmarshal(raw, &jc)
		completed <- jc
	}))
	defer srv.Close()

	wsURL := strings.Replace(srv.URL, "http", "ws", 1)
	log := logger.New("error")

	profiles := profile.NewMemoryCache()
	got := make(chan job.Job, 1)
	disp := dispatcher.New(log)
	disp.Register(job.KindPrint, fakeHandler{got: got})

	jobStore, err := store.New(t.TempDir() + "/jobs.json")
	if err != nil {
		t.Fatal(err)
	}
	lc := lifecycle.New(
		func() comms.Transport { return websocket.New(wsURL, log, nil) },
		agent.NewMachine(),
		disp,
		fakeDiscovery{},
		profiles,
		jobStore,
		ports.Config{BackendURL: wsURL, Token: "tok", HeartbeatInterval: time.Minute},
		log,
		"test",
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = lc.Run(ctx) }()

	// The handler must have received the print job.
	select {
	case j := <-got:
		if j.ID != "job-1" {
			t.Fatalf("handler got job %q, want job-1", j.ID)
		}
	case <-time.After(4 * time.Second):
		t.Fatal("print handler was never invoked")
	}

	// The server must have received job_completed.
	select {
	case jc := <-completed:
		if jc.ID != "job-1" {
			t.Fatalf("job_completed id = %q, want job-1", jc.ID)
		}
	case <-time.After(4 * time.Second):
		t.Fatal("server never received job_completed")
	}

	// The profile must have been cached from profiles_sync.
	if p, err := profiles.Profile("KL200"); err != nil || p.WidthDots != 576 {
		t.Fatalf("profile not cached correctly: %+v err=%v", p, err)
	}
}
