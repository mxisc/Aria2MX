package server

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"ariamx/internal/aria2embed"
)

var managedRestartOptionKeys = map[string]struct{}{
	"auto-save-interval":       {},
	"check-certificate":        {},
	"conf-path":                {},
	"console-log-level":        {},
	"daemon":                   {},
	"deferred-input":           {},
	"dht-file-path":            {},
	"dht-file-path6":           {},
	"dht-listen-port":          {},
	"dht-message-timeout":      {},
	"disable-ipv6":             {},
	"disk-cache":               {},
	"dscp":                     {},
	"enable-color":             {},
	"enable-dht":               {},
	"enable-dht6":              {},
	"enable-rpc":               {},
	"event-poll":               {},
	"human-readable":           {},
	"listen-port":              {},
	"min-tls-version":          {},
	"netrc-path":               {},
	"no-conf":                  {},
	"peer-agent":               {},
	"peer-id-prefix":           {},
	"quiet":                    {},
	"rlimit-nofile":            {},
	"rpc-allow-origin-all":     {},
	"rpc-listen-all":           {},
	"rpc-listen-port":          {},
	"rpc-max-request-size":     {},
	"rpc-secure":               {},
	"save-session-interval":    {},
	"server-stat-timeout":      {},
	"show-console-readout":     {},
	"socket-recv-buffer-size":  {},
	"stop":                     {},
	"summary-interval":         {},
	"truncate-console-readout": {},
}

var managedCommaSeparatedOptionKeys = map[string]struct{}{
	"bt-tracker":         {},
	"bt-exclude-tracker": {},
	"no-proxy":           {},
}

type ManagedAria2 struct {
	configPath string
	cfg        *Config
	cfgMu      *sync.RWMutex
	client     *Aria2Client

	procMu sync.Mutex
	cmd    *exec.Cmd
	waitCh chan error
	reused bool
}

type ManagedOptionsSaveResult struct {
	Restarted bool   `json:"restarted"`
	Message   string `json:"message"`
}

func NewManagedAria2(configPath string, cfg *Config, cfgMu *sync.RWMutex, client *Aria2Client) *ManagedAria2 {
	return &ManagedAria2{
		configPath: configPath,
		cfg:        cfg,
		cfgMu:      cfgMu,
		client:     client,
	}
}

func (m *ManagedAria2) Start() error {
	m.procMu.Lock()
	defer m.procMu.Unlock()
	return m.startLocked()
}

func (m *ManagedAria2) Stop() error {
	m.procMu.Lock()
	defer m.procMu.Unlock()
	return m.stopLocked()
}

func (m *ManagedAria2) SaveOptions(patch map[string]string) (ManagedOptionsSaveResult, error) {
	if len(patch) == 0 {
		return ManagedOptionsSaveResult{Message: "没有需要保存的变化。"}, nil
	}

	currentCfg := m.snapshotConfig()
	targetRPCPort, err := managedRPCPortForPatch(currentCfg.ManagedRPCPort, patch)
	if err != nil {
		return ManagedOptionsSaveResult{}, err
	}
	assignedRPCPort := targetRPCPort
	portAdjusted := false
	if targetRPCPort != currentCfg.ManagedRPCPort {
		assignedRPCPort, err = findAvailableManagedRPCPort(targetRPCPort, 10)
		if err != nil {
			return ManagedOptionsSaveResult{}, err
		}
		portAdjusted = assignedRPCPort != targetRPCPort
	}

	restartNeeded := false
	for key := range patch {
		if _, ok := managedRestartOptionKeys[key]; ok {
			restartNeeded = true
			break
		}
	}

	m.cfgMu.Lock()
	sanitizedPatch, err := applyManagedOptionPatch(m.cfg, patch)
	if err != nil {
		m.cfgMu.Unlock()
		return ManagedOptionsSaveResult{}, err
	}
	if assignedRPCPort != m.cfg.Aria2.ManagedRPCPort {
		m.cfg.Aria2.ManagedRPCPort = assignedRPCPort
		m.cfg.Aria2.RPCURL = managedRPCURL(assignedRPCPort)
	}
	saveErr := SaveConfig(m.configPath, m.cfg)
	m.cfgMu.Unlock()
	if saveErr != nil {
		return ManagedOptionsSaveResult{}, saveErr
	}

	if restartNeeded {
		m.procMu.Lock()
		err = m.restartLocked()
		m.procMu.Unlock()
		if err != nil {
			return ManagedOptionsSaveResult{}, err
		}
		return ManagedOptionsSaveResult{
			Restarted: true,
			Message:   managedRestartMessage(assignedRPCPort, portAdjusted),
		}, nil
	}

	_, err = m.client.Call(Aria2CallRequest{
		Method: "aria2.changeGlobalOption",
		Params: []interface{}{sanitizedPatch},
	})
	if err != nil {
		return ManagedOptionsSaveResult{}, err
	}
	return ManagedOptionsSaveResult{Message: "选项已保存。"}, nil
}

