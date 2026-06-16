package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHandleMCPRequiresPanelSecret(t *testing.T) {
	server, _, closeFn := newPanelRPCTestServer(t)
	defer closeFn()

	req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":"1","method":"initialize","params":{}}`))
	recorder := httptest.NewRecorder()

	server.handleMCP(recorder, req)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("expected status 401, got %d", recorder.Code)
	}
}

func TestHandleMCPDisabled(t *testing.T) {
	server, _, closeFn := newPanelRPCTestServer(t)
	defer closeFn()
	server.cfg.Panel.MCPEnabled = false

	req := httptest.NewRequest(http.MethodPost, "/mcp?secret=panel-secret", strings.NewReader(`{"jsonrpc":"2.0","id":"1","method":"initialize","params":{}}`))
	recorder := httptest.NewRecorder()

	server.handleMCP(recorder, req)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("expected status 403, got %d", recorder.Code)
	}
	var payload mcpResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Error == nil || payload.Error.Code != -32002 {
		t.Fatalf("expected disabled mcp error, got %#v", payload.Error)
	}
}

func TestHandleMCPInitialize(t *testing.T) {
	server, _, closeFn := newPanelRPCTestServer(t)
	defer closeFn()

	req := httptest.NewRequest(http.MethodPost, "/mcp?secret=panel-secret", strings.NewReader(`{"jsonrpc":"2.0","id":"1","method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test","version":"1.0.0"}}}`))
	recorder := httptest.NewRecorder()

	server.handleMCP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", recorder.Code)
	}
	var payload mcpResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Error != nil {
		t.Fatalf("expected no error, got %#v", payload.Error)
	}
	result, ok := payload.Result.(map[string]interface{})
	if !ok {
		t.Fatalf("expected result map, got %#v", payload.Result)
	}
	if result["protocolVersion"] != mcpProtocolVersion {
		t.Fatalf("unexpected protocol version: %#v", result["protocolVersion"])
	}
	capabilities, ok := result["capabilities"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected capabilities map, got %#v", result["capabilities"])
	}
	if _, ok := capabilities["tools"]; !ok {
		t.Fatalf("expected tools capability, got %#v", capabilities)
	}
	if _, ok := capabilities["resources"]; !ok {
		t.Fatalf("expected resources capability, got %#v", capabilities)
	}
	if _, ok := capabilities["prompts"]; !ok {
		t.Fatalf("expected prompts capability, got %#v", capabilities)
	}
	if _, ok := capabilities["completions"]; !ok {
		t.Fatalf("expected completions capability, got %#v", capabilities)
	}
}

func TestHandleMCPToolsList(t *testing.T) {
	server, _, closeFn := newPanelRPCTestServer(t)
	defer closeFn()

	req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":"tools","method":"tools/list","params":{}}`))
	req.Header.Set("Authorization", "Bearer panel-secret")
	recorder := httptest.NewRecorder()

	server.handleMCP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", recorder.Code)
	}
	var payload mcpResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	result, ok := payload.Result.(map[string]interface{})
	if !ok {
		t.Fatalf("expected result map, got %#v", payload.Result)
	}
	tools, ok := result["tools"].([]interface{})
	if !ok || len(tools) == 0 {
		t.Fatalf("expected tool list, got %#v", result["tools"])
	}
}

func TestHandleMCPToolCall(t *testing.T) {
	server, observed, closeFn := newPanelRPCTestServer(t)
	defer closeFn()

	req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":"call","method":"tools/call","params":{"name":"aria2_get_version","arguments":{}}}`))
	req.Header.Set("Authorization", "Bearer panel-secret")
	recorder := httptest.NewRecorder()

	server.handleMCP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", recorder.Code)
	}
	if observed.Method != "aria2.getVersion" {
		t.Fatalf("expected aria2.getVersion, got %s", observed.Method)
	}
	var payload mcpResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	result, ok := payload.Result.(map[string]interface{})
	if !ok {
		t.Fatalf("expected result map, got %#v", payload.Result)
	}
	if result["isError"] != false {
		t.Fatalf("expected non-error result, got %#v", result["isError"])
	}
}

