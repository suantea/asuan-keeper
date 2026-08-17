package syncthing

import (
	"encoding/json"
	"strings"
	"testing"

	"atomgit.com/suantea/asuan-keeper/internal/config"
)

// fakeConfig 构造最小 /rest/config 响应结构。
func fakeConfig() map[string]json.RawMessage {
	devices, _ := json.Marshal([]map[string]any{})
	folders, _ := json.Marshal([]map[string]any{})
	return map[string]json.RawMessage{
		"version": json.RawMessage(`38`),
		"devices": devices,
		"folders": folders,
		"options": json.RawMessage(`{}`),
	}
}

func TestApplyStealthDisablesAll(t *testing.T) {
	full := fakeConfig()
	applyStealth(full, config.Stealth{
		DisableUPnP:            true,
		DisableLocalDiscovery:  true,
		DisableGlobalDiscovery: true,
		DisableRelay:           true,
		DisableNAT:             true,
		TCPPort:                0,
	})
	var opts map[string]any
	if err := json.Unmarshal(full["options"], &opts); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"globalAnnounceEnabled", "localAnnounceEnabled", "relaysEnabled", "natEnabled", "upnpEnabled"} {
		if opts[k] != false {
			t.Errorf("%s 应为 false, got %v", k, opts[k])
		}
	}
	if _, ok := opts["listenAddresses"]; ok {
		t.Error("TCPPort=0 时不应设置 listenAddresses")
	}
}

func TestApplyStealthCustomPort(t *testing.T) {
	full := fakeConfig()
	applyStealth(full, config.Stealth{TCPPort: 41234})
	var opts map[string]any
	_ = json.Unmarshal(full["options"], &opts)
	addrs, _ := opts["listenAddresses"].([]any)
	if len(addrs) != 1 || !strings.Contains(addrs[0].(string), ":41234") {
		t.Fatalf("listenAddresses 应为 tcp://0.0.0.0:41234, got %v", addrs)
	}
}

func TestApplyFoldersAddsTrashcan(t *testing.T) {
	full := fakeConfig()
	if err := applyFolders(full, []config.Folder{
		{ID: "docs", Label: "文档", Path: "D:/Sync/docs"},
	}, "ABCDEF1234567890", []string{"PEER1", "PEER2"}); err != nil {
		t.Fatal(err)
	}
	var folders []map[string]any
	_ = json.Unmarshal(full["folders"], &folders)
	if len(folders) != 1 {
		t.Fatalf("应新增 1 个文件夹, got %d", len(folders))
	}
	f := folders[0]
	if f["id"] != "docs" || f["type"] != "sendreceive" {
		t.Fatalf("文件夹字段错误: %+v", f)
	}
	ver, _ := f["versioning"].(map[string]any)
	if ver["type"] != "trashcan" {
		t.Fatalf("应启用 trashcan 回收站, got %+v", ver)
	}
	devs, _ := f["devices"].([]any)
	if len(devs) != 3 {
		t.Fatalf("文件夹应共享给本机+2个对端, got %+v", devs)
	}
}

func TestApplyFoldersIdempotent(t *testing.T) {
	full := fakeConfig()
	rule := []config.Folder{{ID: "docs", Path: "D:/Sync/docs"}}
	if err := applyFolders(full, rule, "SELF", nil); err != nil {
		t.Fatal(err)
	}
	if err := applyFolders(full, rule, "SELF", nil); err != nil {
		t.Fatal(err)
	}
	var folders []map[string]any
	_ = json.Unmarshal(full["folders"], &folders)
	if len(folders) != 1 {
		t.Fatalf("重复应用不应重复添加, got %d", len(folders))
	}
}

func TestApplyFoldersCustomVersioning(t *testing.T) {
	full := fakeConfig()
	if err := applyFolders(full, []config.Folder{
		{
			ID: "docs", Path: "D:/Sync/docs",
			Versioning: &config.FolderVersioning{
				Type:   "simple",
				Params: map[string]any{"keep": 5},
			},
		},
	}, "SELF", nil); err != nil {
		t.Fatal(err)
	}
	var folders []map[string]any
	_ = json.Unmarshal(full["folders"], &folders)
	ver, _ := folders[0]["versioning"].(map[string]any)
	if ver["type"] != "simple" {
		t.Fatalf("应使用配置的 versioning 类型 simple, got %+v", ver)
	}
	params, _ := ver["params"].(map[string]any)
	if params["keep"] != float64(5) {
		t.Fatalf("versioning params 应透传 keep=5, got %+v", params)
	}
}

