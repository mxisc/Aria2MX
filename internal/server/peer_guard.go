package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

const (
	peerGuardPFAnchor      = "com.apple/250.Aria2MX"
	peerGuardNFTTableName  = "aria2mx_peer_guard"
	peerGuardIPTChainName  = "ARIA2MX_PEER_GUARD"
	peerGuardAutoBanPeriod = 30 * time.Second
	peerGuardBanDuration   = 30 * time.Minute
	peerGuardAutoBanReason = "持续从本机获取数据却不回传"
	defaultAutoBanMinScore = 3
)

var (
	execCommand = exec.Command
	lookPath    = exec.LookPath
	runtimeGOOS = runtime.GOOS
)

type peerGuardRuntime struct {
	mu          sync.RWMutex
	applyMu     sync.Mutex
	lastError   string
	lastApplied time.Time
}

type peerGuardSnapshot struct {
	FirewallMode        string                   `json:"firewallMode"`
	FirewallReady       bool                     `json:"firewallReady"`
	FirewallOperable    bool                     `json:"firewallOperable"`
	ActionBlockedReason string                   `json:"actionBlockedReason,omitempty"`
	LastError           string                   `json:"lastError,omitempty"`
	LastAppliedAt       string                   `json:"lastAppliedAt,omitempty"`
	AutoBanEnabled      bool                     `json:"autoBanEnabled"`
	AutoBanMinScore     int                      `json:"autoBanMinScore"`
	BlockedPeers        []PeerBanRecord          `json:"blockedPeers"`
	Suspicious          []suspiciousPeerSnapshot `json:"suspiciousPeers"`
}

type suspiciousPeerSnapshot struct {
	GID           string `json:"gid"`
	TaskName      string `json:"taskName"`
	IP            string `json:"ip"`
	Port          string `json:"port"`
	DownloadSpeed string `json:"downloadSpeed"`
	UploadSpeed   string `json:"uploadSpeed"`
	Seeder        bool   `json:"seeder"`
	Blocked       bool   `json:"blocked"`
	Score         int    `json:"score"`
	Reason        string `json:"reason"`
}

type peerGuardFirewallState struct {
	Mode                string
	Ready               bool
	Operable            bool
	ActionBlockedReason string
}

func (s *Server) startPeerGuardLoop() {
	s.peerGuardStop = make(chan struct{})
	s.peerGuardDone = make(chan struct{})
	go func() {
		ticker := time.NewTicker(peerGuardAutoBanPeriod)
		defer func() {
			ticker.Stop()
			close(s.peerGuardDone)
		}()
		for {
			select {
			case <-ticker.C:
				s.pruneExpiredPeerBans()
				s.runPeerGuardAutoBanSweep()
			case <-s.peerGuardStop:
				return
			}
		}
	}()
}

func (s *Server) stopPeerGuardLoop() {
	if s.peerGuardStop == nil {
		return
	}
	close(s.peerGuardStop)
	<-s.peerGuardDone
	s.peerGuardStop = nil
	s.peerGuardDone = nil
}