func (m *ManagedAria2) ResetOptions() (ManagedOptionsSaveResult, error) {
	m.cfgMu.Lock()
	m.cfg.Aria2.Options = nil
	m.cfg.Aria2.ManagedRPCPort = defaultManagedRPCPort
	m.cfg.Aria2.RPCURL = managedRPCURL(m.cfg.Aria2.ManagedRPCPort)
	saveErr := SaveConfig(m.configPath, m.cfg)
	m.cfgMu.Unlock()
	if saveErr != nil {
		return ManagedOptionsSaveResult{}, saveErr
	}

	m.procMu.Lock()
	err := m.restartLocked()
	m.procMu.Unlock()
	if err != nil {
		return ManagedOptionsSaveResult{}, err
	}
	currentCfg := m.snapshotConfig()
	return ManagedOptionsSaveResult{
		Restarted: true,
		Message:   managedResetMessage(currentCfg.ManagedRPCPort),
	}, nil
}

func applyManagedOptionPatch(cfg *Config, patch map[string]string) (map[string]string, error) {
	if cfg.Aria2.Options == nil {
		cfg.Aria2.Options = map[string]string{}
	}
	sanitizedPatch := make(map[string]string, len(patch))
	for key, value := range patch {
		normalized := normalizeManagedOptionValue(key, value)
		trimmed := strings.TrimSpace(normalized)
		switch key {
		case "rpc-listen-port":
			if trimmed == "" {
				cfg.Aria2.ManagedRPCPort = defaultManagedRPCPort
			} else {
				port, err := strconv.Atoi(trimmed)
				if err != nil || port <= 0 || port > 65535 {
					return nil, errors.New("RPC 监听端口无效，请输入 1-65535 之间的整数。")
				}
				cfg.Aria2.ManagedRPCPort = port
			}
			cfg.Aria2.RPCURL = managedRPCURL(cfg.Aria2.ManagedRPCPort)
			delete(cfg.Aria2.Options, key)
			continue
		}
		if trimmed == "" {
			delete(cfg.Aria2.Options, key)
			continue
		}
		cfg.Aria2.Options[key] = normalized
		sanitizedPatch[key] = normalized
	}
	return sanitizedPatch, nil
}

func normalizeManagedOptionValue(key, value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}
	if _, ok := managedCommaSeparatedOptionKeys[key]; ok {
		parts := splitManagedOptionList(trimmed)
		return strings.Join(parts, ",")
	}
	return value
}

func splitManagedOptionList(value string) []string {
	fields := strings.FieldsFunc(value, func(r rune) bool {
		return r == '\n' || r == '\r' || r == ','
	})
	parts := make([]string, 0, len(fields))
	seen := map[string]struct{}{}
	for _, field := range fields {
		item := strings.TrimSpace(field)
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		parts = append(parts, item)
	}
	return parts
}

func managedRPCPortForPatch(currentPort int, patch map[string]string) (int, error) {
	value, ok := patch["rpc-listen-port"]
	if !ok {
		return currentPort, nil
	}
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return defaultManagedRPCPort, nil
	}
	port, err := strconv.Atoi(trimmed)
	if err != nil || port <= 0 || port > 65535 {
		return 0, errors.New("RPC 监听端口无效，请输入 1-65535 之间的整数。")
	}
	return port, nil
}

func ensureManagedRPCPortAvailable(port int) error {
	if err := probeManagedRPCPort("tcp4", fmt.Sprintf("127.0.0.1:%d", port)); err != nil {
		return fmt.Errorf("RPC 监听端口 %d 已被占用，请换一个端口后再保存。", port)
	}
	if err := probeManagedRPCPort("tcp6", fmt.Sprintf("[::1]:%d", port)); err != nil && !ignorableIPv6ProbeErr(err) {
		return fmt.Errorf("RPC 监听端口 %d 已被占用，请换一个端口后再保存。", port)
	}
	return nil
}