func TestApplyFoldersVersioningEmptyTypeFallsBack(t *testing.T) {
	full := fakeConfig()
	if err := applyFolders(full, []config.Folder{
		{ID: "docs", Path: "D:/Sync/docs", Versioning: &config.FolderVersioning{}},
	}, "SELF", nil); err != nil {
		t.Fatal(err)
	}
	var folders []map[string]any
	_ = json.Unmarshal(full["folders"], &folders)
	ver, _ := folders[0]["versioning"].(map[string]any)
	if ver["type"] != "trashcan" {
		t.Fatalf("type 为空应回退 trashcan, got %+v", ver)
	}
}

func TestApplyFoldersMaxConflicts(t *testing.T) {
	full := fakeConfig()
	if err := applyFolders(full, []config.Folder{
		{ID: "docs", Path: "D:/Sync/docs", MaxConflicts: 20},
	}, "SELF", nil); err != nil {
		t.Fatal(err)
	}
	var folders []map[string]any
	_ = json.Unmarshal(full["folders"], &folders)
	if folders[0]["maxConflicts"] != float64(20) {
		t.Fatalf("maxConflicts 应透传 20, got %+v", folders[0]["maxConflicts"])
	}

	// 未配置时不应出现该字段（引擎默认值生效）
	full = fakeConfig()
	if err := applyFolders(full, []config.Folder{
		{ID: "docs2", Path: "D:/Sync/docs2"},
	}, "SELF", nil); err != nil {
		t.Fatal(err)
	}
	var folders2 []map[string]any
	_ = json.Unmarshal(full["folders"], &folders2)
	if _, ok := folders2[0]["maxConflicts"]; ok {
		t.Fatalf("未配置时不应输出 maxConflicts, got %+v", folders2[0])
	}
}

func TestApplyDevicesRemoteLimit(t *testing.T) {
	full := fakeConfig()
	peers := []config.Peer{
		{Name: "nas", DeviceID: "PEER_REMOTE", Remote: true},
		{Name: "lan", DeviceID: "PEER_LAN", Address: "192.168.1.5:22000"},
	}
	if err := applyDevices(full, peers, "SELF", "self", config.Remote{
		Enable: true, Endpoint: "wg.example.com:22000", LimitKbps: 2048,
	}, "metadata"); err != nil {
		t.Fatal(err)
	}
	var devices []map[string]any
	_ = json.Unmarshal(full["devices"], &devices)
	got := map[string]map[string]any{}
	for _, d := range devices {
		id, _ := d["deviceID"].(string)
		got[id] = d
	}
	rd := got["PEER_REMOTE"]
	if rd == nil {
		t.Fatal("应包含远程对端")
	}
	addrs, _ := rd["addresses"].([]any)
	if len(addrs) != 1 || addrs[0] != "wg.example.com:22000" {
		t.Fatalf("远程对端地址应为隧道端点, got %v", addrs)
	}
	if rd["maxSendKbps"] != float64(2048) || rd["maxRecvKbps"] != float64(2048) {
		t.Fatalf("远程对端应限速 2048kbps, got %+v", rd)
	}
	lan := got["PEER_LAN"]
	if lan == nil {
		t.Fatal("应包含局域网对端")
	}
	if _, ok := lan["maxSendKbps"]; ok {
		t.Fatalf("局域网对端不应限速, got %+v", lan)
	}
	if lan["addresses"].([]any)[0] != "192.168.1.5:22000" {
		t.Fatalf("局域网对端地址应保持静态地址, got %+v", lan["addresses"])
	}
}

func TestApplyDevicesRemoteDisabled(t *testing.T) {
	full := fakeConfig()
	peers := []config.Peer{{Name: "p", DeviceID: "PEER", Remote: true, Address: "192.168.1.9:22000"}}
	if err := applyDevices(full, peers, "SELF", "self", config.Remote{}, "metadata"); err != nil {
		t.Fatal(err)
	}
	var devices []map[string]any
	_ = json.Unmarshal(full["devices"], &devices)
	d := devices[0]
	if _, ok := d["maxSendKbps"]; ok {
		t.Fatalf("远程未启用时不应限速, got %+v", d)
	}
	if d["addresses"].([]any)[0] != "192.168.1.9:22000" {
		t.Fatalf("远程未启用时应回退静态地址, got %+v", d["addresses"])
	}
}
