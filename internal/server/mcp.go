package server

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"ariamx/internal/version"
)

const (
	mcpProtocolVersion        = "2024-11-05"
	mcpDefaultPageSize        = 50
	mcpResourceConnectionInfo = "ariamx://connection/info"
	mcpResourcePanelConfig    = "ariamx://config/panel"
	mcpResourceGlobalOption   = "ariamx://config/aria2/global-options"
	mcpResourceGlobalStat     = "ariamx://stats/global"
	mcpResourceTasksActive    = "ariamx://tasks/active"
	mcpResourceTasksWaiting   = "ariamx://tasks/waiting"
	mcpResourceTasksStopped   = "ariamx://tasks/stopped"
	mcpTaskTemplate           = "ariamx://task/{gid}"
	mcpTaskBucketTemplate     = "ariamx://tasks/{bucket}"
	mcpOptionTemplate         = "ariamx://option/{key}"
)

var mcpTaskFields = []string{
	"gid",
	"status",
	"totalLength",
	"completedLength",
	"uploadLength",
	"downloadSpeed",
	"uploadSpeed",
	"connections",
	"dir",
	"files",
	"bittorrent",
	"followedBy",
	"following",
	"belongsTo",
	"numPieces",
	"pieceLength",
	"numSeeders",
	"seeder",
	"errorCode",
	"errorMessage",
}

type mcpRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type mcpResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  interface{}     `json:"result,omitempty"`
	Error   *mcpError       `json:"error,omitempty"`
}

type mcpError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type mcpTool struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	InputSchema map[string]interface{} `json:"inputSchema"`
}

type mcpResource struct {
	URI         string `json:"uri"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	MimeType    string `json:"mimeType,omitempty"`
}

type mcpResourceTemplate struct {
	URITemplate string `json:"uriTemplate"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	MimeType    string `json:"mimeType,omitempty"`
}

type mcpPrompt struct {
	Name        string              `json:"name"`
	Title       string              `json:"title,omitempty"`
	Description string              `json:"description,omitempty"`
	Arguments   []mcpPromptArgument `json:"arguments,omitempty"`
}

type mcpPromptArgument struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Required    bool   `json:"required,omitempty"`
}

type mcpResourceContent struct {
	URI      string `json:"uri"`
	MimeType string `json:"mimeType,omitempty"`
	Text     string `json:"text,omitempty"`
	Blob     string `json:"blob,omitempty"`
}

func (s *Server) handleMCP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	s.cfgMu.RLock()
	enabled := s.cfg.Panel.MCPEnabled
	s.cfgMu.RUnlock()
	if !enabled {
		s.writeMCPResponse(w, http.StatusForbidden, mcpResponse{
			JSONRPC: "2.0",
			Error: &mcpError{
				Code:    -32002,
				Message: "面板已关闭 MCP 功能，请在面板设置中开启后重试。",
			},
		})
		return
	}
	if !s.panelRPCOuterAuthorized(r) {
		s.writeMCPResponse(w, http.StatusUnauthorized, mcpResponse{
			JSONRPC: "2.0",
			Error: &mcpError{
				Code:    -32001,
				Message: "请先提供面板 RPC Secret 或登录后重试。",
			},
		})
		return
	}

	var req mcpRequest
	if err := readJSON(r, &req); err != nil {
		s.writeMCPResponse(w, http.StatusBadRequest, mcpResponse{
			JSONRPC: "2.0",
			Error: &mcpError{
				Code:    -32600,
				Message: "请检查 MCP 请求内容后重试。",
			},
		})
		return
	}

	resp, shouldRespond := s.executeMCP(req)
	if !shouldRespond {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	s.writeMCPResponse(w, http.StatusOK, resp)
}

