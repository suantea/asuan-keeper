// Package config 定义 asuan 每设备配置文件模型。
// 配置为 JSON，默认放在客户端程序同目录 asuan.json（绿色便携）。
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// FolderPolicy 每设备每文件夹三态策略。
const (
	PolicySync  = "sync"  // 参与同步（进入 Syncthing 配置）
	PolicyLocal = "local" // 仅本地，不参与任何同步，对其他设备不可见
	PolicyOff   = "off"   // 本设备不安装/不使用该文件夹
)

// DefaultTCPPort 是局域网同步端口默认值：各端开箱即一致，
// 无需每台设备单独配置；仍可显式设为 0 表示随机端口（隐蔽性更好）。
const DefaultTCPPort = 44312

var validPolicies = map[string]bool{PolicySync: true, PolicyLocal: true, PolicyOff: true}

type Config struct {
	Version int    `json:"version"`
	Name    string `json:"name"` // 本设备显示名

	Syncthing   Syncthing   `json:"syncthing"`
	Stealth     Stealth     `json:"stealth"`
	Remote      Remote      `json:"remote"`
	Web         Web         `json:"web"`
	Placeholder Placeholder `json:"placeholder"`
	Peers       []Peer      `json:"peers"`
	Folders     []Folder    `json:"folders"`
}

// Remote 远程访问配置（P2）。启用后对端可经 WireGuard 隧道（UDP 443
// 伪装 QUIC）跨网段同步；远程连接按 limit_kbps 限速，LAN 直连不受限。
type Remote struct {
	Enable       bool   `json:"enable"`         // 启用远程（WireGuard 隧道）访问
	Endpoint     string `json:"endpoint"`       // 隧道端点：主机名或 IP（如 wg.example.com）
	LimitKbps    int    `json:"limit_kbps"`     // 远程限速 kbps，0=不限
	LanFullSpeed bool   `json:"lan_full_speed"` // LAN 直连始终满速，不受远程限速影响
}

// Placeholder 占位符虚拟层（P1）。挂载点为空表示不启用；
// 启用后释放的文件夹以虚拟目录形式可见，访问即水合。
// 运行时需 WinFsp（Windows）/ macFUSE（macOS）/ FUSE（Linux）。
type Placeholder struct {
	Mount string `json:"mount"` // 虚拟层挂载点目录（空=不启用）
}

// Web 内置网页控制台。
type Web struct {
	Bind  string `json:"bind"`            // 监听地址：客户端用 127.0.0.1 仅本机，hub 用 0.0.0.0 供局域网访问
	Token string `json:"token,omitempty"` // 可选访问令牌：非空时控制台需携带此 token（默认空=不鉴权）
}

type Syncthing struct {
	Bin       string `json:"bin"`      // syncthing 可执行文件路径（空=同目录 syncthing.exe/syncthing）
	DataDir   string `json:"data_dir"` // syncthing 配置与数据目录（空=程序目录下 syncthing/）
	GUIBind   string `json:"gui_bind"` // 管理界面监听地址，隐蔽要求绑定 loopback
	GUIAPIKey string `json:"gui_api_key"`
}

type Stealth struct {
	DisableUPnP            bool     `json:"disable_upnp"`
	DisableLocalDiscovery  bool     `json:"disable_local_discovery"`
	DisableGlobalDiscovery bool     `json:"disable_global_discovery"`
	DisableRelay           bool     `json:"disable_relay"`
	DisableNAT             bool     `json:"disable_nat"`
	TCPPort                int      `json:"tcp_port"` // 0 = 随机（隐蔽性更好）
	AllowedNetworks        []string `json:"allowed_networks,omitempty"` // 可选：仅接受这些网段/主机连接（空=不限制，CIDR 或 IP）
	EnableUPnPNotUsed      bool     `json:"-"`        // 占位防止误用
}

