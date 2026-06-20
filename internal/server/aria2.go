package server

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type Aria2Client struct {
	httpClient *http.Client
	config     func() Aria2Config
}

type Aria2CallRequest struct {
	Method string        `json:"method"`
	Params []interface{} `json:"params"`
}

func NewAria2Client(config func() Aria2Config) *Aria2Client {
	return &Aria2Client{
		config: config,
		httpClient: &http.Client{
			Timeout: 20 * time.Second,
		},
	}
}

func (c *Aria2Client) Call(req Aria2CallRequest) (interface{}, error) {
	if !isAllowedAria2Method(req.Method) {
		return nil, errors.New("method is not allowed")
	}
	cfg := c.config()
	params := aria2RPCParams(req.Method, req.Params, cfg.RPCSecret)

	payload := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      "ariamx",
		"method":  req.Method,
		"params":  params,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	httpReq, err := http.NewRequest(http.MethodPost, cfg.RPCURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("aria2 unreachable: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("aria2 returned status %d", resp.StatusCode)
	}
	var decoded struct {
		Result interface{} `json:"result"`
		Error  *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return nil, err
	}
	if decoded.Error != nil {
		return nil, fmt.Errorf("aria2 error %d: %s", decoded.Error.Code, decoded.Error.Message)
	}
	return localizeAria2Result(req.Method, decoded.Result), nil
}

func aria2RPCParams(method string, params []interface{}, secret string) []interface{} {
	if secret == "" {
		return params
	}
	token := "token:" + secret
	if method != "system.multicall" {
		next := make([]interface{}, 0, len(params)+1)
		next = append(next, token)
		next = append(next, params...)
		return next
	}
	if len(params) == 0 {
		return params
	}
	calls, ok := params[0].([]interface{})
	if !ok {
		return params
	}
	nextCalls := make([]interface{}, 0, len(calls))
	for _, call := range calls {
		callMap, ok := call.(map[string]interface{})
		if !ok {
			nextCalls = append(nextCalls, call)
			continue
		}
		nextCall := make(map[string]interface{}, len(callMap))
		for key, value := range callMap {
			nextCall[key] = value
		}
		methodName, _ := callMap["methodName"].(string)
		if strings.HasPrefix(methodName, "aria2.") {
			childParams, _ := callMap["params"].([]interface{})
			nextCall["params"] = prependOrReplaceAria2Token(childParams, token)
		}
		nextCalls = append(nextCalls, nextCall)
	}
	next := append([]interface{}{}, params...)
	next[0] = nextCalls
	return next
}

func prependOrReplaceAria2Token(params []interface{}, token string) []interface{} {
	if len(params) > 0 {
		if first, ok := params[0].(string); ok && strings.HasPrefix(first, "token:") {
			next := append([]interface{}{}, params...)
			next[0] = token
			return next
		}
	}
	next := make([]interface{}, 0, len(params)+1)
	next = append(next, token)
	next = append(next, params...)
	return next
}

func (c *Aria2Client) AddTorrent(r io.Reader, options map[string]string) (interface{}, error) {
	data, err := io.ReadAll(io.LimitReader(r, 32<<20))
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return nil, errors.New("empty torrent file")
	}
	encoded := base64.StdEncoding.EncodeToString(data)
	params := []interface{}{encoded}
	if len(options) > 0 {
		params = append(params, []string{}, options)
	}
	return c.Call(Aria2CallRequest{
		Method: "aria2.addTorrent",
		Params: params,
	})
}

func isAllowedAria2Method(method string) bool {
	if strings.HasPrefix(method, "aria2.") {
		return true
	}
	return strings.HasPrefix(method, "system.")
}
