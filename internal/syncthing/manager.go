// Package syncthing 管理 Syncthing sidecar 进程：生成配置、启动、通过 REST API
// 应用隐蔽选项、设备与文件夹。
package syncthing

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/suantea/asuan-keeper/internal/config"
	"github.com/suantea/asuan-keeper/internal/procprio"
)

type Manager struct {
	Cfg     *config.Config
	Exe     string
	HomeDir string
	APIBase string

	mu         sync.Mutex
	proc       *exec.Cmd
	restarting bool // Reload 进行中：子进程退出不视为停止
}

func New(cfg *config.Config, exeDir string) *Manager {
	exe := cfg.Syncthing.Bin
	if exe == "" {
		candidates := []string{
			filepath.Join(exeDir, "syncthing.exe"),
			filepath.Join(exeDir, "syncthing"),
		}
		for _, c := range candidates {
			if fi, err := os.Stat(c); err == nil && !fi.IsDir() {
				exe = c
				break
			}
		}
	} else if !filepath.IsAbs(exe) {
		// 相对路径：优先解析为程序目录下的绝对路径（如 rename 产生的
		// "syncw.exe"）。Go 1.19+ 的 exec.Command 拒绝执行不带 ./ 前缀的
		// 相对路径可执行文件（"cannot run executable found relative to
		// current directory"），必须在 exeDir 下解析为绝对路径。
		abs := filepath.Join(exeDir, exe)
		if fi, err := os.Stat(abs); err == nil && !fi.IsDir() {
			exe = abs
		}
		// 若程序目录下不存在，保留原名交给 PATH。
	}
	if exe == "" {
		exe = "syncthing" // 交给 PATH
	}
	home := cfg.Syncthing.DataDir
	if home == "" {
		home = filepath.Join(exeDir, "syncthing")
	}
	return &Manager{Cfg: cfg, Exe: exe, HomeDir: home, APIBase: "http://" + cfg.Syncthing.GUIBind}
}

func RandomKey() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "asuan-dev-key"
	}
	return hex.EncodeToString(b)
}

// Generate 用 syncthing generate 生成最小配置（证书、默认配置）。
func (m *Manager) Generate() error {
	if err := os.MkdirAll(m.HomeDir, 0o700); err != nil {
		return err
	}
	cmd := exec.Command(m.Exe, "generate", "--home="+m.HomeDir)
	cmd.Stdout, cmd.Stderr = io.Discard, io.Discard
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("syncthing generate 失败: %w", err)
	}
	return nil
}

// ConfigExists 判断 syncthing 配置是否已生成。
func (m *Manager) ConfigExists() bool {
	_, err := os.Stat(filepath.Join(m.HomeDir, "config.xml"))
	return err == nil
}

// CleanStaleLocks 清理本实例 home 下的残留锁文件（强杀/崩溃后可能残留，
// 会导致重启时 "Failed to acquire lock"）。
func (m *Manager) CleanStaleLocks() {
	_ = os.Remove(filepath.Join(m.HomeDir, "syncthing.lock"))
	entries, err := os.ReadDir(m.HomeDir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".syncthing.tmp") {
			_ = os.Remove(filepath.Join(m.HomeDir, e.Name()))
		}
	}
}

// Start 启动 sidecar 进程，GUI 绑定 loopback，关闭浏览器/自动升级。
func (m *Manager) Start() error {
	m.mu.Lock()
	if m.proc != nil && m.proc.Process != nil {
		m.mu.Unlock()
		return nil
	}
	m.mu.Unlock()
	cmd := exec.Command(m.Exe, "--home="+m.HomeDir, "--no-browser")
	cmd.Env = append(os.Environ(),
		"STGUIADDRESS="+m.Cfg.Syncthing.GUIBind,
		"STGUIAPIKEY="+m.Cfg.Syncthing.GUIAPIKey,
		"STNOUPGRADE=1",
		// 禁止 syncthing 配置变更后自重启（否则进程所有权会脱离 asuan，
		// 残留孤儿进程）。配置落盘后由 asuan 显式重启。
		"STNORESTART=1",
	)
	logPath := filepath.Join(m.HomeDir, "syncthing.log")
	// 日志瘦身：syncthing.log 超过上限（LogMaxMB，默认 5MB）时截断，
	// 防止长期运行日志无限增长占用磁盘。
	if maxMB := m.Cfg.Syncthing.LogMaxMB; maxMB > 0 {
		if fi, err := os.Stat(logPath); err == nil && fi.Size() > int64(maxMB)*1024*1024 {
			_ = os.Remove(logPath + ".1")
			_ = os.Rename(logPath, logPath+".1")
		}
	}
	if f, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600); err == nil {
		cmd.Stdout, cmd.Stderr = f, f
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("启动 syncthing 失败: %w", err)
	}
	// 后台 sidecar 设低优先级，避免抢占前台应用（Windows 生效，其他平台 no-op）。
	procprio.SetBelowNormal(cmd.Process.Pid)
	m.mu.Lock()
	m.proc = cmd
	m.mu.Unlock()
	return nil
}