func findAvailableManagedRPCPort(preferredPort, step int) (int, error) {
	if step <= 0 {
		step = 10
	}
	for port := preferredPort; port <= 65535; port += step {
		if ensureManagedRPCPortAvailable(port) == nil {
			return port, nil
		}
	}
	return 0, fmt.Errorf("RPC 监听端口 %d 以及后续步进端口都不可用，请检查端口占用。", preferredPort)
}

func probeManagedRPCPort(network, address string) error {
	listener, err := net.Listen(network, address)
	if err != nil {
		return err
	}
	return listener.Close()
}

func ignorableIPv6ProbeErr(err error) bool {
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "address family not supported") ||
		strings.Contains(message, "cannot assign requested address") ||
		strings.Contains(message, "unsupported operation")
}

func (m *ManagedAria2) startLocked() error {
	if m.cmd != nil {
		return nil
	}
	if m.reused {
		return nil
	}

	cfg := m.snapshotConfig()
	desiredPort := cfg.ManagedRPCPort
	if err := ensureManagedRPCPortAvailable(cfg.ManagedRPCPort); err != nil {
		if m.canReuseExistingLocked() {
			m.reused = true
			log.Printf("managed aria2 reused existing process on %s", cfg.RPCURL)
			return nil
		}
		assignedPort, assignErr := findAvailableManagedRPCPort(cfg.ManagedRPCPort+10, 10)
		if assignErr != nil {
			return err
		}
		if updateErr := m.persistManagedRPCPortLocked(assignedPort); updateErr != nil {
			return updateErr
		}
		cfg = m.snapshotConfig()
		log.Printf("managed aria2 rpc port %d occupied, switched to %d", desiredPort, assignedPort)
	}
	root := m.stateRoot(cfg)
	if err := os.MkdirAll(root, 0o755); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(root, "downloads"), 0o755); err != nil {
		return err
	}

	binaryPath, runtimeEnv, err := m.resolveBinary(cfg)
	if err != nil {
		return err
	}
	confPath, err := m.writeConfigFile(root, cfg)
	if err != nil {
		return err
	}
	logFile, err := os.OpenFile(filepath.Join(root, "aria2.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}

	sessionPath := filepath.Join(root, "session.dat")
	if _, err := os.Stat(sessionPath); errors.Is(err, os.ErrNotExist) {
		if err := os.WriteFile(sessionPath, nil, 0o600); err != nil {
			_ = logFile.Close()
			return err
		}
	}
	args := []string{
		"--conf-path=" + confPath,
		"--enable-rpc=true",
		"--rpc-listen-all=false",
		fmt.Sprintf("--rpc-listen-port=%d", cfg.ManagedRPCPort),
		"--rpc-save-upload-metadata=true",
		"--rpc-secret=" + cfg.RPCSecret,
		"--rpc-secure=false",
		"--rpc-allow-origin-all=false",
		"--input-file=" + sessionPath,
		"--save-session=" + sessionPath,
		"--save-session-interval=30",
		"--auto-save-interval=30",
	}

	cmd := exec.Command(binaryPath, args...)
	cmd.Dir = root
	cmd.Env = append(os.Environ(), runtimeEnv...)
	cmd.Stdout = logFile
	cmd.Stderr = logFile

	if err := cmd.Start(); err != nil {
		_ = logFile.Close()
		return err
	}

	waitCh := make(chan error, 1)
	go func() {
		err := cmd.Wait()
		_ = logFile.Close()
		waitCh <- err
	}()

	m.cmd = cmd
	m.waitCh = waitCh

	if err := m.waitForReady(15 * time.Second); err != nil {
		_ = m.stopLocked()
		return err
	}

	log.Printf("managed aria2 started with %s", binaryPath)
	return nil
}

func (m *ManagedAria2) persistManagedRPCPortLocked(port int) error {
	m.cfgMu.Lock()
	defer m.cfgMu.Unlock()
	m.cfg.Aria2.ManagedRPCPort = port
	m.cfg.Aria2.RPCURL = managedRPCURL(port)
	return SaveConfig(m.configPath, m.cfg)
}

func (m *ManagedAria2) restartLocked() error {
	if err := m.stopLocked(); err != nil {
		return err
	}
	return m.startLocked()
}

func (m *ManagedAria2) stopLocked() error {
	if m.reused {
		m.reused = false
		return m.shutdownReusedLocked()
	}
	if m.cmd == nil {
		return nil
	}
	cmd := m.cmd
	waitCh := m.waitCh
	m.cmd = nil
	m.waitCh = nil

	select {
	case err := <-waitCh:
		return normalizeExitErr(err)
	default:
	}

	if err := cmd.Process.Signal(os.Interrupt); err != nil && !errors.Is(err, os.ErrProcessDone) {
		_ = cmd.Process.Kill()
		<-waitCh
		return err
	}

	select {
	case err := <-waitCh:
		return normalizeExitErr(err)
	case <-time.After(5 * time.Second):
		if err := cmd.Process.Signal(syscall.SIGKILL); err != nil && !errors.Is(err, os.ErrProcessDone) {
			return err
		}
		return normalizeExitErr(<-waitCh)
	}
}

func normalizeExitErr(err error) error {
	if err == nil {
		return nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && exitErr.ExitCode() == -1 {
		return nil
	}
	return err
}

func (m *ManagedAria2) canReuseExistingLocked() bool {
	result, err := m.client.Call(Aria2CallRequest{Method: "aria2.getVersion"})
	if err != nil {
		return false
	}
	versionInfo, ok := result.(map[string]interface{})
	if !ok {
		return false
	}
	version, _ := versionInfo["version"].(string)
	return strings.TrimSpace(version) != ""
}

func (m *ManagedAria2) shutdownReusedLocked() error {
	_, err := m.client.Call(Aria2CallRequest{Method: "aria2.forceShutdown"})
	if err != nil && !isExpectedShutdownErr(err) {
		return err
	}
	return m.waitForRPCPortReleased(5 * time.Second)
}

func (m *ManagedAria2) waitForRPCPortReleased(timeout time.Duration) error {
	cfg := m.snapshotConfig()
	deadline := time.Now().Add(timeout)
	addresses := []string{
		fmt.Sprintf("127.0.0.1:%d", cfg.ManagedRPCPort),
		fmt.Sprintf("[::1]:%d", cfg.ManagedRPCPort),
	}
	for time.Now().Before(deadline) {
		if !isAnyRPCAddressReachable(addresses) {
			return nil
		}
		time.Sleep(200 * time.Millisecond)
	}
	return errors.New("等待 aria2 退出超时")
}

func isAnyRPCAddressReachable(addresses []string) bool {
	for _, address := range addresses {
		conn, err := net.DialTimeout("tcp", address, 300*time.Millisecond)
		if err == nil {
			_ = conn.Close()
			return true
		}
	}
	return false
}

func isExpectedShutdownErr(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "connection reset") ||
		strings.Contains(message, "eof") ||
		strings.Contains(message, "broken pipe") ||
		strings.Contains(message, "aria2 unreachable")
}

func managedRestartMessage(port int, adjusted bool) string {
	if adjusted {
		return fmt.Sprintf("目标 RPC 端口已被占用，已自动切换到 %d，aria2 已重启并重新加载配置。", port)
	}
	return "选项已保存，aria2 已重启并重新加载配置。"
}

func managedResetMessage(port int) string {
	if port != defaultManagedRPCPort {
		return fmt.Sprintf("aria2 配置已重置为默认值；默认 RPC 端口 %d 被占用，已自动切换到 %d 并重启。", defaultManagedRPCPort, port)
	}
	return "aria2 配置已重置为默认值，aria2 已重启。"
}

func (m *ManagedAria2) waitForReady(timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		select {
		case err := <-m.waitCh:
			m.cmd = nil
			m.waitCh = nil
			if err == nil {
				return errors.New("aria2 exited before initialization")
			}
			return fmt.Errorf("aria2 exited early: %w", err)
		default:
		}
		if _, err := m.client.Call(Aria2CallRequest{Method: "aria2.getVersion"}); err == nil {
			return nil
		}
		time.Sleep(300 * time.Millisecond)
	}
	return errors.New("aria2 startup timed out")
}