func (s *Server) handlePeerGuard(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	snapshot, err := s.peerGuardSnapshot()
	if err != nil {
		writeAPIError(w, http.StatusBadGateway, "peer_guard_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, apiResponse{OK: true, Data: snapshot})
}

func (s *Server) handlePeerGuardSettings(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	var payload struct {
		AutoBanEnabled *bool `json:"autoBanEnabled"`
	}
	if err := readJSON(r, &payload); err != nil || payload.AutoBanEnabled == nil {
		writeAPIError(w, http.StatusBadRequest, "bad_request", "请检查自动封禁设置后重试。")
		return
	}
	if err := s.updatePeerGuardSettings(*payload.AutoBanEnabled); err != nil {
		writeAPIError(w, http.StatusBadGateway, "peer_guard_settings_failed", err.Error())
		return
	}
	if *payload.AutoBanEnabled {
		s.runPeerGuardAutoBanSweep()
	}
	snapshot, err := s.peerGuardSnapshot()
	if err != nil {
		writeAPIError(w, http.StatusBadGateway, "peer_guard_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, apiResponse{OK: true, Data: snapshot})
}

func (s *Server) handlePeerGuardBan(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	var payload struct {
		IP     string `json:"ip"`
		Reason string `json:"reason"`
	}
	if err := readJSON(r, &payload); err != nil {
		writeAPIError(w, http.StatusBadRequest, "bad_request", "请检查节点地址后重试。")
		return
	}
	snapshot, err := s.banPeer(strings.TrimSpace(payload.IP), strings.TrimSpace(payload.Reason))
	if err != nil {
		writeAPIError(w, http.StatusBadGateway, "peer_guard_ban_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, apiResponse{OK: true, Data: snapshot})
}

func (s *Server) handlePeerGuardUnban(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	var payload struct {
		IP string `json:"ip"`
	}
	if err := readJSON(r, &payload); err != nil {
		writeAPIError(w, http.StatusBadRequest, "bad_request", "请检查节点地址后重试。")
		return
	}
	snapshot, err := s.unbanPeer(strings.TrimSpace(payload.IP))
	if err != nil {
		writeAPIError(w, http.StatusBadGateway, "peer_guard_unban_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, apiResponse{OK: true, Data: snapshot})
}

func (s *Server) peerGuardSnapshot() (peerGuardSnapshot, error) {
	s.pruneExpiredPeerBans()
	s.cfgMu.RLock()
	blocked := append([]PeerBanRecord(nil), s.cfg.PeerGuard.BlockedPeers...)
	autoBanEnabled := s.cfg.PeerGuard.AutoBanEnabled
	autoBanMinScore := s.cfg.PeerGuard.AutoBanMinScore
	s.cfgMu.RUnlock()
	if blocked == nil {
		blocked = []PeerBanRecord{}
	}
	if autoBanMinScore <= 0 {
		autoBanMinScore = defaultAutoBanMinScore
	}

	firewall := detectPeerGuardFirewallState()
	suspicious, err := s.scanSuspiciousPeers(blocked)
	if err != nil {
		return peerGuardSnapshot{}, err
	}
	if suspicious == nil {
		suspicious = []suspiciousPeerSnapshot{}
	}
	lastError, lastApplied := s.peerGuardStatus()
	return peerGuardSnapshot{
		FirewallMode:        firewall.Mode,
		FirewallReady:       firewall.Ready,
		FirewallOperable:    firewall.Operable,
		ActionBlockedReason: firewall.ActionBlockedReason,
		LastError:           lastError,
		LastAppliedAt:       formatPeerGuardTime(lastApplied),
		AutoBanEnabled:      autoBanEnabled,
		AutoBanMinScore:     autoBanMinScore,
		BlockedPeers:        blocked,
		Suspicious:          suspicious,
	}, nil
}

func (s *Server) updatePeerGuardSettings(autoBanEnabled bool) error {
	s.cfgMu.Lock()
	previous := s.cfg.PeerGuard.AutoBanEnabled
	s.cfg.PeerGuard.AutoBanEnabled = autoBanEnabled
	if err := SaveConfig(s.configPath, s.cfg); err != nil {
		s.cfg.PeerGuard.AutoBanEnabled = previous
		s.cfgMu.Unlock()
		return errors.New("自动封禁设置暂时无法保存，请稍后重试。")
	}
	s.cfgMu.Unlock()
	return nil
}

func (s *Server) banPeer(ip, reason string) (peerGuardSnapshot, error) {
	ip, err := normalizePeerIP(ip)
	if err != nil {
		return peerGuardSnapshot{}, errors.New("请提供有效的节点 IP。")
	}
	firewall := detectPeerGuardFirewallState()
	if !firewall.Operable {
		return peerGuardSnapshot{}, errors.New(firewall.ActionBlockedReason)
	}

	s.cfgMu.RLock()
	current := append([]PeerBanRecord(nil), s.cfg.PeerGuard.BlockedPeers...)
	s.cfgMu.RUnlock()
	for _, record := range current {
		if record.IP == ip {
			return s.peerGuardSnapshot()
		}
	}
	now := time.Now()
	next := append(current, PeerBanRecord{
		IP:        ip,
		Reason:    reason,
		CreatedAt: formatPeerGuardTime(now),
	})
	if err := s.updatePeerGuardPeers(next); err != nil {
		return peerGuardSnapshot{}, err
	}
	return s.peerGuardSnapshot()
}

func (s *Server) unbanPeer(ip string) (peerGuardSnapshot, error) {
	ip, err := normalizePeerIP(ip)
	if err != nil {
		return peerGuardSnapshot{}, errors.New("请提供有效的节点 IP。")
	}
	firewall := detectPeerGuardFirewallState()
	if !firewall.Operable {
		return peerGuardSnapshot{}, errors.New(firewall.ActionBlockedReason)
	}

	s.cfgMu.RLock()
	current := append([]PeerBanRecord(nil), s.cfg.PeerGuard.BlockedPeers...)
	s.cfgMu.RUnlock()
	next := make([]PeerBanRecord, 0, len(current))
	for _, record := range current {
		if record.IP != ip {
			next = append(next, record)
		}
	}
	if len(next) == len(current) {
		return s.peerGuardSnapshot()
	}
	if err := s.updatePeerGuardPeers(next); err != nil {
		return peerGuardSnapshot{}, err
	}
	return s.peerGuardSnapshot()
}

func (s *Server) updatePeerGuardPeers(next []PeerBanRecord) error {
	next = normalizePeerBanRecords(next)

	s.peerGuard.applyMu.Lock()
	defer s.peerGuard.applyMu.Unlock()

	s.cfgMu.Lock()
	previous := append([]PeerBanRecord(nil), s.cfg.PeerGuard.BlockedPeers...)
	s.cfg.PeerGuard.BlockedPeers = next
	if err := SaveConfig(s.configPath, s.cfg); err != nil {
		s.cfg.PeerGuard.BlockedPeers = previous
		s.cfgMu.Unlock()
		return errors.New("封禁列表暂时无法保存，请稍后重试。")
	}
	s.cfgMu.Unlock()

	if err := s.applyPeerGuardFirewall(previous, next); err != nil {
		s.cfgMu.Lock()
		s.cfg.PeerGuard.BlockedPeers = previous
		_ = SaveConfig(s.configPath, s.cfg)
		s.cfgMu.Unlock()
		_ = s.applyPeerGuardFirewall(next, previous)
		return err
	}
	return nil
}

func (s *Server) runPeerGuardAutoBanSweep() {
	s.cfgMu.RLock()
	autoBanEnabled := s.cfg.PeerGuard.AutoBanEnabled
	autoBanMinScore := s.cfg.PeerGuard.AutoBanMinScore
	current := append([]PeerBanRecord(nil), s.cfg.PeerGuard.BlockedPeers...)
	s.cfgMu.RUnlock()
	if !autoBanEnabled {
		return
	}
	if autoBanMinScore <= 0 {
		autoBanMinScore = defaultAutoBanMinScore
	}

	firewall := detectPeerGuardFirewallState()
	if !firewall.Operable {
		s.setPeerGuardStatus(firewall.ActionBlockedReason, time.Time{})
		return
	}

	suspicious, err := s.scanSuspiciousPeers(current)
	if err != nil {
		return
	}
	blockedSet := map[string]struct{}{}
	for _, record := range current {
		blockedSet[record.IP] = struct{}{}
	}
	next := append([]PeerBanRecord(nil), current...)
	for _, peer := range suspicious {
		if peer.Score < autoBanMinScore {
			continue
		}
		if _, ok := blockedSet[peer.IP]; ok {
			continue
		}
		now := time.Now()
		next = append(next, PeerBanRecord{
			IP:        peer.IP,
			Reason:    fmt.Sprintf("自动封禁：评分 %d 分，%s", peer.Score, peer.Reason),
			CreatedAt: formatPeerGuardTime(now),
			ExpiresAt: formatPeerGuardTime(peerGuardBanExpiresAt(now)),
		})
		blockedSet[peer.IP] = struct{}{}
	}
	if len(next) == len(current) {
		return
	}
	_ = s.updatePeerGuardPeers(next)
}

func (s *Server) pruneExpiredPeerBans() {
	now := time.Now()
	s.cfgMu.RLock()
	current := append([]PeerBanRecord(nil), s.cfg.PeerGuard.BlockedPeers...)
	s.cfgMu.RUnlock()
	if len(current) == 0 {
		return
	}
	next := make([]PeerBanRecord, 0, len(current))
	for _, record := range current {
		if peerBanExpired(record, now) {
			continue
		}
		next = append(next, record)
	}
	if len(next) == len(current) {
		return
	}
	if err := s.updatePeerGuardPeers(next); err != nil {
		s.setPeerGuardStatus(err.Error(), time.Time{})
	}
}

func (s *Server) applyPeerGuardFirewall(previous, peers []PeerBanRecord) error {
	firewall := detectPeerGuardFirewallState()
	if !firewall.Ready || !firewall.Operable {
		s.setPeerGuardStatus(firewall.ActionBlockedReason, time.Time{})
		return errors.New(firewall.ActionBlockedReason)
	}

	var err error
	switch firewall.Mode {
	case "pf":
		err = s.applyPFFirewall(peers)
	case "firewalld":
		err = s.applyFirewalldFirewall(previous, peers)
	case "ufw":
		err = s.applyUFWFirewall(previous, peers)
	case "nft":
		err = s.applyNFTFirewall(peers)
	case "iptables":
		err = s.applyIPTablesFirewall(peers)
	default:
		err = errors.New("当前系统暂不支持自动封禁节点。")
	}
	if err != nil {
		s.setPeerGuardStatus(err.Error(), time.Time{})
		return err
	}
	s.setPeerGuardStatus("", time.Now())
	return nil
}

func detectPeerGuardFirewallState() peerGuardFirewallState {
	switch runtimeGOOS {
	case "darwin":
		return detectPFFirewallState()
	case "linux":
		return detectLinuxFirewallState()
	default:
		return peerGuardFirewallState{
			Mode:                "unsupported",
			Ready:               false,
			Operable:            false,
			ActionBlockedReason: "当前系统暂不支持自动封禁节点。",
		}
	}
}

func detectLinuxFirewallState() peerGuardFirewallState {
	if state := detectFirewalldFirewallState(); state.Operable {
		return state
	}
	if state := detectUFFirewallState(); state.Operable {
		return state
	}
	if state := detectNFTFirewallState(); state.Operable {
		return state
	}
	if state := detectIPTablesFirewallState(); state.Operable {
		return state
	}
	if state := detectFirewalldFirewallState(); state.Ready {
		return state
	}
	if state := detectUFFirewallState(); state.Ready {
		return state
	}
	if state := detectNFTFirewallState(); state.Ready {
		return state
	}
	if state := detectIPTablesFirewallState(); state.Ready {
		return state
	}
	return peerGuardFirewallState{
		Mode:                "unsupported",
		Ready:               false,
		Operable:            false,
		ActionBlockedReason: "当前 Linux 环境未找到可用的系统防火墙命令，暂时无法封禁节点。",
	}
}

func detectFirewalldFirewallState() peerGuardFirewallState {
	firewallCmdPath, err := lookPath("firewall-cmd")
	if err != nil {
		return peerGuardFirewallState{Mode: "firewalld"}
	}
	output, cmdErr := runFirewallCommand(firewallCmdPath, "--state")
	if cmdErr != nil {
		if strings.Contains(strings.ToLower(output), "not running") {
			return peerGuardFirewallState{Mode: "firewalld"}
		}
		return peerGuardFirewallState{
			Mode:                "firewalld",
			Ready:               true,
			Operable:            false,
			ActionBlockedReason: "当前进程没有管理 firewalld 的权限，请使用具备 root 权限的方式运行 Aria2MX。",
		}
	}
	if strings.TrimSpace(output) != "running" {
		return peerGuardFirewallState{Mode: "firewalld"}
	}
	return peerGuardFirewallState{
		Mode:     "firewalld",
		Ready:    true,
		Operable: true,
	}
}

func detectUFFirewallState() peerGuardFirewallState {
	ufwPath, err := lookPath("ufw")
	if err != nil {
		return peerGuardFirewallState{Mode: "ufw"}
	}
	output, cmdErr := runFirewallCommand(ufwPath, "status")
	if cmdErr != nil {
		return peerGuardFirewallState{
			Mode:                "ufw",
			Ready:               true,
			Operable:            false,
			ActionBlockedReason: "当前进程没有管理 ufw 的权限，请使用具备 root 权限的方式运行 Aria2MX。",
		}
	}
	if !strings.Contains(strings.ToLower(output), "status: active") {
		return peerGuardFirewallState{Mode: "ufw"}
	}
	return peerGuardFirewallState{
		Mode:     "ufw",
		Ready:    true,
		Operable: true,
	}
}

func detectPFFirewallState() peerGuardFirewallState {
	if _, err := lookPath("pfctl"); err != nil {
		return peerGuardFirewallState{
			Mode:                "pf",
			Ready:               false,
			Operable:            false,
			ActionBlockedReason: "当前系统未找到 pf 防火墙命令，暂时无法封禁节点。",
		}
	}
	if _, err := runFirewallCommand("/sbin/pfctl", "-s", "info"); err != nil {
		return peerGuardFirewallState{
			Mode:                "pf",
			Ready:               true,
			Operable:            false,
			ActionBlockedReason: "当前进程没有管理 pf 防火墙的权限，请使用具备管理员权限的方式运行 Aria2MX。",
		}
	}
	return peerGuardFirewallState{
		Mode:     "pf",
		Ready:    true,
		Operable: true,
	}
}

func detectNFTFirewallState() peerGuardFirewallState {
	nftPath, err := lookPath("nft")
	if err != nil {
		return peerGuardFirewallState{
			Mode:                "nft",
			Ready:               false,
			Operable:            false,
			ActionBlockedReason: "当前 Linux 环境未找到 nft 防火墙命令，暂时无法封禁节点。",
		}
	}
	if _, err := runFirewallCommand(nftPath, "list", "ruleset"); err != nil {
		return peerGuardFirewallState{
			Mode:                "nft",
			Ready:               true,
			Operable:            false,
			ActionBlockedReason: "当前进程没有管理 nft 防火墙的权限，请使用具备 root 权限的方式运行 Aria2MX。",
		}
	}
	return peerGuardFirewallState{
		Mode:     "nft",
		Ready:    true,
		Operable: true,
	}
}

func detectIPTablesFirewallState() peerGuardFirewallState {
	iptablesPath, iptablesErr := lookPath("iptables")
	ip6tablesPath, ip6tablesErr := lookPath("ip6tables")
	if iptablesErr != nil && ip6tablesErr != nil {
		return peerGuardFirewallState{
			Mode:                "iptables",
			Ready:               false,
			Operable:            false,
			ActionBlockedReason: "当前 Linux 环境未找到 iptables 或 ip6tables，暂时无法封禁节点。",
		}
	}
	if iptablesErr == nil {
		if _, err := runFirewallCommand(iptablesPath, "-S"); err != nil {
			return peerGuardFirewallState{
				Mode:                "iptables",
				Ready:               true,
				Operable:            false,
				ActionBlockedReason: "当前进程没有管理 iptables 防火墙的权限，请使用具备 root 权限的方式运行 Aria2MX。",
			}
		}
	}
	if ip6tablesErr == nil {
		if _, err := runFirewallCommand(ip6tablesPath, "-S"); err != nil {
			return peerGuardFirewallState{
				Mode:                "iptables",
				Ready:               true,
				Operable:            false,
				ActionBlockedReason: "当前进程没有管理 iptables 防火墙的权限，请使用具备 root 权限的方式运行 Aria2MX。",
			}
		}
	}
	return peerGuardFirewallState{
		Mode:     "iptables",
		Ready:    true,
		Operable: true,
	}
}

func (s *Server) applyPFFirewall(peers []PeerBanRecord) error {
	rulesPath, err := s.writePFRules(peers)
	if err != nil {
		return errors.New("封禁规则写入失败，请稍后重试。")
	}
	if _, err := runFirewallCommand("/sbin/pfctl", "-E"); err != nil {
		return errors.New("系统防火墙未能启用，节点尚未封禁。请用具备权限的方式运行 Aria2MX 后重试。")
	}
	if _, err := runFirewallCommand("/sbin/pfctl", "-a", peerGuardPFAnchor, "-f", rulesPath); err != nil {
		return errors.New("系统防火墙规则应用失败，节点尚未封禁。请用具备权限的方式运行 Aria2MX 后重试。")
	}
	return nil
}

func (s *Server) applyFirewalldFirewall(previous, peers []PeerBanRecord) error {
	firewallCmdPath, err := lookPath("firewall-cmd")
	if err != nil {
		return errors.New("当前 Linux 环境未找到 firewalld 命令，暂时无法封禁节点。")
	}
	previousSet := peerGuardRecordSet(previous)
	currentSet := peerGuardRecordSet(peers)
	for ip := range previousSet {
		if _, ok := currentSet[ip]; ok {
			continue
		}
		_ = firewalldDeleteRule(firewallCmdPath, ip)
	}
	for ip := range currentSet {
		if _, ok := previousSet[ip]; ok {
			continue
		}
		if err := firewalldAddRule(firewallCmdPath, ip); err != nil {
			return errors.New("firewalld 规则应用失败，节点尚未封禁。请用具备 root 权限的方式运行 Aria2MX 后重试。")
		}
	}
	return nil
}

func firewalldAddRule(binary, ip string) error {
	for _, rule := range firewalldRulesForIP(ip) {
		if _, err := runFirewallCommand(binary, "--quiet", "--add-rich-rule="+rule); err != nil {
			return err
		}
	}
	return nil
}

func firewalldDeleteRule(binary, ip string) error {
	for _, rule := range firewalldRulesForIP(ip) {
		output, err := runFirewallCommand(binary, "--quiet", "--remove-rich-rule="+rule)
		if err != nil && !strings.Contains(strings.ToUpper(output), "NOT_ENABLED") && !strings.Contains(strings.ToLower(output), "not enabled") {
			return err
		}
	}
	return nil
}

func firewalldRulesForIP(ip string) []string {
	family := "ipv4"
	if strings.Contains(ip, ":") {
		family = "ipv6"
	}
	return []string{
		fmt.Sprintf(`rule family="%s" source address="%s" drop`, family, ip),
		fmt.Sprintf(`rule family="%s" destination address="%s" drop`, family, ip),
	}
}

func (s *Server) applyUFWFirewall(previous, peers []PeerBanRecord) error {
	ufwPath, err := lookPath("ufw")
	if err != nil {
		return errors.New("当前 Linux 环境未找到 ufw 命令，暂时无法封禁节点。")
	}
	previousSet := peerGuardRecordSet(previous)
	currentSet := peerGuardRecordSet(peers)
	for ip := range previousSet {
		if _, ok := currentSet[ip]; ok {
			continue
		}
		_ = ufwDeleteRule(ufwPath, ip)
	}
	for ip := range currentSet {
		if _, ok := previousSet[ip]; ok {
			continue
		}
		if err := ufwAddRule(ufwPath, ip); err != nil {
			return errors.New("ufw 规则应用失败，节点尚未封禁。请用具备 root 权限的方式运行 Aria2MX 后重试。")
		}
	}
	return nil
}

func ufwAddRule(binary, ip string) error {
	for _, args := range [][]string{
		{"--force", "deny", "from", ip},
		{"--force", "deny", "out", "to", ip},
	} {
		if _, err := runFirewallCommand(binary, args...); err != nil {
			return err
		}
	}
	return nil
}

func ufwDeleteRule(binary, ip string) error {
	for _, args := range [][]string{
		{"--force", "delete", "deny", "from", ip},
		{"--force", "delete", "deny", "out", "to", ip},
	} {
		output, err := runFirewallCommand(binary, args...)
		if err != nil && !strings.Contains(strings.ToLower(output), "could not find") && !strings.Contains(strings.ToLower(output), "could not delete") {
			return err
		}
	}
	return nil
}

func (s *Server) applyNFTFirewall(peers []PeerBanRecord) error {
	nftPath, err := lookPath("nft")
	if err != nil {
		return errors.New("当前 Linux 环境未找到 nft 防火墙命令，暂时无法封禁节点。")
	}
	rulesPath, err := s.writeNFTRules(peers)
	if err != nil {
		return errors.New("封禁规则写入失败，请稍后重试。")
	}
	_, _ = runFirewallCommand(nftPath, "delete", "table", "inet", peerGuardNFTTableName)
	if _, err := runFirewallCommand(nftPath, "-f", rulesPath); err != nil {
		return errors.New("系统防火墙规则应用失败，节点尚未封禁。请用具备权限的方式运行 Aria2MX 后重试。")
	}
	return nil
}

func (s *Server) applyIPTablesFirewall(peers []PeerBanRecord) error {
	iptablesPath, iptablesErr := lookPath("iptables")
	ip6tablesPath, ip6tablesErr := lookPath("ip6tables")
	if iptablesErr != nil && ip6tablesErr != nil {
		return errors.New("当前 Linux 环境未找到 iptables 或 ip6tables，暂时无法封禁节点。")
	}

	ipv4 := make([]string, 0)
	ipv6 := make([]string, 0)
	for _, peer := range peers {
		if strings.Contains(peer.IP, ":") {
			ipv6 = append(ipv6, peer.IP)
		} else {
			ipv4 = append(ipv4, peer.IP)
		}
	}

	if iptablesErr == nil {
		if err := syncIPTablesChain(iptablesPath, ipv4); err != nil {
			return errors.New("iptables 规则应用失败，节点尚未封禁。请用具备 root 权限的方式运行 Aria2MX 后重试。")
		}
	}
	if ip6tablesErr == nil {
		if err := syncIPTablesChain(ip6tablesPath, ipv6); err != nil {
			return errors.New("ip6tables 规则应用失败，节点尚未封禁。请用具备 root 权限的方式运行 Aria2MX 后重试。")
		}
	}
	return nil
}

func syncIPTablesChain(binary string, peers []string) error {
	_, _ = runFirewallCommand(binary, "-N", peerGuardIPTChainName)
	_, _ = runFirewallCommand(binary, "-F", peerGuardIPTChainName)

	if _, err := runFirewallCommand(binary, "-C", "INPUT", "-j", peerGuardIPTChainName); err != nil {
		if _, err := runFirewallCommand(binary, "-I", "INPUT", "-j", peerGuardIPTChainName); err != nil {
			return err
		}
	}
	if _, err := runFirewallCommand(binary, "-C", "OUTPUT", "-j", peerGuardIPTChainName); err != nil {
		if _, err := runFirewallCommand(binary, "-I", "OUTPUT", "-j", peerGuardIPTChainName); err != nil {
			return err
		}
	}

	for _, peer := range peers {
		if _, err := runFirewallCommand(binary, "-A", peerGuardIPTChainName, "-s", peer, "-j", "DROP"); err != nil {
			return err
		}
		if _, err := runFirewallCommand(binary, "-A", peerGuardIPTChainName, "-d", peer, "-j", "DROP"); err != nil {
			return err
		}
	}
	return nil
}

func peerGuardRecordSet(records []PeerBanRecord) map[string]struct{} {
	result := make(map[string]struct{}, len(records))
	for _, record := range records {
		result[record.IP] = struct{}{}
	}
	return result
}

func runFirewallCommand(binary string, args ...string) (string, error) {
	cmd := execCommand(binary, args...)
	output, err := cmd.CombinedOutput()
	text := strings.TrimSpace(string(output))
	if err != nil {
		if text == "" {
			text = err.Error()
		}
		return text, fmt.Errorf("%s", text)
	}
	return text, nil
}

func (s *Server) writePFRules(peers []PeerBanRecord) (string, error) {
	rulesPath := filepath.Join(filepath.Dir(s.configPath), "aria2mx-data", "peer-guard", "pf.conf")
	if err := os.MkdirAll(filepath.Dir(rulesPath), 0o755); err != nil {
		return "", err
	}
	builder := strings.Builder{}
	builder.WriteString("table <aria2mx_peer_guard> persist")
	if len(peers) > 0 {
		builder.WriteString(" { ")
		for index, peer := range peers {
			if index > 0 {
				builder.WriteString(", ")
			}
			builder.WriteString(peer.IP)
		}
		builder.WriteString(" }")
	}
	builder.WriteString("\n")
	builder.WriteString("block drop quick from <aria2mx_peer_guard> to any\n")
	builder.WriteString("block drop quick to <aria2mx_peer_guard> from any\n")
	if err := os.WriteFile(rulesPath, []byte(builder.String()), 0o600); err != nil {
		return "", err
	}
	return rulesPath, nil
}

func (s *Server) writeNFTRules(peers []PeerBanRecord) (string, error) {
	rulesPath := filepath.Join(filepath.Dir(s.configPath), "aria2mx-data", "peer-guard", "nft.conf")
	if err := os.MkdirAll(filepath.Dir(rulesPath), 0o755); err != nil {
		return "", err
	}
	ipv4 := make([]string, 0)
	ipv6 := make([]string, 0)
	for _, peer := range peers {
		if strings.Contains(peer.IP, ":") {
			ipv6 = append(ipv6, peer.IP)
		} else {
			ipv4 = append(ipv4, peer.IP)
		}
	}
	builder := strings.Builder{}
	builder.WriteString(fmt.Sprintf("table inet %s {\n", peerGuardNFTTableName))
	builder.WriteString(buildNFTSet("blocked_v4", "ipv4_addr", ipv4))
	builder.WriteString(buildNFTSet("blocked_v6", "ipv6_addr", ipv6))
	builder.WriteString("  chain input {\n")
	builder.WriteString("    type filter hook input priority 0; policy accept;\n")
	builder.WriteString("    ip saddr @blocked_v4 drop\n")
	builder.WriteString("    ip daddr @blocked_v4 drop\n")
	builder.WriteString("    ip6 saddr @blocked_v6 drop\n")
	builder.WriteString("    ip6 daddr @blocked_v6 drop\n")
	builder.WriteString("  }\n")
	builder.WriteString("  chain output {\n")
	builder.WriteString("    type filter hook output priority 0; policy accept;\n")
	builder.WriteString("    ip saddr @blocked_v4 drop\n")
	builder.WriteString("    ip daddr @blocked_v4 drop\n")
	builder.WriteString("    ip6 saddr @blocked_v6 drop\n")
	builder.WriteString("    ip6 daddr @blocked_v6 drop\n")
	builder.WriteString("  }\n")
	builder.WriteString("}\n")
	if err := os.WriteFile(rulesPath, []byte(builder.String()), 0o600); err != nil {
		return "", err
	}
	return rulesPath, nil
}

func buildNFTSet(name, valueType string, values []string) string {
	builder := strings.Builder{}
	builder.WriteString(fmt.Sprintf("  set %s {\n", name))
	builder.WriteString(fmt.Sprintf("    type %s\n", valueType))
	builder.WriteString("    flags interval\n")
	if len(values) > 0 {
		builder.WriteString("    elements = { ")
		for index, value := range values {
			if index > 0 {
				builder.WriteString(", ")
			}
			builder.WriteString(value)
		}
		builder.WriteString(" }\n")
	}
	builder.WriteString("  }\n")
	return builder.String()
}

func normalizePeerIP(value string) (string, error) {
	ip := net.ParseIP(strings.TrimSpace(value))
	if ip == nil {
		return "", errors.New("invalid ip")
	}
	return ip.String(), nil
}

func (s *Server) scanSuspiciousPeers(blocked []PeerBanRecord) ([]suspiciousPeerSnapshot, error) {
	result, err := s.aria2.Call(Aria2CallRequest{
		Method: "aria2.tellActive",
		Params: []interface{}{[]string{"gid", "bittorrent", "files"}},
	})
	if err != nil {
		return nil, errors.New(userFacingAria2Error(err))
	}
	taskList, ok := result.([]interface{})
	if !ok {
		return nil, errors.New("当前任务列表读取失败，请稍后重试。")
	}

	blockedSet := map[string]struct{}{}
	for _, record := range blocked {
		blockedSet[record.IP] = struct{}{}
	}
	suspicious := make([]suspiciousPeerSnapshot, 0)
	for _, item := range taskList {
		task, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		gid, _ := task["gid"].(string)
		if gid == "" {
			continue
		}
		taskName := peerGuardTaskName(task)
		peersRaw, err := s.aria2.Call(Aria2CallRequest{
			Method: "aria2.getPeers",
			Params: []interface{}{gid},
		})
		if err != nil {
			continue
		}
		peers, ok := peersRaw.([]interface{})
		if !ok {
			continue
		}
		for _, peerItem := range peers {
			peer, ok := peerItem.(map[string]interface{})
			if !ok {
				continue
			}
			snapshot, ok := suspiciousPeerFromMap(gid, taskName, peer, blockedSet)
			if ok {
				suspicious = append(suspicious, snapshot)
			}
		}
	}
	return suspicious, nil
}

func suspiciousPeerFromMap(gid, taskName string, peer map[string]interface{}, blockedSet map[string]struct{}) (suspiciousPeerSnapshot, bool) {
	ip, _ := peer["ip"].(string)
	port, _ := peer["port"].(string)
	downloadSpeed, _ := peer["downloadSpeed"].(string)
	uploadSpeed, _ := peer["uploadSpeed"].(string)
	seeder := peerFlag(peer["seeder"])
	peerChoking := peerFlag(peer["peerChoking"])

	peerDownload := parseAria2Int64(downloadSpeed)
	peerUpload := parseAria2Int64(uploadSpeed)

	score := 0
	reason := ""
	if !seeder && peerChoking && peerDownload >= 64*1024 && peerUpload <= 4*1024 {
		score = defaultAutoBanMinScore
		reason = peerGuardAutoBanReason
	}
	if score < 2 || ip == "" {
		return suspiciousPeerSnapshot{}, false
	}
	_, blocked := blockedSet[ip]
	return suspiciousPeerSnapshot{
		GID:           gid,
		TaskName:      taskName,
		IP:            ip,
		Port:          port,
		DownloadSpeed: downloadSpeed,
		UploadSpeed:   uploadSpeed,
		Seeder:        seeder,
		Blocked:       blocked,
		Score:         score,
		Reason:        reason,
	}, true
}

func peerGuardTaskName(task map[string]interface{}) string {
	if bt, ok := task["bittorrent"].(map[string]interface{}); ok {
		if info, ok := bt["info"].(map[string]interface{}); ok {
			if name, ok := info["name"].(string); ok && strings.TrimSpace(name) != "" {
				return name
			}
		}
	}
	files, _ := task["files"].([]interface{})
	for _, item := range files {
		fileMap, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		path, _ := fileMap["path"].(string)
		if path == "" {
			continue
		}
		return filepath.Base(path)
	}
	gid, _ := task["gid"].(string)
	return gid
}

func parseAria2Int64(value string) int64 {
	numeric := strings.TrimSpace(value)
	if numeric == "" {
		return 0
	}
	var parsed int64
	_, _ = fmt.Sscan(numeric, &parsed)
	return parsed
}

func peerFlag(value interface{}) bool {
	text, _ := value.(string)
	return text == "true"
}

func (s *Server) setPeerGuardStatus(message string, appliedAt time.Time) {
	s.peerGuard.mu.Lock()
	defer s.peerGuard.mu.Unlock()
	s.peerGuard.lastError = message
	s.peerGuard.lastApplied = appliedAt
}

func (s *Server) peerGuardStatus() (string, time.Time) {
	s.peerGuard.mu.RLock()
	defer s.peerGuard.mu.RUnlock()
	return s.peerGuard.lastError, s.peerGuard.lastApplied
}

func formatPeerGuardTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.Format(time.RFC3339)
}

func peerGuardBanExpiresAt(createdAt time.Time) time.Time {
	return createdAt.Add(peerGuardBanDuration)
}

func peerBanExpired(record PeerBanRecord, now time.Time) bool {
	expiresAt, err := time.Parse(time.RFC3339, strings.TrimSpace(record.ExpiresAt))
	if err != nil || expiresAt.IsZero() {
		return false
	}
	return !expiresAt.After(now)
}

func (s *Server) marshalPeerGuardSnapshot() ([]byte, error) {
	snapshot, err := s.peerGuardSnapshot()
	if err != nil {
		return nil, err
	}
	return json.Marshal(snapshot)
}
