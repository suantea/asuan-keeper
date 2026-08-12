package config

import (
	"path/filepath"
	"testing"
)

func TestDefaultValid(t *testing.T) {
	c := Default()
	if err := c.Validate(); err != nil {
		t.Fatalf("默认配置应通过校验: %v", err)
	}
	if got := len(c.SyncedFolders()); got != 0 {
		t.Fatalf("默认配置不应有同步文件夹, got %d", got)
	}
}

func TestPolicyInvalid(t *testing.T) {
	c := Default()
	c.Folders = []Folder{{ID: "f1", Path: "C:/x", Policy: "bad"}}
	if err := c.Validate(); err == nil {
		t.Fatal("非法 policy 应报错")
	}
}

func TestPolicyLocalExcludedFromSync(t *testing.T) {
	c := Default()
	c.Folders = []Folder{
		{ID: "f1", Path: "C:/sync", Policy: PolicySync},
		{ID: "f2", Path: "C:/local", Policy: PolicyLocal},
		{ID: "f3", Path: "C:/off", Policy: PolicyOff},
	}
	got := c.SyncedFolders()
	if len(got) != 1 || got[0].ID != "f1" {
		t.Fatalf("应仅返回 sync 策略文件夹, got %+v", got)
	}
}

func TestSaveLoadRoundtrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "asuan.json")
	c := Default()
	c.Name = "测试机"
	c.Remote = Remote{Enable: true, Endpoint: "wg.example.com", LimitKbps: 1024}
	if err := c.Save(path); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "测试机" {
		t.Fatalf("名称不一致: %q", got.Name)
	}
	if !got.Remote.Enable || got.Remote.Endpoint != "wg.example.com" || got.Remote.LimitKbps != 1024 {
		t.Fatalf("remote 配置不一致: %+v", got.Remote)
	}
}

func TestRemoteValidate(t *testing.T) {
	c := Default()
	c.Remote.LimitKbps = -1
	if err := c.Validate(); err == nil {
		t.Fatal("负限速应报错")
	}
	c = Default()
	c.Remote.Enable = true
	if err := c.Validate(); err == nil {
		t.Fatal("启用远程但无 endpoint 应报错")
	}
	c = Default()
	c.Remote.Enable = true
	c.Remote.Endpoint = "wg.example.com"
	if err := c.Validate(); err != nil {
		t.Fatalf("合法远程配置不应报错: %v", err)
	}
}