func (m *ManagedAria2) snapshotConfig() Aria2Config {
	m.cfgMu.RLock()
	defer m.cfgMu.RUnlock()
	cfg := m.cfg.Aria2
	if m.cfg.Aria2.Options != nil {
		cfg.Options = make(map[string]string, len(m.cfg.Aria2.Options))
		for key, value := range m.cfg.Aria2.Options {
			cfg.Options[key] = value
		}
	}
	return cfg
}

func (m *ManagedAria2) stateRoot(cfg Aria2Config) string {
	if cfg.ManagedStateDir != "" {
		if absolute, err := filepath.Abs(cfg.ManagedStateDir); err == nil {
			return absolute
		}
		return cfg.ManagedStateDir
	}
	base := filepath.Dir(m.configPath)
	if base == "." || base == "" {
		base = "."
	}
	root := filepath.Join(base, "ariamx-data", "aria2")
	if absolute, err := filepath.Abs(root); err == nil {
		return absolute
	}
	return root
}

func (m *ManagedAria2) writeConfigFile(root string, cfg Aria2Config) (string, error) {
	confPath := filepath.Join(root, "aria2.conf")
	lines := make([]string, 0, len(cfg.Options)+2)
	keys := make([]string, 0, len(cfg.Options))
	for key := range cfg.Options {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		value := strings.TrimSpace(normalizeManagedOptionValue(key, cfg.Options[key]))
		if value == "" {
			continue
		}
		lines = append(lines, fmt.Sprintf("%s=%s", key, value))
	}
	m.cfgMu.RLock()
	defaultDir := strings.TrimSpace(m.cfg.Panel.DefaultDownloadDir)
	m.cfgMu.RUnlock()
	if _, ok := cfg.Options["dir"]; !ok && defaultDir != "" {
		lines = append(lines, fmt.Sprintf("dir=%s", defaultDir))
	} else if _, ok := cfg.Options["dir"]; !ok {
		lines = append(lines, fmt.Sprintf("dir=%s", filepath.Join(root, "downloads")))
	}
	data := strings.Join(lines, "\n")
	if data != "" {
		data += "\n"
	}
	if err := os.WriteFile(confPath, []byte(data), 0o600); err != nil {
		return "", err
	}
	return confPath, nil
}