func (s *Server) executeMCP(req mcpRequest) (mcpResponse, bool) {
	resp := mcpResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
	}

	switch req.Method {
	case "notifications/initialized":
		return resp, false
	case "initialize":
		resp.Result = map[string]interface{}{
			"protocolVersion": mcpProtocolVersion,
			"capabilities": map[string]interface{}{
				"tools": map[string]interface{}{
					"listChanged": false,
				},
				"resources": map[string]interface{}{},
				"prompts": map[string]interface{}{
					"listChanged": false,
				},
				"completions": map[string]interface{}{},
			},
			"serverInfo": map[string]interface{}{
				"name":    "AriaMX MCP",
				"version": version.PanelVersion,
			},
		}
		return resp, true
	case "ping":
		resp.Result = map[string]interface{}{}
		return resp, true
	case "tools/list":
		result, err := mcpListResponse(req.Params, "tools", mcpToolList())
		if err != nil {
			resp.Error = mcpInvalidParams(err.Error())
		} else {
			resp.Result = result
		}
		return resp, true
	case "tools/call":
		result, err := s.executeMCPTool(req.Params)
		if err != nil {
			resp.Result = map[string]interface{}{
				"content": []map[string]string{
					{
						"type": "text",
						"text": err.Error(),
					},
				},
				"isError": true,
			}
			return resp, true
		}
		resp.Result = result
		return resp, true
	case "resources/list":
		result, err := mcpListResponse(req.Params, "resources", mcpResourceList())
		if err != nil {
			resp.Error = mcpInvalidParams(err.Error())
		} else {
			resp.Result = result
		}
		return resp, true
	case "resources/templates/list":
		result, err := mcpListResponse(req.Params, "resourceTemplates", mcpResourceTemplateList())
		if err != nil {
			resp.Error = mcpInvalidParams(err.Error())
		} else {
			resp.Result = result
		}
		return resp, true
	case "resources/read":
		result, err := s.executeMCPResourceRead(req.Params)
		if err != nil {
			resp.Error = mcpInvalidParams(err.Error())
		} else {
			resp.Result = result
		}
		return resp, true
	case "prompts/list":
		result, err := mcpListResponse(req.Params, "prompts", mcpPromptList())
		if err != nil {
			resp.Error = mcpInvalidParams(err.Error())
		} else {
			resp.Result = result
		}
		return resp, true
	case "prompts/get":
		result, err := s.executeMCPPromptGet(req.Params)
		if err != nil {
			resp.Error = mcpInvalidParams(err.Error())
		} else {
			resp.Result = result
		}
		return resp, true
	case "completion/complete":
		result, err := s.executeMCPCompletion(req.Params)
		if err != nil {
			resp.Error = mcpInvalidParams(err.Error())
		} else {
			resp.Result = result
		}
		return resp, true
	default:
		resp.Error = &mcpError{
			Code:    -32601,
			Message: "暂不支持该 MCP 方法。",
		}
		return resp, true
	}
}

func (s *Server) executeMCPTool(raw json.RawMessage) (map[string]interface{}, error) {
	var payload struct {
		Name      string                 `json:"name"`
		Arguments map[string]interface{} `json:"arguments"`
	}
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil, errors.New("请检查 MCP 工具调用参数后重试。")
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, errors.New("请检查 MCP 工具调用参数后重试。")
	}

	result, err := s.callMCPTool(payload.Name, payload.Arguments)
	if err != nil {
		return nil, err
	}

	text, err := marshalMCPText(result)
	if err != nil {
		return nil, errors.New("MCP 工具结果暂时无法编码，请稍后重试。")
	}
	return map[string]interface{}{
		"content": []map[string]string{
			{
				"type": "text",
				"text": text,
			},
		},
		"structuredContent": result,
		"isError":           false,
	}, nil
}

func (s *Server) executeMCPResourceRead(raw json.RawMessage) (map[string]interface{}, error) {
	var payload struct {
		URI string `json:"uri"`
	}
	if len(bytes.TrimSpace(raw)) == 0 || json.Unmarshal(raw, &payload) != nil || strings.TrimSpace(payload.URI) == "" {
		return nil, errors.New("请提供要读取的资源 URI。")
	}

	contents, err := s.readMCPResource(strings.TrimSpace(payload.URI))
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{
		"contents": contents,
	}, nil
}

func (s *Server) executeMCPPromptGet(raw json.RawMessage) (map[string]interface{}, error) {
	var payload struct {
		Name      string            `json:"name"`
		Arguments map[string]string `json:"arguments"`
	}
	if len(bytes.TrimSpace(raw)) == 0 || json.Unmarshal(raw, &payload) != nil || strings.TrimSpace(payload.Name) == "" {
		return nil, errors.New("请提供要读取的 Prompt 名称。")
	}

	messages, description, err := s.getMCPPrompt(strings.TrimSpace(payload.Name), payload.Arguments)
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{
		"description": description,
		"messages":    messages,
	}, nil
}

func (s *Server) executeMCPCompletion(raw json.RawMessage) (map[string]interface{}, error) {
	var payload struct {
		Ref      map[string]interface{} `json:"ref"`
		Argument struct {
			Name  string `json:"name"`
			Value string `json:"value"`
		} `json:"argument"`
		Context struct {
			Arguments map[string]string `json:"arguments"`
		} `json:"context"`
	}
	if len(bytes.TrimSpace(raw)) == 0 || json.Unmarshal(raw, &payload) != nil {
		return nil, errors.New("请检查补全请求参数后重试。")
	}
	refType, _ := payload.Ref["type"].(string)
	argName := strings.TrimSpace(payload.Argument.Name)
	argValue := strings.TrimSpace(payload.Argument.Value)
	if refType == "" || argName == "" {
		return nil, errors.New("请提供补全引用和参数名。")
	}

	var (
		values []string
		err    error
	)
	switch refType {
	case "ref/prompt":
		promptName, _ := payload.Ref["name"].(string)
		values, err = s.completeMCPPromptArgument(promptName, argName, argValue, payload.Context.Arguments)
	case "ref/resource":
		uri, _ := payload.Ref["uri"].(string)
		values, err = s.completeMCPResourceArgument(uri, argName, argValue)
	default:
		return nil, errors.New("暂不支持该补全引用类型。")
	}
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{
		"completion": map[string]interface{}{
			"values":  values,
			"total":   len(values),
			"hasMore": false,
		},
	}, nil
}