type Peer struct {
	Name     string `json:"name"`
	DeviceID string `json:"device_id"`
	Address  string `json:"address"` // 局域网 host:port；仅静态地址，默认不扫描
	MAC      string `json:"mac"`     // 可选，仅作记录/ARP 辅助解析
	Remote   bool   `json:"remote"`  // 该对端走远程隧道（WireGuard），受远程限速约束
}

type Folder struct {
	ID     string `json:"id"`
	Label  string `json:"label"`
	Path   string `json:"path"`
	Policy string `json:"policy"`

	// Versioning 可选版本控制策略；nil 时使用默认（trashcan 回收站，保留 30 天）。
	Versioning *FolderVersioning `json:"versioning,omitempty"`
	// MaxConflicts 保留的冲突副本数（.sync-conflict-*）；0 表示用引擎默认值（10）。
	MaxConflicts int `json:"max_conflicts,omitempty"`
}

// FolderVersioning 对应 syncthing 文件夹的 versioning 配置。
// Type 支持 trashcan / simple / staggered / external；Params 透传各
// 类型的参数（trashcan 用 cleanoutDays，simple 用 keep 等）。
type FolderVersioning struct {
	Type   string         `json:"type"`
	Params map[string]any `json:"params,omitempty"`
}

func Default() *Config {
	return &Config{
		Version: 1,
		Name:    "asuan-device",
		Syncthing: Syncthing{
			Bin:     "",
			DataDir: "",
			GUIBind: "127.0.0.1:8384",
		},
		Web: Web{
			Bind: "127.0.0.1:18084",
		},
		Stealth: Stealth{
			DisableUPnP:            true,
			DisableLocalDiscovery:  true,
			DisableGlobalDiscovery: true,
			DisableRelay:           true,
			DisableNAT:             true,
			TCPPort:                DefaultTCPPort,
		},
		Remote: Remote{
			LanFullSpeed: true,
		},
	}
}

func DefaultPath(exeDir string) string {
	if exeDir == "" {
		exeDir = "."
	}
	return filepath.Join(exeDir, "asuan.json")
}

func Load(path string) (*Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var c Config
	if err := json.Unmarshal(b, &c); err != nil {
		return nil, fmt.Errorf("解析配置 %s 失败: %w", path, err)
	}
	if c.Web.Bind == "" {
		c.Web.Bind = "127.0.0.1:18084"
	}
	if err := c.Validate(); err != nil {
		return nil, err
	}
	return &c, nil
}

func (c *Config) Save(path string) error {
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o600)
}

func (c *Config) Validate() error {
	if c.Version != 1 {
		return fmt.Errorf("不支持的配置版本: %d", c.Version)
	}
	if c.Syncthing.GUIBind == "" {
		return fmt.Errorf("syncthing.gui_bind 不能为空")
	}
	if c.Stealth.TCPPort < 0 || c.Stealth.TCPPort > 65535 {
		return fmt.Errorf("stealth.tcp_port 非法: %d", c.Stealth.TCPPort)
	}
	if c.Remote.LimitKbps < 0 {
		return fmt.Errorf("remote.limit_kbps 非法: %d", c.Remote.LimitKbps)
	}
	if c.Remote.Enable && c.Remote.Endpoint == "" {
		return fmt.Errorf("remote.enable 为 true 时 remote.endpoint 不能为空")
	}
	for i, p := range c.Peers {
		if p.DeviceID == "" {
			return fmt.Errorf("peers[%d] 缺少 device_id", i)
		}
	}
	for i, f := range c.Folders {
		if f.ID == "" {
			return fmt.Errorf("folders[%d] 缺少 id", i)
		}
		if !validPolicies[f.Policy] {
			return fmt.Errorf("folders[%d] policy 非法: %q（应为 sync/local/off）", i, f.Policy)
		}
	}
	return nil
}

func (c *Config) SyncedFolders() []Folder {
	var out []Folder
	for _, f := range c.Folders {
		if f.Policy == PolicySync {
			out = append(out, f)
		}
	}
	return out
}