func (m *ManagedAria2) resolveBinary(cfg Aria2Config) (string, []string, error) {
	if cfg.ManagedBinaryPath != "" {
		return cfg.ManagedBinaryPath, nil, nil
	}

	runtimeKey := runtime.GOOS + "-" + runtime.GOARCH
	root := filepath.Join(m.stateRoot(cfg), "runtime", runtimeKey)
	binaryName := "aria2c"
	binaryPath := filepath.Join(root, "bin", binaryName)
	if stat, err := os.Stat(binaryPath); err == nil && !stat.IsDir() {
		return binaryPath, runtimeLibraryEnv(root), nil
	}

	archive, err := aria2embed.RuntimeArchive()
	if err == nil {
		if err := os.RemoveAll(root); err != nil && !errors.Is(err, os.ErrNotExist) {
			return "", nil, err
		}
		if err := os.MkdirAll(root, 0o755); err != nil {
			return "", nil, err
		}
		if err := extractTarGz(archive, root); err != nil {
			return "", nil, err
		}
		if err := os.Chmod(binaryPath, 0o755); err != nil && !errors.Is(err, os.ErrNotExist) {
			return "", nil, err
		}
		return binaryPath, runtimeLibraryEnv(root), nil
	}

	fallback, lookupErr := exec.LookPath("aria2c")
	if lookupErr == nil {
		return fallback, nil, nil
	}
	return "", nil, fmt.Errorf("未找到可用的 aria2 运行时，请先为 %s-%s 执行 node scripts/prepare-aria2-runtime.mjs 后再使用 -tags allinone 构建", runtime.GOOS, runtime.GOARCH)
}

func runtimeLibraryEnv(root string) []string {
	libDir := filepath.Join(root, "lib")
	if _, err := os.Stat(libDir); err != nil {
		return nil
	}
	switch runtime.GOOS {
	case "darwin":
		return []string{
			"DYLD_LIBRARY_PATH=" + libDir,
			"DYLD_FALLBACK_LIBRARY_PATH=" + libDir,
		}
	case "linux":
		return []string{"LD_LIBRARY_PATH=" + libDir}
	default:
		return nil
	}
}

func extractTarGz(data []byte, target string) error {
	gzipReader, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return err
	}
	defer gzipReader.Close()

	reader := tar.NewReader(gzipReader)
	for {
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		fullPath := filepath.Join(target, header.Name)
		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(fullPath, 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
				return err
			}
			file, err := os.OpenFile(fullPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, os.FileMode(header.Mode))
			if err != nil {
				return err
			}
			if _, err := io.Copy(file, reader); err != nil {
				_ = file.Close()
				return err
			}
			if err := file.Close(); err != nil {
				return err
			}
		case tar.TypeSymlink:
			if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
				return err
			}
			if err := os.Symlink(header.Linkname, fullPath); err != nil && !errors.Is(err, os.ErrExist) {
				return err
			}
		}
	}
}
