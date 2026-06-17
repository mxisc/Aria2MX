package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHandlePanelRPCAcceptsBearerSecret(t *testing.T) {
	server, _, closeFn := newPanelRPCTestServer(t)
	defer closeFn()

	req := httptest.NewRequest(http.MethodPost, "/jsonrpc", strings.NewReader(`{"jsonrpc":"2.0","id":"1","method":"aria2.getVersion","params":[]}`))
	req.Header.Set("Authorization", "Bearer panel-secret")
	recorder := httptest.NewRecorder()

	server.handlePanelRPC(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", recorder.Code)
	}
	assertPanelRPCResult(t, recorder)
}

func TestHandlePanelRPCAcceptsAriaNgStylePanelSecret(t *testing.T) {
	server, observed, closeFn := newPanelRPCTestServer(t)
	defer closeFn()

	req := httptest.NewRequest(http.MethodPost, "/jsonrpc", strings.NewReader(`{"jsonrpc":"2.0","id":"1","method":"aria2.getVersion","params":["token:panel-secret"]}`))
	recorder := httptest.NewRecorder()

	server.handlePanelRPC(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", recorder.Code)
	}
	assertPanelRPCResult(t, recorder)
	if len(observed.Params) == 0 || observed.Params[0] != "token:aria-secret" {
		t.Fatalf("expected forwarded aria2 token, got %#v", observed.Params)
	}
}

func TestHandlePanelRPCRejectsWrongPanelSecret(t *testing.T) {
	server, _, closeFn := newPanelRPCTestServer(t)
	defer closeFn()

	req := httptest.NewRequest(http.MethodPost, "/jsonrpc", strings.NewReader(`{"jsonrpc":"2.0","id":"1","method":"aria2.getVersion","params":["token:wrong-secret"]}`))
	recorder := httptest.NewRecorder()

	server.handlePanelRPC(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", recorder.Code)
	}
	var payload panelRPCResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Error == nil || payload.Error.Code != -32001 {
		t.Fatalf("expected unauthorized panel rpc error, got %#v", payload.Error)
	}
}

func TestPanelRPCOriginAllowedSameOriginMode(t *testing.T) {
	server, _, closeFn := newPanelRPCTestServer(t)
	defer closeFn()

	server.cfg.Panel.RPCOriginCheckMode = panelRPCOriginModeSameOrigin

	req := httptest.NewRequest(http.MethodGet, "/jsonrpc", nil)
	req.Host = "panel.example.com"
	req.Header.Set("Origin", "https://panel.example.com")
	if !server.panelRPCOriginAllowed(req) {
		t.Fatal("expected same host origin to be allowed")
	}

	req.Header.Set("Origin", "https://ariang.example.com")
	if server.panelRPCOriginAllowed(req) {
		t.Fatal("expected foreign origin to be rejected in same origin mode")
	}
}

func TestPanelRPCOriginAllowedDisabledMode(t *testing.T) {
	server, _, closeFn := newPanelRPCTestServer(t)
	defer closeFn()

	server.cfg.Panel.RPCOriginCheckMode = panelRPCOriginModeDisabled

	req := httptest.NewRequest(http.MethodGet, "/jsonrpc", nil)
	req.Host = "panel.example.com"
	req.Header.Set("Origin", "https://ariang.example.com")
	if !server.panelRPCOriginAllowed(req) {
		t.Fatal("expected foreign origin to be allowed when disabled")
	}
}