// Wait 阻塞等待 sidecar 进程退出。
//
// 引擎内部会因配置重载而重启（Reload），期间子进程退出属于正常重启而非停止；
// 只有真正停止（asuan stop 触发 shutdown，且不在重载中）才返回。
func (m *Manager) Wait() error {
	for {
		m.mu.Lock()
		p := m.proc
		restarting := m.restarting
		m.mu.Unlock()
		if p == nil || p.Process == nil {
			// 可能是停止请求（正常返回），也可能是 Reload 的间隙；确认一下。
			time.Sleep(300 * time.Millisecond)
			m.mu.Lock()
			p2 := m.proc
			restarting = m.restarting
			m.mu.Unlock()
			if p2 != nil || restarting {
				continue
			}
			return nil
		}
		err := p.Wait()
		m.mu.Lock()
		cur := m.proc
		restarting = m.restarting
		m.mu.Unlock()
		if cur != p || restarting {
			continue // Reload 已换新进程或正在进行中
		}
		return err
	}
}

// Stop 请求优雅关闭（REST）。无论是否跟踪进程都先尝试 shutdown，
// 使独立调用的 `asuan stop` 也能生效；随后等待跟踪进程退出，失败则杀进程。
// Stop 优雅关闭。
//
// 配置 PUT（Apply）可能让 syncthing 自重启出一个不受跟踪的新进程（孤儿），
// 单独一次 shutdown 可能落在重启间隙上，因此反复发 shutdown，直到 GUI 端口
// 彻底无响应（所有实例都退出）才返回。独立调用的 `asuan stop` 也走此路径。
func (m *Manager) Stop() error {
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		_, _ = m.api("POST", "/rest/system/shutdown", nil, nil)
		m.mu.Lock()
		p := m.proc
		m.mu.Unlock()
		if p != nil && p.Process != nil {
			done := make(chan struct{})
			go func() { p.Wait(); close(done) }()
			select {
			case <-done:
			case <-time.After(5 * time.Second):
				_ = p.Process.Kill()
				<-done
			}
		}
		m.mu.Lock()
		m.proc = nil
		m.mu.Unlock()
		if err := m.WaitDown(3 * time.Second); err == nil {
			return nil
		}
		time.Sleep(300 * time.Millisecond)
	}
	return fmt.Errorf("syncthing 未在 30s 内完全停止（GUI 端口仍被占用）")
}

// WaitAPI 轮询等待管理接口就绪。
func (m *Manager) WaitAPI(timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if _, err := m.api("GET", "/rest/system/version", nil, nil); err == nil {
			return nil
		}
		time.Sleep(300 * time.Millisecond)
	}
	return fmt.Errorf("等待管理接口超时（%s）", timeout)
}

// WaitDown 等待管理端口不再响应（旧实例进程与文件句柄完全释放）。
func (m *Manager) WaitDown(timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if _, err := m.api("GET", "/rest/system/version", nil, nil); err != nil {
			return nil
		}
		time.Sleep(200 * time.Millisecond)
	}
	return fmt.Errorf("旧实例未在 %s 内退出", timeout)
}

// Scan 触发指定文件夹的重扫（占位符释放/水合后让引擎感知本地变化）。
func (m *Manager) Scan(folderID string) error {
	_, err := m.api("POST", "/rest/db/scan?folder="+url.QueryEscape(folderID), nil, nil)
	return err
}

// Pause 暂停全部同步（托盘"暂停同步"入口）：停止拉取/推送，连接保留。
func (m *Manager) Pause() error {
	_, err := m.api("POST", "/rest/system/pause", nil, nil)
	return err
}

// Resume 恢复全部同步（托盘"继续同步"入口）。
func (m *Manager) Resume() error {
	_, err := m.api("POST", "/rest/system/resume", nil, nil)
	return err
}