func (s *Server) callMCPTool(name string, arguments map[string]interface{}) (interface{}, error) {
	switch name {
	case "aria2_get_version":
		return s.aria2.Call(Aria2CallRequest{Method: "aria2.getVersion"})
	case "aria2_get_global_stat":
		return s.aria2.Call(Aria2CallRequest{Method: "aria2.getGlobalStat"})
	case "aria2_get_global_option":
		return s.aria2.Call(Aria2CallRequest{Method: "aria2.getGlobalOption"})
	case "aria2_tell_active":
		keys := stringSliceArg(arguments, "keys")
		params := []interface{}{}
		if len(keys) > 0 {
			params = append(params, keys)
		}
		return s.aria2.Call(Aria2CallRequest{Method: "aria2.tellActive", Params: params})
	case "aria2_tell_waiting":
		offset := intArg(arguments, "offset", 0)
		limit := intArg(arguments, "limit", 20)
		params := []interface{}{offset, limit}
		if keys := stringSliceArg(arguments, "keys"); len(keys) > 0 {
			params = append(params, keys)
		}
		return s.aria2.Call(Aria2CallRequest{Method: "aria2.tellWaiting", Params: params})
	case "aria2_tell_stopped":
		offset := intArg(arguments, "offset", 0)
		limit := intArg(arguments, "limit", 20)
		params := []interface{}{offset, limit}
		if keys := stringSliceArg(arguments, "keys"); len(keys) > 0 {
			params = append(params, keys)
		}
		return s.aria2.Call(Aria2CallRequest{Method: "aria2.tellStopped", Params: params})
	case "aria2_add_uri":
		uris := stringSliceArg(arguments, "uris")
		if len(uris) == 0 {
			return nil, errors.New("请至少提供一个下载地址。")
		}
		params := []interface{}{uris}
		if options := stringMapArg(arguments, "options"); len(options) > 0 {
			params = append(params, options)
		}
		if position, ok := optionalIntArg(arguments, "position"); ok {
			params = append(params, position)
		}
		return s.aria2.Call(Aria2CallRequest{Method: "aria2.addUri", Params: params})
	case "aria2_pause":
		gid := stringArg(arguments, "gid")
		if gid == "" {
			return nil, errors.New("请提供任务 GID。")
		}
		return s.aria2.Call(Aria2CallRequest{Method: "aria2.pause", Params: []interface{}{gid}})
	case "aria2_unpause":
		gid := stringArg(arguments, "gid")
		if gid == "" {
			return nil, errors.New("请提供任务 GID。")
		}
		return s.aria2.Call(Aria2CallRequest{Method: "aria2.unpause", Params: []interface{}{gid}})
	case "aria2_remove":
		gid := stringArg(arguments, "gid")
		if gid == "" {
			return nil, errors.New("请提供任务 GID。")
		}
		return s.aria2.Call(Aria2CallRequest{Method: "aria2.remove", Params: []interface{}{gid}})
	case "aria2_pause_all":
		return s.aria2.Call(Aria2CallRequest{Method: "aria2.pauseAll"})
	case "aria2_unpause_all":
		return s.aria2.Call(Aria2CallRequest{Method: "aria2.unpauseAll"})
	case "aria2_save_session":
		return s.aria2.Call(Aria2CallRequest{Method: "aria2.saveSession"})
	default:
		return nil, fmt.Errorf("未找到 MCP 工具：%s", name)
	}
}

