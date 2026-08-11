package syncthing

import (
	"archive/zip"
	"os"
	"path/filepath"
	"testing"
)

// TestExtractZipFindsBinary 用与官方发行包一致的结构验证 agent 更新解压逻辑。
func TestExtractZipFindsBinary(t *testing.T) {
	dir := t.TempDir()
	zpath := filepath.Join(dir, "engine.zip")
	f, err := os.Create(zpath)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	add := func(name, content string) {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		w.Write([]byte(content))
	}
	add("syncthing-windows-amd64-v2.1.3/syncthing.exe", "MZ-binary-bytes")
	add("syncthing-windows-amd64-v2.1.3/LICENSE.txt", "MPL-2.0 text")
	add("syncthing-windows-amd64-v2.1.3/README.txt", "readme")
	zw.Close()
	f.Close()

	got, err := extractZip(zpath)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(got) != "syncthing" {
		t.Fatalf("未命中引擎二进制: %s", got)
	}
	b, err := os.ReadFile(got + ".exe")
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != "MZ-binary-bytes" {
		t.Fatalf("内容不符: %q", b)
	}
}

// TestEngineCheck 验证版本对照元数据。
func TestEngineCheck(t *testing.T) {
	info := &EngineInfo{Current: CurrentEngineVersion, Verified: VerifiedVersions}
	if info.Current == "" {
		t.Fatal("CurrentEngineVersion 不能为空")
	}
	if len(info.Verified) == 0 {
		t.Fatal("VerifiedVersions 不能为空")
	}
	if !containsStr(info.Verified, info.Current) {
		t.Fatalf("推荐版本 %s 应在已验证清单中: %v", info.Current, info.Verified)
	}
}

func containsStr(ss []string, s string) bool {
	for _, x := range ss {
		if x == s {
			return true
		}
	}
	return false
}