// WaitFolderSynced 轮询等待文件夹本地与全局一致（水合后等对端内容拉回）。
// timeout 内未达成返回错误，便于 CLI 给出明确提示。
func (m *Manager) WaitFolderSynced(folderID string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		var fs FolderStatus
		if _, err := m.api("GET", "/rest/db/status?folder="+url.QueryEscape(folderID), nil, &fs); err == nil {
			// 本地总项数 ≥ 全局总项数即视为拉回完成（含忽略项不计）。
			if fs.LocalTotalItems >= fs.GlobalTotalItems && fs.NeedFiles == 0 {
				return nil
			}
		}
		time.Sleep(2 * time.Second)
	}
	return fmt.Errorf("等待文件夹 %s 同步完成超时（%s）", folderID, timeout)
}

// BrowseItem 是全局索引中的一个条目（占位符虚拟层数据源）。
type BrowseItem struct {
	Name string `json:"name"`
	Type string `json:"type"` // "FILE" | "DIRECTORY"
	Size int64  `json:"size"`
}

// Browse 列出文件夹全局索引中 prefix 路径下的条目（对端内容视图，
// 用于占位符虚拟层展示已释放文件夹的远端文件）。
func (m *Manager) Browse(folderID, prefix string) ([]BrowseItem, error) {
	var items []BrowseItem
	_, err := m.api("GET", "/rest/db/browse?folder="+url.QueryEscape(folderID)+"&prefix="+url.QueryEscape(prefix), nil, &items)
	return items, err
}

// MyID 返回本机 syncthing 设备 ID。
func (m *Manager) MyID() (string, error) {
	var st struct {
		MyID string `json:"myID"`
	}
	_, err := m.api("GET", "/rest/system/status", nil, &st)
	return st.MyID, err
}

// Apply 将 asuan 配置（隐蔽选项/对端/文件夹）应用到 syncthing。
func (m *Manager) Apply() error {
	var full map[string]json.RawMessage
	if _, err := m.api("GET", "/rest/config", nil, &full); err != nil {
		return err
	}
	selfID, _ := m.MyID()
	applyStealth(full, m.Cfg.Stealth)
	if err := applyDevices(full, m.Cfg.Peers, selfID, m.Cfg.Name, m.Cfg.Remote, m.Cfg.Syncthing.Compression); err != nil {
		return err
	}
	var peerIDs []string
	for _, p := range m.Cfg.Peers {
		peerIDs = append(peerIDs, p.DeviceID)
	}
	if err := applyFolders(full, m.Cfg.SyncedFolders(), selfID, peerIDs); err != nil {
		return err
	}
	_, err := m.api("PUT", "/rest/config", full, nil)
	return err
}

// Reload 应用配置后重启 syncthing 使生效。
// Stop 负责清理配置自重启产生的孤儿实例；Windows 下退出/启动的锁竞态
// 用缓冲 + 失败清理残留锁重试兜底。
func (m *Manager) Reload() error {
	m.mu.Lock()
	m.restarting = true
	m.mu.Unlock()
	defer func() {
		m.mu.Lock()
		m.restarting = false
		m.mu.Unlock()
	}()
	if err := m.Apply(); err != nil {
		return err
	}
	if err := m.Stop(); err != nil {
		return err
	}
	time.Sleep(1500 * time.Millisecond)
	if err := m.Start(); err != nil {
		return err
	}
	if err := m.WaitAPI(30 * time.Second); err != nil {
		m.CleanStaleLocks()
		_ = m.Stop()
		if err2 := m.Start(); err2 != nil {
			return err2
		}
		return m.WaitAPI(30 * time.Second)
	}
	return nil
}

func applyStealth(full map[string]json.RawMessage, s config.Stealth) {
	opts := map[string]any{
		"globalAnnounceEnabled": !s.DisableGlobalDiscovery,
		"localAnnounceEnabled":  !s.DisableLocalDiscovery,
		"relaysEnabled":         !s.DisableRelay,
		"natEnabled":            !s.DisableNAT,
		"upnpEnabled":           !s.DisableUPnP,
		"urAccepted":            -1,
		"autoUpgradeIntervalH":  0,
		"restartOnWakeup":       false,
		"startBrowser":          false,
	}
	if s.TCPPort != 0 {
		opts["listenAddresses"] = []string{fmt.Sprintf("tcp://0.0.0.0:%d", s.TCPPort)}
	}
	// 注意：syncthing 2.1.3 的 options 无 allowedNetworks 字段（PUT 静默忽略），
	// 网络层 IP 白名单通过系统防火墙 remoteip 实现（见 firewall 包），
	// 设备级白名单由 BEP 握手认证保证（devices 只含已声明对端）。
	b, _ := json.Marshal(opts)
	full["options"] = b
}

