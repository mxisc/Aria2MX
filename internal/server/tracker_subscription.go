package server

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const trackerSubscriptionSyncPeriod = 24 * time.Hour

type trackerSubscriptionSource struct {
	Key  string
	Name string
	URL  string
}

type trackerSubscriptionState struct {
	Enabled             bool   `json:"enabled"`
	SelectedSource      string `json:"selectedSource"`
	CurrentTrackerCount int    `json:"currentTrackerCount"`
	Message             string `json:"message,omitempty"`
}

var trackerSubscriptionSources = map[string]trackerSubscriptionSource{
	"ngosang-best": {
		Key:  "ngosang-best",
		Name: "ngosang trackers_best",
		URL:  "https://raw.githubusercontent.com/ngosang/trackerslist/master/trackers_best.txt",
	},
	"ngosang-all": {
		Key:  "ngosang-all",
		Name: "ngosang trackers_all",
		URL:  "https://raw.githubusercontent.com/ngosang/trackerslist/master/trackers_all.txt",
	},
	"newtrackon-stable": {
		Key:  "newtrackon-stable",
		Name: "newtrackon stable",
		URL:  "https://newtrackon.com/api/stable",
	},
	"adysec-best": {
		Key:  "adysec-best",
		Name: "adysec trackers_best",
		URL:  "https://raw.githubusercontent.com/adysec/tracker/main/trackers_best.txt",
	},
}

func isSupportedTrackerSubscriptionSource(key string) bool {
	_, ok := trackerSubscriptionSources[key]
	return ok
}

func (s *Server) startTrackerSubscriptionLoop() {
	s.trackerSubscriptionStop = make(chan struct{})
	s.trackerSubscriptionDone = make(chan struct{})
	go func() {
		ticker := time.NewTicker(trackerSubscriptionSyncPeriod)
		defer func() {
			ticker.Stop()
			close(s.trackerSubscriptionDone)
		}()
		for {
			select {
			case <-ticker.C:
				s.syncTrackerSubscription()
			case <-s.trackerSubscriptionStop:
				return
			}
		}
	}()
}

func (s *Server) stopTrackerSubscriptionLoop() {
	if s.trackerSubscriptionStop == nil {
		return
	}
	close(s.trackerSubscriptionStop)
	<-s.trackerSubscriptionDone
	s.trackerSubscriptionStop = nil
	s.trackerSubscriptionDone = nil
}

func (s *Server) handleTrackerSubscription(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		state := s.currentTrackerSubscriptionState("")
		writeJSON(w, http.StatusOK, apiResponse{OK: true, Data: state})
	case http.MethodPost:
		var payload struct {
			Enabled bool   `json:"enabled"`
			Source  string `json:"source"`
		}
		if err := readJSON(r, &payload); err != nil {
			writeAPIError(w, http.StatusBadRequest, "bad_request", "请检查节点订阅设置后重试。")
			return
		}
		source := strings.TrimSpace(payload.Source)
		if payload.Enabled && !isSupportedTrackerSubscriptionSource(source) {
			writeAPIError(w, http.StatusBadRequest, "bad_request", "请选择有效的节点订阅源。")
			return
		}

		var trackers []string
		if payload.Enabled {
			var err error
			trackers, err = fetchTrackerSubscriptionTrackers(source)
			if err != nil {
				writeAPIError(w, http.StatusBadGateway, "tracker_subscription_fetch_failed", err.Error())
				return
			}
		}

		s.cfgMu.Lock()
		previousEnabled := s.cfg.Panel.TrackerSubscriptionEnabled
		previousSource := s.cfg.Panel.TrackerSubscriptionSource
		s.cfg.Panel.TrackerSubscriptionEnabled = payload.Enabled
		s.cfg.Panel.TrackerSubscriptionSource = source
		err := SaveConfig(s.configPath, s.cfg)
		s.cfgMu.Unlock()
		if err != nil {
			writeAPIError(w, http.StatusInternalServerError, "save_failed", "节点订阅设置暂时无法保存，请稍后重试。")
			return
		}

		if !payload.Enabled {
			writeJSON(w, http.StatusOK, apiResponse{OK: true, Data: s.currentTrackerSubscriptionState("已关闭节点订阅自动同步，当前 bt-tracker 保持不变。")})
			return
		}

		if err := s.applyTrackerList(trackers); err != nil {
			s.cfgMu.Lock()
			s.cfg.Panel.TrackerSubscriptionEnabled = previousEnabled
			s.cfg.Panel.TrackerSubscriptionSource = previousSource
			rollbackErr := SaveConfig(s.configPath, s.cfg)
			s.cfgMu.Unlock()
			if rollbackErr != nil {
				log.Printf("tracker subscription rollback failed: %v", rollbackErr)
			}
			writeAPIError(w, http.StatusBadGateway, "tracker_subscription_apply_failed", err.Error())
			return
		}
		state := s.currentTrackerSubscriptionState(fmt.Sprintf("节点订阅已应用，当前写入 %d 条 tracker。", len(trackers)))
		writeJSON(w, http.StatusOK, apiResponse{OK: true, Data: state})
	default:
		methodNotAllowed(w)
	}
}

