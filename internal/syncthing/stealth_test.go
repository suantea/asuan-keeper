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