func TestPanelRPCOriginAllowedWhitelistMode(t *testing.T) {
	server, _, closeFn := newPanelRPCTestServer(t)
	defer closeFn()

	server.cfg.Panel.RPCOriginCheckMode = panelRPCOriginModeWhitelist
	server.cfg.Panel.RPCOriginWhitelist = []string{"ariang.example.com", "panel.example.com:8443"}

	req := httptest.NewRequest(http.MethodGet, "/jsonrpc", nil)
	req.Host = "panel.example.com"
	req.Header.Set("Origin", "https://ariang.example.com")
	if !server.panelRPCOriginAllowed(req) {
		t.Fatal("expected whitelisted host to be allowed")
	}

	req.Header.Set("Origin", "https://panel.example.com:8443")
	if !server.panelRPCOriginAllowed(req) {
		t.Fatal("expected exact host:port whitelist to be allowed")
	}

	req.Header.Set("Origin", "https://evil.example")
	if server.panelRPCOriginAllowed(req) {
		t.Fatal("expected non-whitelisted host to be rejected")
	}
}

type observedRPCRequest struct {
	Method string        `json:"method"`
	Params []interface{} `json:"params"`
}

func newPanelRPCTestServer(t *testing.T) (*Server, *observedRPCRequest, func()) {
	t.Helper()
	observed := &observedRPCRequest{}
	aria2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(observed); err != nil {
			t.Fatalf("decode aria2 request: %v", err)
		}
		result := interface{}(map[string]interface{}{
			"version": "1.37.0",
		})
		switch observed.Method {
		case "aria2.getVersion":
			result = map[string]interface{}{
				"version": "1.37.0",
				"enabledFeatures": []string{
					"BitTorrent",
					"HTTPS",
				},
			}
		case "aria2.getGlobalStat":
			result = map[string]interface{}{
				"downloadSpeed": "1024",
				"uploadSpeed":   "64",
				"numActive":     "1",
				"numWaiting":    "1",
				"numStopped":    "1",
			}
		case "aria2.getGlobalOption":
			result = map[string]interface{}{
				"dir":                      "/downloads",
				"max-concurrent-downloads": "5",
				"max-download-limit":       "0",
			}
		case "aria2.tellActive":
			result = []map[string]interface{}{
				{
					"gid":           "gid-active-1",
					"status":        "active",
					"downloadSpeed": "1024",
					"totalLength":   "2048",
				},
			}
		case "aria2.tellWaiting":
			result = []map[string]interface{}{
				{
					"gid":         "gid-waiting-1",
					"status":      "waiting",
					"totalLength": "4096",
				},
			}
		case "aria2.tellStopped":
			result = []map[string]interface{}{
				{
					"gid":          "gid-stopped-1",
					"status":       "error",
					"errorCode":    "16",
					"errorMessage": "Operation not permitted",
				},
			}
		case "aria2.tellStatus":
			gid := "gid-unknown"
			if len(observed.Params) > 0 {
				gid, _ = observed.Params[0].(string)
			}
			result = map[string]interface{}{
				"gid":          gid,
				"status":       "error",
				"dir":          "/downloads",
				"totalLength":  "8192",
				"errorCode":    "16",
				"errorMessage": "Operation not permitted",
			}
		case "aria2.addUri":
			result = "new-gid"
		case "aria2.pause", "aria2.unpause", "aria2.remove":
			if len(observed.Params) > 0 {
				result = observed.Params[0]
			}
		case "aria2.pauseAll", "aria2.unpauseAll", "aria2.saveSession":
			result = "OK"
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"jsonrpc": "2.0",
			"id":      "ariamx",
			"result":  result,
		})
	}))
	server := &Server{
		cfg: &Config{
			Aria2: Aria2Config{
				RPCURL:    aria2.URL,
				RPCSecret: "aria-secret",
			},
			Panel: PanelConfig{
				RPCSecret:          "panel-secret",
				RPCOriginCheckMode: panelRPCOriginModeSameOrigin,
				MCPEnabled:         true,
			},
		},
		sessions: NewSessionStore(),
	}
	server.aria2 = NewAria2Client(func() Aria2Config { return server.cfg.Aria2 })
	return server, observed, aria2.Close
}

func assertPanelRPCResult(t *testing.T, recorder *httptest.ResponseRecorder) {
	t.Helper()
	var payload panelRPCResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Error != nil {
		t.Fatalf("expected no rpc error, got %#v", payload.Error)
	}
}