func (s *Server) syncTrackerSubscription() {
	s.cfgMu.RLock()
	enabled := s.cfg.Panel.TrackerSubscriptionEnabled
	source := s.cfg.Panel.TrackerSubscriptionSource
	s.cfgMu.RUnlock()
	if !enabled || source == "" {
		return
	}
	if _, err := s.applyTrackerSubscription(source); err != nil {
		log.Printf("tracker subscription apply skipped: %v", err)
	}
}

func (s *Server) currentTrackerSubscriptionState(message string) trackerSubscriptionState {
	s.cfgMu.RLock()
	state := trackerSubscriptionState{
		Enabled:        s.cfg.Panel.TrackerSubscriptionEnabled,
		SelectedSource: s.cfg.Panel.TrackerSubscriptionSource,
		Message:        message,
	}
	s.cfgMu.RUnlock()
	state.CurrentTrackerCount = s.currentBTTrackerCount()
	return state
}

func (s *Server) currentBTTrackerCount() int {
	result, err := s.aria2.Call(Aria2CallRequest{Method: "aria2.getGlobalOption"})
	if err != nil {
		return 0
	}
	options, ok := result.(map[string]interface{})
	if !ok {
		return 0
	}
	value, _ := options["bt-tracker"].(string)
	return len(splitManagedOptionList(value))
}

func (s *Server) applyTrackerSubscription(sourceKey string) (int, error) {
	trackers, err := fetchTrackerSubscriptionTrackers(sourceKey)
	if err != nil {
		return 0, err
	}
	if err := s.applyTrackerList(trackers); err != nil {
		return 0, err
	}
	return len(trackers), nil
}

func (s *Server) applyTrackerList(trackers []string) error {
	joined := strings.Join(trackers, "\n")
	if s.managed != nil {
		if _, err := s.managed.SaveOptions(map[string]string{"bt-tracker": joined}); err != nil {
			return fmt.Errorf("节点订阅应用失败，请稍后重试。")
		}
		return nil
	}
	if _, err := s.aria2.Call(Aria2CallRequest{
		Method: "aria2.changeGlobalOption",
		Params: []interface{}{map[string]string{"bt-tracker": strings.Join(trackers, ",")}},
	}); err != nil {
		return fmt.Errorf("节点订阅已保存，但 aria2 暂时无法应用该订阅源。")
	}
	return nil
}

func fetchTrackerSubscriptionTrackers(sourceKey string) ([]string, error) {
	source, ok := trackerSubscriptionSources[sourceKey]
	if !ok {
		return nil, fmt.Errorf("无效的节点订阅源。")
	}
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Get(source.URL)
	if err != nil {
		return nil, fmt.Errorf("节点订阅源暂时无法访问，请稍后重试。")
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("节点订阅源返回异常状态码 %d。", resp.StatusCode)
	}
	trackers, err := parseTrackerSubscriptionList(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	if len(trackers) == 0 {
		return nil, fmt.Errorf("订阅源里没有可用的 Tracker 地址。")
	}
	return trackers, nil
}

func parseTrackerSubscriptionList(r io.Reader) ([]string, error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	items := make([]string, 0, 128)
	seen := map[string]struct{}{}
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parsed, err := url.Parse(line)
		if err != nil {
			continue
		}
		switch strings.ToLower(parsed.Scheme) {
		case "udp", "http", "https":
		default:
			continue
		}
		if _, ok := seen[line]; ok {
			continue
		}
		seen[line] = struct{}{}
		items = append(items, line)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("节点订阅内容读取失败，请稍后重试。")
	}
	return items, nil
}