func (s *Server) readMCPResource(uri string) ([]mcpResourceContent, error) {
	switch uri {
	case mcpResourceConnectionInfo:
		payload, err := s.mcpConnectionInfoResource()
		if err != nil {
			return nil, err
		}
		return mcpJSONContents(uri, payload)
	case mcpResourcePanelConfig:
		payload := s.mcpPanelConfigResource()
		return mcpJSONContents(uri, payload)
	case mcpResourceGlobalOption:
		payload, err := s.aria2.Call(Aria2CallRequest{Method: "aria2.getGlobalOption"})
		if err != nil {
			return nil, errors.New(userFacingAria2Error(err))
		}
		return mcpJSONContents(uri, payload)
	case mcpResourceGlobalStat:
		payload, err := s.aria2.Call(Aria2CallRequest{Method: "aria2.getGlobalStat"})
		if err != nil {
			return nil, errors.New(userFacingAria2Error(err))
		}
		return mcpJSONContents(uri, payload)
	case mcpResourceTasksActive:
		payload, err := s.mcpTaskBucketResource("active")
		if err != nil {
			return nil, err
		}
		return mcpJSONContents(uri, payload)
	case mcpResourceTasksWaiting:
		payload, err := s.mcpTaskBucketResource("waiting")
		if err != nil {
			return nil, err
		}
		return mcpJSONContents(uri, payload)
	case mcpResourceTasksStopped:
		payload, err := s.mcpTaskBucketResource("stopped")
		if err != nil {
			return nil, err
		}
		return mcpJSONContents(uri, payload)
	}

	if strings.HasPrefix(uri, "ariamx://task/") {
		gid := strings.TrimSpace(strings.TrimPrefix(uri, "ariamx://task/"))
		if gid == "" {
			return nil, errors.New("请提供任务 GID。")
		}
		payload, err := s.mcpTaskResource(gid)
		if err != nil {
			return nil, err
		}
		return mcpJSONContents(uri, payload)
	}

	if strings.HasPrefix(uri, "ariamx://tasks/") {
		bucket := strings.TrimSpace(strings.TrimPrefix(uri, "ariamx://tasks/"))
		payload, err := s.mcpTaskBucketResource(bucket)
		if err != nil {
			return nil, err
		}
		return mcpJSONContents(uri, payload)
	}

	if strings.HasPrefix(uri, "ariamx://option/") {
		key := strings.TrimSpace(strings.TrimPrefix(uri, "ariamx://option/"))
		if key == "" {
			return nil, errors.New("请提供 aria2 配置项名称。")
		}
		payload, err := s.mcpOptionResource(key)
		if err != nil {
			return nil, err
		}
		return mcpJSONContents(uri, payload)
	}

	return nil, fmt.Errorf("未找到资源：%s", uri)
}

func (s *Server) getMCPPrompt(name string, arguments map[string]string) ([]map[string]interface{}, string, error) {
	switch name {
	case "ariamx_diagnose_task_failure":
		gid := strings.TrimSpace(arguments["gid"])
		if gid == "" {
			return nil, "", errors.New("请提供失败任务的 GID。")
		}
		resource, err := s.mcpEmbeddedResource("ariamx://task/" + gid)
		if err != nil {
			return nil, "", err
		}
		return []map[string]interface{}{
			{
				"role": "user",
				"content": map[string]interface{}{
					"type": "text",
					"text": "请结合下面的任务详情，判断当前下载失败或停止的直接原因，优先指出用户下一步该改什么。",
				},
			},
			{
				"role": "user",
				"content": map[string]interface{}{
					"type":     "resource",
					"resource": resource,
				},
			},
		}, "分析单个下载任务为什么失败或停止。", nil
	case "ariamx_summarize_downloads":
		bucket := strings.TrimSpace(arguments["bucket"])
		if bucket == "" {
			bucket = "all"
		}
		uris := []string{}
		switch bucket {
		case "all":
			uris = []string{mcpResourceTasksActive, mcpResourceTasksWaiting, mcpResourceTasksStopped}
		case "active", "waiting", "stopped":
			uris = []string{"ariamx://tasks/" + bucket}
		default:
			return nil, "", errors.New("bucket 仅支持 all、active、waiting、stopped。")
		}
		messages := []map[string]interface{}{
			{
				"role": "user",
				"content": map[string]interface{}{
					"type": "text",
					"text": "请总结当前下载队列状态，指出活动任务、等待任务和失败任务的关键信息，并给出需要优先处理的任务。",
				},
			},
		}
		for _, uri := range uris {
			resource, err := s.mcpEmbeddedResource(uri)
			if err != nil {
				return nil, "", err
			}
			messages = append(messages, map[string]interface{}{
				"role": "user",
				"content": map[string]interface{}{
					"type":     "resource",
					"resource": resource,
				},
			})
		}
		return messages, "总结当前下载队列和异常任务。", nil
	case "ariamx_explain_client_setup":
		client := strings.TrimSpace(arguments["client"])
		if client == "" {
			client = "ariang"
		}
		resource, err := s.mcpEmbeddedResource(mcpResourceConnectionInfo)
		if err != nil {
			return nil, "", err
		}
		return []map[string]interface{}{
			{
				"role": "user",
				"content": map[string]interface{}{
					"type": "text",
					"text": fmt.Sprintf("请基于下面的连接信息，说明如何让 %s 连接 AriaMX 面板代理的 aria2 RPC。优先使用 aria2 原生的 token 写法。", client),
				},
			},
			{
				"role": "user",
				"content": map[string]interface{}{
					"type":     "resource",
					"resource": resource,
				},
			},
		}, "解释不同客户端如何连接 AriaMX 的 RPC 代理。", nil
	case "ariamx_tune_global_options":
		goal := strings.TrimSpace(arguments["goal"])
		if goal == "" {
			goal = "speed"
		}
		resource, err := s.mcpEmbeddedResource(mcpResourceGlobalOption)
		if err != nil {
			return nil, "", err
		}
		return []map[string]interface{}{
			{
				"role": "user",
				"content": map[string]interface{}{
					"type": "text",
					"text": fmt.Sprintf("请基于当前 aria2 全局配置，给出面向 %s 的参数调整建议。只输出有必要修改的项，并说明理由。", goal),
				},
			},
			{
				"role": "user",
				"content": map[string]interface{}{
					"type":     "resource",
					"resource": resource,
				},
			},
		}, "根据目标给出 aria2 全局参数调优建议。", nil
	default:
		return nil, "", fmt.Errorf("未找到 Prompt：%s", name)
	}
}