func TestHandleMCPResourcesList(t *testing.T) {
	server, _, closeFn := newPanelRPCTestServer(t)
	defer closeFn()

	req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":"resources","method":"resources/list","params":{}}`))
	req.Header.Set("Authorization", "Bearer panel-secret")
	recorder := httptest.NewRecorder()

	server.handleMCP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", recorder.Code)
	}
	var payload mcpResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	result, ok := payload.Result.(map[string]interface{})
	if !ok {
		t.Fatalf("expected result map, got %#v", payload.Result)
	}
	resources, ok := result["resources"].([]interface{})
	if !ok || len(resources) == 0 {
		t.Fatalf("expected resources list, got %#v", result["resources"])
	}
}

func TestHandleMCPResourcesRead(t *testing.T) {
	server, observed, closeFn := newPanelRPCTestServer(t)
	defer closeFn()

	req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":"read","method":"resources/read","params":{"uri":"ariamx://task/gid-stopped-1"}}`))
	req.Header.Set("Authorization", "Bearer panel-secret")
	recorder := httptest.NewRecorder()

	server.handleMCP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", recorder.Code)
	}
	if observed.Method != "aria2.tellStatus" {
		t.Fatalf("expected aria2.tellStatus, got %s", observed.Method)
	}
	var payload mcpResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	result, ok := payload.Result.(map[string]interface{})
	if !ok {
		t.Fatalf("expected result map, got %#v", payload.Result)
	}
	contents, ok := result["contents"].([]interface{})
	if !ok || len(contents) != 1 {
		t.Fatalf("expected single content entry, got %#v", result["contents"])
	}
}

func TestHandleMCPPromptsGet(t *testing.T) {
	server, observed, closeFn := newPanelRPCTestServer(t)
	defer closeFn()

	req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":"prompt","method":"prompts/get","params":{"name":"ariamx_diagnose_task_failure","arguments":{"gid":"gid-stopped-1"}}}`))
	req.Header.Set("Authorization", "Bearer panel-secret")
	recorder := httptest.NewRecorder()

	server.handleMCP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", recorder.Code)
	}
	if observed.Method != "aria2.tellStatus" {
		t.Fatalf("expected aria2.tellStatus, got %s", observed.Method)
	}
	var payload mcpResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	result, ok := payload.Result.(map[string]interface{})
	if !ok {
		t.Fatalf("expected result map, got %#v", payload.Result)
	}
	messages, ok := result["messages"].([]interface{})
	if !ok || len(messages) == 0 {
		t.Fatalf("expected prompt messages, got %#v", result["messages"])
	}
}

func TestHandleMCPCompletion(t *testing.T) {
	server, _, closeFn := newPanelRPCTestServer(t)
	defer closeFn()

	req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":"complete","method":"completion/complete","params":{"ref":{"type":"ref/resource","uri":"ariamx://tasks/{bucket}"},"argument":{"name":"bucket","value":"st"}}}`))
	req.Header.Set("Authorization", "Bearer panel-secret")
	recorder := httptest.NewRecorder()

	server.handleMCP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", recorder.Code)
	}
	var payload mcpResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	result, ok := payload.Result.(map[string]interface{})
	if !ok {
		t.Fatalf("expected result map, got %#v", payload.Result)
	}
	completion, ok := result["completion"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected completion result, got %#v", result["completion"])
	}
	values, ok := completion["values"].([]interface{})
	if !ok || len(values) == 0 || values[0] != "stopped" {
		t.Fatalf("expected stopped completion, got %#v", completion["values"])
	}
}

func TestHandleMCPConnectionResourceDoesNotExposeInternalRPC(t *testing.T) {
	server, _, closeFn := newPanelRPCTestServer(t)
	defer closeFn()

	req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":"read","method":"resources/read","params":{"uri":"ariamx://connection/info"}}`))
	req.Header.Set("Authorization", "Bearer panel-secret")
	recorder := httptest.NewRecorder()

	server.handleMCP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", recorder.Code)
	}
	var payload mcpResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	result, ok := payload.Result.(map[string]interface{})
	if !ok {
		t.Fatalf("expected result map, got %#v", payload.Result)
	}
	contents, ok := result["contents"].([]interface{})
	if !ok || len(contents) != 1 {
		t.Fatalf("expected contents, got %#v", result["contents"])
	}
	content, ok := contents[0].(map[string]interface{})
	if !ok {
		t.Fatalf("expected content map, got %#v", contents[0])
	}
	text, _ := content["text"].(string)
	if strings.Contains(text, "internalAria2RpcUrl") || strings.Contains(text, "managedRpcPort") || strings.Contains(text, "aria2RpcUrl") {
		t.Fatalf("expected internal rpc details to be hidden, got %s", text)
	}
}
