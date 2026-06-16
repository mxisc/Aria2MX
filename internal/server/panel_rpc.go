package server

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"

	"github.com/gorilla/websocket"
)

var errPanelRPCUnauthorized = errors.New("请先提供面板 RPC Secret 或登录后重试。")

type panelRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Method  string          `json:"method"`
	Params  []interface{}   `json:"params"`
}

type panelRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  interface{}     `json:"result,omitempty"`
	Error   *panelRPCError  `json:"error,omitempty"`
}

type panelRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

var panelRPCUpgrader = websocket.Upgrader{
	ReadBufferSize:  32 << 10,
	WriteBufferSize: 32 << 10,
	CheckOrigin: func(r *http.Request) bool {
		origin := r.Header.Get("Origin")
		if origin == "" {
			return true
		}
		parsed, err := url.Parse(origin)
		if err != nil {
			return false
		}
		return parsed.Host == r.Host
	},
}

func (s *Server) handlePanelRPC(w http.ResponseWriter, r *http.Request) {
	outerAuthorized := s.panelRPCOuterAuthorized(r)
	if websocket.IsWebSocketUpgrade(r) {
		s.handlePanelRPCWebSocket(w, r, outerAuthorized)
		return
	}
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}

	payload, status, err := s.proxyPanelRPCPayload(r.Body, outerAuthorized)
	if err != nil {
		writeRawPanelRPCError(w, status, nil, err)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(payload)
}

func (s *Server) handlePanelRPCWebSocket(w http.ResponseWriter, r *http.Request, outerAuthorized bool) {
	conn, err := panelRPCUpgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	for {
		messageType, data, err := conn.ReadMessage()
		if err != nil {
			return
		}
		if messageType != websocket.TextMessage && messageType != websocket.BinaryMessage {
			continue
		}
		payload, _, rpcErr := s.proxyPanelRPCPayload(bytes.NewReader(data), outerAuthorized)
		if rpcErr != nil {
			response, marshalErr := json.Marshal(panelRPCResponse{
				JSONRPC: "2.0",
				Error: &panelRPCError{
					Code:    -32600,
					Message: rpcErr.Error(),
				},
			})
			if marshalErr != nil {
				return
			}
			if err := conn.WriteMessage(websocket.TextMessage, response); err != nil {
				return
			}
			continue
		}
		if len(bytes.TrimSpace(payload)) == 0 {
			continue
		}
		if err := conn.WriteMessage(websocket.TextMessage, payload); err != nil {
			return
		}
	}
}

func (s *Server) proxyPanelRPCPayload(bodyReader interface{ Read([]byte) (int, error) }, outerAuthorized bool) ([]byte, int, error) {
	raw, err := readLimitedBytes(bodyReader)
	if err != nil {
		return nil, http.StatusBadRequest, errors.New("请检查 RPC 请求内容后重试。")
	}
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 {
		return nil, http.StatusBadRequest, errors.New("请检查 RPC 请求内容后重试。")
	}

	if raw[0] == '[' {
		var requests []panelRPCRequest
		if err := json.Unmarshal(raw, &requests); err != nil {
			return nil, http.StatusBadRequest, errors.New("请检查 RPC 请求内容后重试。")
		}
		responses := make([]panelRPCResponse, 0, len(requests))
		for _, req := range requests {
			response, respond := s.executePanelRPC(req, outerAuthorized)
			if !respond {
				continue
			}
			responses = append(responses, response)
		}
		if len(responses) == 0 {
			return nil, http.StatusOK, nil
		}
		payload, err := json.Marshal(responses)
		if err != nil {
			return nil, http.StatusInternalServerError, errors.New("RPC 代理暂时不可用，请稍后重试。")
		}
		return payload, http.StatusOK, nil
	}

	var req panelRPCRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		return nil, http.StatusBadRequest, errors.New("请检查 RPC 请求内容后重试。")
	}
	response, respond := s.executePanelRPC(req, outerAuthorized)
	if !respond {
		return nil, http.StatusOK, nil
	}
	payload, err := json.Marshal(response)
	if err != nil {
		return nil, http.StatusInternalServerError, errors.New("RPC 代理暂时不可用，请稍后重试。")
	}
	return payload, http.StatusOK, nil
}

func (s *Server) executePanelRPC(req panelRPCRequest, outerAuthorized bool) (panelRPCResponse, bool) {
	response := panelRPCResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
	}
	if req.Method == "" {
		response.Error = &panelRPCError{Code: -32600, Message: "请检查 RPC 请求内容后重试。"}
		return response, len(req.ID) > 0
	}
	if !outerAuthorized && !s.matchesPanelRPCParamSecret(req.Params) {
		response.Error = &panelRPCError{Code: -32001, Message: errPanelRPCUnauthorized.Error()}
		return response, len(req.ID) > 0
	}

	result, err := s.aria2.Call(Aria2CallRequest{
		Method: req.Method,
		Params: sanitizePanelRPCParams(req.Params),
	})
	if err != nil {
		response.Error = &panelRPCError{Code: -32000, Message: "aria2 暂时不可用，请检查连接设置。"}
		return response, len(req.ID) > 0
	}
	response.Result = result
	return response, len(req.ID) > 0
}

func sanitizePanelRPCParams(params []interface{}) []interface{} {
	if len(params) == 0 {
		return params
	}
	first, ok := params[0].(string)
	if ok && len(first) > 6 && first[:6] == "token:" {
		return params[1:]
	}
	return params
}

func (s *Server) panelRPCOuterAuthorized(r *http.Request) bool {
	if _, ok, _ := s.authenticatedSession(r); ok {
		return true
	}
	return s.matchesPanelRPCSecret(r)
}

func (s *Server) matchesPanelRPCParamSecret(params []interface{}) bool {
	if len(params) == 0 {
		return false
	}
	token, ok := params[0].(string)
	if !ok || len(token) <= 6 || token[:6] != "token:" {
		return false
	}
	return s.matchesPanelRPCSecretValue(token[6:])
}

func readLimitedBytes(reader interface{ Read([]byte) (int, error) }) ([]byte, error) {
	return io.ReadAll(io.LimitReader(reader, 1<<20))
}

func writeRawPanelRPCError(w http.ResponseWriter, status int, id json.RawMessage, err error) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(panelRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Error: &panelRPCError{
			Code:    -32600,
			Message: err.Error(),
		},
	})
}