func applyDevices(full map[string]json.RawMessage, peers []config.Peer, selfID, selfName string, remote config.Remote, compression string) error {
	if compression == "" {
		compression = "metadata"
	}
	switch compression {
	case "metadata", "full", "off":
	default:
		compression = "metadata"
	}
	var cur []map[string]any
	if err := json.Unmarshal(full["devices"], &cur); err != nil {
		return err
	}
	byID := map[string]map[string]any{}
	for _, d := range cur {
		id, _ := d["deviceID"].(string)
		byID[id] = d
		// 本机设备名默认取主机名，易暴露身份；改为 asuan 配置的设备名。
		if id == selfID {
			d["name"] = selfName
		}
	}
	for _, p := range peers {
		d, exists := byID[p.DeviceID]
		if !exists {
			d = map[string]any{
				"deviceID":    p.DeviceID,
				"name":        p.Name,
				"compression": compression,
				"paused":      false,
			}
			byID[p.DeviceID] = d
		}
		// 已存在的对端也应用压缩策略（新建时已设置，这里统一覆盖）。
		d["compression"] = compression
		// 远程对端：地址走 WireGuard 隧道端点，并应用远程限速。
		if p.Remote && remote.Enable {
			if remote.Endpoint != "" {
				d["addresses"] = []string{remote.Endpoint}
			}
			if remote.LimitKbps > 0 {
				d["maxSendKbps"] = remote.LimitKbps
				d["maxRecvKbps"] = remote.LimitKbps
			}
		} else {
			addr := []string{}
			if p.Address != "" {
				addr = []string{p.Address}
			}
			d["addresses"] = addr
			// LAN 对端不受远程限速约束；若此前设置过限速则清除。
			delete(d, "maxSendKbps")
			delete(d, "maxRecvKbps")
		}
	}
	// 连接白名单：devices 列表只保留本机 + asuan 配置的 peers。
	// syncthing 的 BEP 握手要求 device ID 在对方配置中，未配置设备无法
	// 建立连接；这里进一步清掉 syncthing 自动发现/历史残留的设备记录，
	// 使本机配置收敛为「只认已声明对端」，减少暴露面。
	allowed := map[string]bool{}
	if selfID != "" {
		allowed[selfID] = true
	}
	for _, p := range peers {
		allowed[p.DeviceID] = true
	}
	cur = cur[:0]
	for id, d := range byID {
		if allowed[id] {
			cur = append(cur, d)
		}
	}
	b, _ := json.Marshal(cur)
	full["devices"] = b
	return nil
}

func applyFolders(full map[string]json.RawMessage, folders []config.Folder, selfID string, peerIDs []string) error {
	var cur []map[string]any
	if err := json.Unmarshal(full["folders"], &cur); err != nil {
		return err
	}
	have := map[string]bool{}
	for _, f := range cur {
		id, _ := f["id"].(string)
		have[id] = true
	}
	// 文件夹共享给本机 + 全部对端（同步策略文件夹）。
	deviceList := []any{}
	if selfID != "" {
		deviceList = append(deviceList, map[string]any{"deviceID": selfID})
	}
	for _, id := range peerIDs {
		deviceList = append(deviceList, map[string]any{"deviceID": id})
	}
	for _, f := range folders {
		if have[f.ID] {
			// 已有文件夹：合并缺失的对端设备（保证重复 Apply 后对端共享也在）。
			for _, f2 := range cur {
				if id, _ := f2["id"].(string); id == f.ID {
					mergeFolderDevices(f2, peerIDs)
					break
				}
			}
			continue
		}
		entry := map[string]any{
			"id":               f.ID,
			"label":            f.Label,
			"path":             f.Path,
			"type":             "sendreceive",
			"rescanIntervalS":  3600,
			"fsWatcherEnabled": true,
			"fsWatcherDelayS":  10,
			"ignorePerms":      true,
			"autoNormalize":    true,
			"versioning":       defaultVersioning(),
			"devices":          deviceList,
		}
		if f.MaxConflicts > 0 {
			entry["maxConflicts"] = f.MaxConflicts
		}
		if f.Versioning != nil {
			entry["versioning"] = folderVersioning(f.Versioning)
		}
		cur = append(cur, entry)
	}
	b, _ := json.Marshal(cur)
	full["folders"] = b
	return nil
}

// defaultVersioning 返回缺省版本控制：trashcan 回收站，保留 30 天。
func defaultVersioning() map[string]any {
	return map[string]any{
		"type":   "trashcan",
		"params": map[string]string{"cleanoutDays": "30"},
	}
}

