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

var validPolicies = map[string]bool{PolicySync: true, PolicyLocal: true, PolicyOff: true}

type Config struct {
	Version int    `json:"version"`
	Name    string `json:"name"` // 本设备显示名

	Syncthing Syncthing `json:"syncthing"`
	Stealth   Stealth   `json:"stealth"`
	Web       Web       `json:"web"`
	Peers     []Peer    `json:"peers"`
	Folders   []Folder  `json:"folders"`
}

// Web 内置网页控制台。
type Web struct {
	Bind string `json:"bind"` // 监听地址：客户端用 127.0.0.1 仅本机，hub 用 0.0.0.0 供局域网访问
}

type Syncthing struct {
	Bin       string `json:"bin"`      // syncthing 可执行文件路径（空=同目录 syncthing.exe/syncthing）
	DataDir   string `json:"data_dir"` // syncthing 配置与数据目录（空=程序目录下 syncthing/）
	GUIBind   string `json:"gui_bind"` // 管理界面监听地址，隐蔽要求绑定 loopback
	GUIAPIKey string `json:"gui_api_key"`
}

type Stealth struct {
	DisableUPnP            bool `json:"disable_upnp"`
	DisableLocalDiscovery  bool `json:"disable_local_discovery"`
	DisableGlobalDiscovery bool `json:"disable_global_discovery"`
	DisableRelay           bool `json:"disable_relay"`
	DisableNAT             bool `json:"disable_nat"`
	TCPPort                int  `json:"tcp_port"` // 0 = 随机（隐蔽性更好）
	EnableUPnPNotUsed      bool `json:"-"`        // 占位防止误用
}

type Peer struct {
	Name     string `json:"name"`
	DeviceID string `json:"device_id"`
	Address  string `json:"address"` // 局域网 host:port；仅静态地址，默认不扫描
	MAC      string `json:"mac"`     // 可选，仅作记录/ARP 辅助解析
}

type Folder struct {
	ID     string `json:"id"`
	Label  string `json:"label"`
	Path   string `json:"path"`
	Policy string `json:"policy"`
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
			TCPPort:                0,
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