func (s *Server) completeMCPPromptArgument(name, argName, prefix string, context map[string]string) ([]string, error) {
	switch name {
	case "ariamx_diagnose_task_failure":
		if argName != "gid" {
			return nil, nil
		}
		gids, err := s.mcpTaskGIDs("stopped")
		if err != nil {
			return nil, err
		}
		return filterCompletionValues(gids, prefix), nil
	case "ariamx_summarize_downloads":
		if argName != "bucket" {
			return nil, nil
		}
		return filterCompletionValues([]string{"all", "active", "waiting", "stopped"}, prefix), nil
	case "ariamx_explain_client_setup":
		if argName != "client" {
			return nil, nil
		}
		return filterCompletionValues([]string{"ariang", "postman", "curl", "custom"}, prefix), nil
	case "ariamx_tune_global_options":
		if argName != "goal" {
			return nil, nil
		}
		values := []string{"speed", "stability", "bt", "seeding"}
		if strings.EqualFold(strings.TrimSpace(context["transport"]), "http") {
			values = append(values, "http")
		}
		return filterCompletionValues(values, prefix), nil
	default:
		return nil, fmt.Errorf("未找到 Prompt：%s", name)
	}
}

func (s *Server) completeMCPResourceArgument(uriTemplate, argName, prefix string) ([]string, error) {
	switch uriTemplate {
	case mcpTaskTemplate:
		if argName != "gid" {
			return nil, nil
		}
		gids, err := s.mcpTaskGIDs("all")
		if err != nil {
			return nil, err
		}
		return filterCompletionValues(gids, prefix), nil
	case mcpTaskBucketTemplate:
		if argName != "bucket" {
			return nil, nil
		}
		return filterCompletionValues([]string{"active", "waiting", "stopped"}, prefix), nil
	case mcpOptionTemplate:
		if argName != "key" {
			return nil, nil
		}
		options, err := s.aria2.Call(Aria2CallRequest{Method: "aria2.getGlobalOption"})
		if err != nil {
			return nil, errors.New(userFacingAria2Error(err))
		}
		optionMap, ok := options.(map[string]interface{})
		if !ok {
			return nil, errors.New("当前 aria2 配置读取失败，请稍后重试。")
		}
		keys := make([]string, 0, len(optionMap))
		for key := range optionMap {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		return filterCompletionValues(keys, prefix), nil
	default:
		return nil, fmt.Errorf("未找到资源模板：%s", uriTemplate)
	}
}

func (s *Server) mcpConnectionInfoResource() (map[string]interface{}, error) {
	s.cfgMu.RLock()
	panelSecret := s.cfg.Panel.RPCSecret
	enabled := s.cfg.Panel.MCPEnabled
	managed := s.cfg.Aria2.Managed
	s.cfgMu.RUnlock()

	return map[string]interface{}{
		"panelVersion":   version.PanelVersion,
		"aria2Version":   s.currentAria2Version(),
		"jsonrpcPath":    "/jsonrpc",
		"mcpPath":        "/mcp",
		"mcpEnabled":     enabled,
		"panelRpcSecret": panelSecret,
		"aria2Managed":   managed,
		"authModes": []string{
			`params[0] = "token:<panel rpc secret>"`,
			"Authorization: Bearer <panel rpc secret>",
			"?secret=<panel rpc secret>",
		},
	}, nil
}

func (s *Server) mcpPanelConfigResource() map[string]interface{} {
	s.cfgMu.RLock()
	defer s.cfgMu.RUnlock()
	return map[string]interface{}{
		"refreshIntervalMs":  s.cfg.Panel.RefreshIntervalMs,
		"defaultDownloadDir": s.cfg.Panel.DefaultDownloadDir,
		"mcpEnabled":         s.cfg.Panel.MCPEnabled,
		"theme":              s.cfg.Panel.Theme,
		"colorMode":          s.cfg.Panel.ColorMode,
		"aria2Managed":       s.cfg.Aria2.Managed,
	}
}

func (s *Server) mcpTaskBucketResource(bucket string) (map[string]interface{}, error) {
	tasks, err := s.mcpFetchTasks(bucket, 0, 100)
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{
		"bucket": bucket,
		"tasks":  tasks,
	}, nil
}

func (s *Server) mcpTaskResource(gid string) (interface{}, error) {
	result, err := s.aria2.Call(Aria2CallRequest{
		Method: "aria2.tellStatus",
		Params: []interface{}{gid, mcpTaskFields},
	})
	if err != nil {
		return nil, errors.New(userFacingAria2Error(err))
	}
	return result, nil
}

func (s *Server) mcpOptionResource(key string) (map[string]interface{}, error) {
	options, err := s.aria2.Call(Aria2CallRequest{Method: "aria2.getGlobalOption"})
	if err != nil {
		return nil, errors.New(userFacingAria2Error(err))
	}
	optionMap, ok := options.(map[string]interface{})
	if !ok {
		return nil, errors.New("当前 aria2 配置读取失败，请稍后重试。")
	}
	value, ok := optionMap[key]
	if !ok {
		return nil, fmt.Errorf("未找到 aria2 配置项：%s", key)
	}
	return map[string]interface{}{
		"key":   key,
		"value": value,
	}, nil
}

func (s *Server) mcpFetchTasks(bucket string, offset, limit int) (interface{}, error) {
	switch bucket {
	case "active":
		return s.aria2.Call(Aria2CallRequest{
			Method: "aria2.tellActive",
			Params: []interface{}{mcpTaskFields},
		})
	case "waiting":
		return s.aria2.Call(Aria2CallRequest{
			Method: "aria2.tellWaiting",
			Params: []interface{}{offset, limit, mcpTaskFields},
		})
	case "stopped":
		return s.aria2.Call(Aria2CallRequest{
			Method: "aria2.tellStopped",
			Params: []interface{}{offset, limit, mcpTaskFields},
		})
	default:
		return nil, errors.New("bucket 仅支持 active、waiting、stopped。")
	}
}

func (s *Server) mcpTaskGIDs(bucket string) ([]string, error) {
	buckets := []string{bucket}
	if bucket == "all" {
		buckets = []string{"active", "waiting", "stopped"}
	}
	seen := map[string]struct{}{}
	gids := make([]string, 0, 16)
	for _, item := range buckets {
		raw, err := s.mcpFetchTasks(item, 0, 100)
		if err != nil {
			return nil, errors.New(userFacingAria2Error(err))
		}
		rows, ok := raw.([]interface{})
		if !ok {
			continue
		}
		for _, row := range rows {
			task, ok := row.(map[string]interface{})
			if !ok {
				continue
			}
			gid, _ := task["gid"].(string)
			if gid == "" {
				continue
			}
			if _, exists := seen[gid]; exists {
				continue
			}
			seen[gid] = struct{}{}
			gids = append(gids, gid)
		}
	}
	sort.Strings(gids)
	return gids, nil
}

func (s *Server) currentAria2Version() string {
	result, err := s.aria2.Call(Aria2CallRequest{Method: "aria2.getVersion"})
	if err != nil {
		return ""
	}
	payload, ok := result.(map[string]interface{})
	if !ok {
		return ""
	}
	versionText, _ := payload["version"].(string)
	return versionText
}

func (s *Server) mcpEmbeddedResource(uri string) (map[string]interface{}, error) {
	contents, err := s.readMCPResource(uri)
	if err != nil {
		return nil, err
	}
	if len(contents) == 0 {
		return nil, fmt.Errorf("资源为空：%s", uri)
	}
	return map[string]interface{}{
		"uri":      contents[0].URI,
		"mimeType": contents[0].MimeType,
		"text":     contents[0].Text,
	}, nil
}

func mcpToolList() []mcpTool {
	return []mcpTool{
		{
			Name:        "aria2_get_version",
			Description: "读取当前 aria2 版本信息。",
			InputSchema: objectSchema(nil, nil),
		},
		{
			Name:        "aria2_get_global_stat",
			Description: "读取当前下载、上传速度和任务数量。",
			InputSchema: objectSchema(nil, nil),
		},
		{
			Name:        "aria2_get_global_option",
			Description: "读取 aria2 当前全局配置。",
			InputSchema: objectSchema(nil, nil),
		},
		{
			Name:        "aria2_tell_active",
			Description: "读取活动任务列表。",
			InputSchema: objectSchema(map[string]interface{}{
				"keys": stringArraySchema("需要返回的字段列表。"),
			}, nil),
		},
		{
			Name:        "aria2_tell_waiting",
			Description: "读取等待中的任务列表。",
			InputSchema: objectSchema(map[string]interface{}{
				"offset": intSchema("起始偏移量。"),
				"limit":  intSchema("返回数量，默认 20。"),
				"keys":   stringArraySchema("需要返回的字段列表。"),
			}, nil),
		},
		{
			Name:        "aria2_tell_stopped",
			Description: "读取已停止或已完成的任务列表。",
			InputSchema: objectSchema(map[string]interface{}{
				"offset": intSchema("起始偏移量。"),
				"limit":  intSchema("返回数量，默认 20。"),
				"keys":   stringArraySchema("需要返回的字段列表。"),
			}, nil),
		},
		{
			Name:        "aria2_add_uri",
			Description: "添加一个或多个下载地址。",
			InputSchema: objectSchema(map[string]interface{}{
				"uris":     stringArraySchema("下载地址列表。"),
				"options":  stringMapSchema("可选的 aria2 参数覆盖。"),
				"position": intSchema("插入队列的位置。"),
			}, []string{"uris"}),
		},
		{
			Name:        "aria2_pause",
			Description: "暂停单个任务。",
			InputSchema: objectSchema(map[string]interface{}{
				"gid": stringSchema("任务 GID。"),
			}, []string{"gid"}),
		},
		{
			Name:        "aria2_unpause",
			Description: "继续单个任务。",
			InputSchema: objectSchema(map[string]interface{}{
				"gid": stringSchema("任务 GID。"),
			}, []string{"gid"}),
		},
		{
			Name:        "aria2_remove",
			Description: "移除单个任务。",
			InputSchema: objectSchema(map[string]interface{}{
				"gid": stringSchema("任务 GID。"),
			}, []string{"gid"}),
		},
		{
			Name:        "aria2_pause_all",
			Description: "暂停全部任务。",
			InputSchema: objectSchema(nil, nil),
		},
		{
			Name:        "aria2_unpause_all",
			Description: "继续全部任务。",
			InputSchema: objectSchema(nil, nil),
		},
		{
			Name:        "aria2_save_session",
			Description: "立即保存当前会话。",
			InputSchema: objectSchema(nil, nil),
		},
	}
}

func mcpResourceList() []mcpResource {
	return []mcpResource{
		{
			URI:         mcpResourceConnectionInfo,
			Name:        "连接信息",
			Description: "读取面板版本、aria2 版本、RPC 路径和当前面板 Secret。",
			MimeType:    "application/json",
		},
		{
			URI:         mcpResourcePanelConfig,
			Name:        "面板配置",
			Description: "读取当前面板刷新间隔、主题模式和默认下载目录。",
			MimeType:    "application/json",
		},
		{
			URI:         mcpResourceGlobalOption,
			Name:        "aria2 全局配置",
			Description: "读取当前 aria2 全局参数。",
			MimeType:    "application/json",
		},
		{
			URI:         mcpResourceGlobalStat,
			Name:        "全局统计",
			Description: "读取当前下载、上传速度与任务数量。",
			MimeType:    "application/json",
		},
		{
			URI:         mcpResourceTasksActive,
			Name:        "活动任务列表",
			Description: "读取所有活动中的任务。",
			MimeType:    "application/json",
		},
		{
			URI:         mcpResourceTasksWaiting,
			Name:        "等待任务列表",
			Description: "读取等待或暂停中的任务。",
			MimeType:    "application/json",
		},
		{
			URI:         mcpResourceTasksStopped,
			Name:        "停止任务列表",
			Description: "读取已停止、已完成或失败的任务。",
			MimeType:    "application/json",
		},
	}
}

func mcpResourceTemplateList() []mcpResourceTemplate {
	return []mcpResourceTemplate{
		{
			URITemplate: mcpTaskTemplate,
			Name:        "单任务详情",
			Description: "按 GID 读取单个 aria2 任务的完整状态。",
			MimeType:    "application/json",
		},
		{
			URITemplate: mcpTaskBucketTemplate,
			Name:        "任务分桶列表",
			Description: "按 active、waiting、stopped 读取任务列表。",
			MimeType:    "application/json",
		},
		{
			URITemplate: mcpOptionTemplate,
			Name:        "单个 aria2 配置项",
			Description: "按 key 读取一个 aria2 全局配置值。",
			MimeType:    "application/json",
		},
	}
}

func mcpPromptList() []mcpPrompt {
	return []mcpPrompt{
		{
			Name:        "ariamx_diagnose_task_failure",
			Title:       "诊断任务失败",
			Description: "读取单个任务详情并分析它为什么失败或停止。",
			Arguments: []mcpPromptArgument{
				{
					Name:        "gid",
					Description: "要诊断的任务 GID。",
					Required:    true,
				},
			},
		},
		{
			Name:        "ariamx_summarize_downloads",
			Title:       "总结下载队列",
			Description: "总结当前下载队列，并指出优先处理项。",
			Arguments: []mcpPromptArgument{
				{
					Name:        "bucket",
					Description: "all、active、waiting 或 stopped，默认 all。",
				},
			},
		},
		{
			Name:        "ariamx_explain_client_setup",
			Title:       "解释客户端连接方式",
			Description: "基于当前连接信息，说明第三方客户端如何接入 AriaMX RPC。",
			Arguments: []mcpPromptArgument{
				{
					Name:        "client",
					Description: "ariang、postman、curl 或 custom。",
				},
			},
		},
		{
			Name:        "ariamx_tune_global_options",
			Title:       "调优 aria2 全局配置",
			Description: "结合当前 aria2 配置给出调优建议。",
			Arguments: []mcpPromptArgument{
				{
					Name:        "goal",
					Description: "speed、stability、bt 或 seeding。",
				},
			},
		},
	}
}

func mcpListResponse[T any](raw json.RawMessage, field string, items []T) (map[string]interface{}, error) {
	cursor, err := mcpCursorArg(raw)
	if err != nil {
		return nil, err
	}
	page, nextCursor, err := paginateMCPItems(items, cursor)
	if err != nil {
		return nil, err
	}
	result := map[string]interface{}{
		field: page,
	}
	if nextCursor != "" {
		result["nextCursor"] = nextCursor
	}
	return result, nil
}

func mcpCursorArg(raw json.RawMessage) (string, error) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return "", nil
	}
	var payload struct {
		Cursor string `json:"cursor"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return "", errors.New("请检查 MCP 列表请求参数后重试。")
	}
	return strings.TrimSpace(payload.Cursor), nil
}

func paginateMCPItems[T any](items []T, cursor string) ([]T, string, error) {
	start := 0
	if cursor != "" {
		value, err := strconv.Atoi(cursor)
		if err != nil || value < 0 || value > len(items) {
			return nil, "", errors.New("MCP cursor 无效。")
		}
		start = value
	}
	if start == len(items) {
		return []T{}, "", nil
	}
	end := start + mcpDefaultPageSize
	if end > len(items) {
		end = len(items)
	}
	nextCursor := ""
	if end < len(items) {
		nextCursor = strconv.Itoa(end)
	}
	return items[start:end], nextCursor, nil
}

func mcpJSONContents(uri string, payload interface{}) ([]mcpResourceContent, error) {
	text, err := marshalMCPText(payload)
	if err != nil {
		return nil, errors.New("资源内容暂时无法编码，请稍后重试。")
	}
	return []mcpResourceContent{
		{
			URI:      uri,
			MimeType: "application/json",
			Text:     text,
		},
	}, nil
}

func mcpInvalidParams(message string) *mcpError {
	return &mcpError{
		Code:    -32602,
		Message: message,
	}
}

func objectSchema(properties map[string]interface{}, required []string) map[string]interface{} {
	if properties == nil {
		properties = map[string]interface{}{}
	}
	schema := map[string]interface{}{
		"type":                 "object",
		"properties":           properties,
		"additionalProperties": false,
	}
	if len(required) > 0 {
		schema["required"] = required
	}
	return schema
}

func stringSchema(description string) map[string]interface{} {
	return map[string]interface{}{
		"type":        "string",
		"description": description,
	}
}

func intSchema(description string) map[string]interface{} {
	return map[string]interface{}{
		"type":        "integer",
		"description": description,
	}
}

func stringArraySchema(description string) map[string]interface{} {
	return map[string]interface{}{
		"type":        "array",
		"description": description,
		"items": map[string]interface{}{
			"type": "string",
		},
	}
}

func stringMapSchema(description string) map[string]interface{} {
	return map[string]interface{}{
		"type":        "object",
		"description": description,
		"additionalProperties": map[string]interface{}{
			"type": "string",
		},
	}
}

func marshalMCPText(result interface{}) (string, error) {
	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func stringArg(arguments map[string]interface{}, key string) string {
	if arguments == nil {
		return ""
	}
	value, _ := arguments[key].(string)
	return value
}

func stringSliceArg(arguments map[string]interface{}, key string) []string {
	if arguments == nil {
		return nil
	}
	raw, ok := arguments[key].([]interface{})
	if !ok {
		return nil
	}
	items := make([]string, 0, len(raw))
	for _, item := range raw {
		text, ok := item.(string)
		if ok && text != "" {
			items = append(items, text)
		}
	}
	return items
}

func stringMapArg(arguments map[string]interface{}, key string) map[string]string {
	if arguments == nil {
		return nil
	}
	raw, ok := arguments[key].(map[string]interface{})
	if !ok {
		return nil
	}
	items := make(map[string]string, len(raw))
	for k, value := range raw {
		text, ok := value.(string)
		if ok {
			items[k] = text
		}
	}
	return items
}

func intArg(arguments map[string]interface{}, key string, fallback int) int {
	if value, ok := optionalIntArg(arguments, key); ok {
		return value
	}
	return fallback
}

func optionalIntArg(arguments map[string]interface{}, key string) (int, bool) {
	if arguments == nil {
		return 0, false
	}
	value, ok := arguments[key]
	if !ok {
		return 0, false
	}
	switch typed := value.(type) {
	case float64:
		return int(typed), true
	case int:
		return typed, true
	default:
		return 0, false
	}
}

func filterCompletionValues(values []string, prefix string) []string {
	prefix = strings.ToLower(strings.TrimSpace(prefix))
	filtered := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		if value == "" {
			continue
		}
		if prefix != "" && !strings.HasPrefix(strings.ToLower(value), prefix) {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		filtered = append(filtered, value)
		if len(filtered) >= 100 {
			break
		}
	}
	return filtered
}

func (s *Server) writeMCPResponse(w http.ResponseWriter, status int, payload mcpResponse) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