// folderVersioning 按配置渲染 versioning 段；type 为空时回退缺省。
func folderVersioning(v *config.FolderVersioning) map[string]any {
	if v == nil || v.Type == "" {
		return defaultVersioning()
	}
	ver := map[string]any{"type": v.Type}
	if len(v.Params) > 0 {
		ver["params"] = v.Params
	}
	return ver
}

// mergeFolderDevices 向已有文件夹的 devices 列表补充缺失的对端设备。
func mergeFolderDevices(folder map[string]any, peerIDs []string) {
	devs, _ := folder["devices"].([]any)
	have := map[string]bool{}
	for _, d := range devs {
		if m, ok := d.(map[string]any); ok {
			if id, ok := m["deviceID"].(string); ok {
				have[id] = true
			}
		}
	}
	changed := false
	for _, id := range peerIDs {
		if !have[id] {
			devs = append(devs, map[string]any{"deviceID": id})
			changed = true
		}
	}
	if changed {
		folder["devices"] = devs
	}
}

// --- HTTP 帮助函数 ---

func (m *Manager) api(method, path string, reqBody, respOut any) (int, error) {
	var rd io.Reader
	if reqBody != nil {
		b, err := json.Marshal(reqBody)
		if err != nil {
			return 0, err
		}
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, m.APIBase+path, rd)
	if err != nil {
		return 0, err
	}
	req.Header.Set("X-API-Key", m.Cfg.Syncthing.GUIAPIKey)
	if reqBody != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if respOut != nil && resp.StatusCode < 400 {
		b, err := io.ReadAll(resp.Body)
		if err != nil {
			return resp.StatusCode, err
		}
		if err := json.Unmarshal(b, respOut); err != nil {
			return resp.StatusCode, fmt.Errorf("解析响应 %s: %w", path, err)
		}
	}
	return resp.StatusCode, nil
}

// Status 汇总同步状态（供 status 命令与 Web 界面）。
type Status struct {
	Running       bool
	Version       string
	MyID          string
	Folders       []FolderStatus
	Peers         []PeerStatus
	InBytesTotal  int64 // 在线对端累计接收字节（用于速率计算）
	OutBytesTotal int64 // 在线对端累计发送字节（用于速率计算）
}

type PeerStatus struct {
	Name      string `json:"name"`
	DeviceID  string `json:"device_id"`
	Connected bool   `json:"connected"`
	Address   string `json:"address"`
}

type FolderStatus struct {
	ID               string `json:"id"`
	Label            string `json:"label"`
	Path             string `json:"path"`
	State            string `json:"state"`
	Error            string `json:"error"` // 文件夹同步错误（如拉取失败/冲突），空=正常
	GlobalFiles      int64  `json:"globalFiles"`
	GlobalBytes      int64  `json:"globalBytes"`
	LocalFiles       int64  `json:"localFiles"`
	LocalBytes       int64  `json:"localBytes"`
	NeedFiles        int64  `json:"needFiles"`
	GlobalTotalItems int64  `json:"globalTotalItems"`
	LocalTotalItems  int64  `json:"localTotalItems"`
}

func (m *Manager) Status() (*Status, error) {
	var ver struct {
		Version string `json:"version"`
	}
	if _, err := m.api("GET", "/rest/system/version", nil, &ver); err != nil {
		return &Status{Running: false}, nil
	}
	st := &Status{Running: true, Version: ver.Version}
	st.MyID, _ = m.MyID()
	for _, f := range m.Cfg.SyncedFolders() {
		var fs FolderStatus
		if _, err := m.api("GET", "/rest/db/status?folder="+f.ID, nil, &fs); err != nil {
			continue
		}
		fs.ID = f.ID
		fs.Label = f.Label
		fs.Path = f.Path
		st.Folders = append(st.Folders, fs)
	}
	var conns struct {
		Connections map[string]struct {
			Connected     bool   `json:"connected"`
			Address       string `json:"address"`
			InBytesTotal  int64  `json:"inBytesTotal"`
			OutBytesTotal int64  `json:"outBytesTotal"`
		} `json:"connections"`
	}
	_, err := m.api("GET", "/rest/system/connections", nil, &conns)
	if err == nil {
		for _, p := range m.Cfg.Peers {
			c, ok := conns.Connections[p.DeviceID]
			st.Peers = append(st.Peers, PeerStatus{
				Name: p.Name, DeviceID: p.DeviceID,
				Connected: ok && c.Connected, Address: c.Address,
			})
			if ok && c.Connected {
				st.InBytesTotal += c.InBytesTotal
				st.OutBytesTotal += c.OutBytesTotal
			}
		}
	}
	return st, nil
}
