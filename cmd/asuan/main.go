// asuan 端上守护进程 CLI。
//
// 用法:
//
//	asuan init           生成默认配置并初始化 syncthing sidecar
//	asuan run            启动同步（前台）
//	asuan status         查看同步状态
//	asuan stop           优雅停止
//	asuan config         打印当前配置
//	asuan engine         查看引擎版本对照（agent 更新说明）
//	asuan engine-update  更新引擎（可选版本号，默认最新已验证版本）
//	asuan release <folder> [relpath]  释放文件夹或单路径（删本地实体不传播，对端保留）
//	asuan hydrate <folder> [relpath]  水合文件夹或单路径（从对端重新拉回内容）
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"atomgit.com/suantea/asuan-keeper/internal/config"
	"atomgit.com/suantea/asuan-keeper/internal/placeholder"
	"atomgit.com/suantea/asuan-keeper/internal/syncthing"
	"atomgit.com/suantea/asuan-keeper/internal/web"
)

func exeDir() string {
	exe, err := os.Executable()
	if err != nil {
		return "."
	}
	return filepath.Dir(exe)
}

func loadOrDie(path string) (*config.Config, string) {
	if path == "" {
		path = config.DefaultPath(exeDir())
	}
	cfg, err := config.Load(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "配置加载失败（%s）：%v\n先运行 `asuan init`\n", path, err)
		os.Exit(1)
	}
	return cfg, path
}

func main() {
	configPath := flag.String("config", "", "配置文件路径（默认 exe 同目录 asuan.json）")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "用法: %s [-config 路径] <init|run|status|stop|config|engine|engine-update|release|hydrate>\n", os.Args[0])
		flag.PrintDefaults()
	}
	flag.Parse()
	cmd := flag.Arg(0)
	if cmd == "" {
		flag.Usage()
		os.Exit(2)
	}

	var err error
	switch cmd {
	case "init":
		err = cmdInit()
	case "run":
		err = cmdRun(*configPath)
	case "status":
		err = cmdStatus(*configPath)
	case "stop":
		err = cmdStop(*configPath)
	case "config":
		err = cmdConfig(*configPath)
	case "engine":
		err = cmdEngine(*configPath)
	case "engine-update":
		err = cmdEngineUpdate(*configPath, flag.Args())
	case "release":
		err = cmdRelease(*configPath, flag.Args())
	case "hydrate":
		err = cmdHydrate(*configPath, flag.Args())
	default:
		flag.Usage()
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "错误:", err)
		os.Exit(1)
	}
}

func cmdInit() error {
	dir := exeDir()
	path := config.DefaultPath(dir)
	cfg := config.Default()
	cfg.Syncthing.GUIAPIKey = syncthing.RandomKey()
	if err := cfg.Save(path); err != nil {
		return err
	}
	fmt.Printf("已生成配置: %s\n", path)
	fmt.Println("提示：请编辑配置填写 name / peers(设备ID+地址) / folders(路径+策略)，再运行 `asuan run`")

	m := syncthing.New(cfg, dir)
	if err := m.Generate(); err != nil {
		fmt.Fprintln(os.Stderr, "警告：syncthing 初始化失败，请确认二进制可用：", err)
		return nil
	}
	if err := m.Start(); err != nil {
		return err
	}
	defer m.Stop()
	if err := m.WaitAPI(30 * time.Second); err != nil {
		return err
	}
	id, err := m.MyID()
	if err != nil {
		return err
	}
	fmt.Printf("本机设备 ID: %s\n", id)
	fmt.Println("将此 ID 填入其它设备的 peers，并把其它设备 ID 填入本配置的 peers。")
	return nil
}

func cmdRun(configPath string) error {
	cfg, path := loadOrDie(configPath)
	dir := exeDir()
	if cfg.Syncthing.GUIAPIKey == "" {
		cfg.Syncthing.GUIAPIKey = syncthing.RandomKey()
		_ = cfg.Save(path)
	}
	m := syncthing.New(cfg, dir)
	if !m.ConfigExists() {
		if err := m.Generate(); err != nil {
			return err
		}
	}
	if err := m.Start(); err != nil {
		return err
	}
	if err := m.WaitAPI(30 * time.Second); err != nil {
		return err
	}
	// 应用配置并重启使生效（含 Windows 锁竞态回收）。
	if err := m.Reload(); err != nil {
		return fmt.Errorf("应用同步配置失败: %w", err)
	}
	// 内置网页控制台：客户端默认仅本机访问。
	ui := web.New(cfg, path, m)
	if err := ui.Start(); err != nil {
		return fmt.Errorf("网页控制台启动失败: %w", err)
	}
	fmt.Printf("asuan 同步运行中 (设备 %s, GUI %s, Web %s)\n", cfg.Name, cfg.Syncthing.GUIBind, cfg.Web.Bind)

	// 占位符虚拟层：配置了挂载点时，把已释放文件夹以虚拟目录挂出
	// （访问即水合）。需 WinFsp/macFUSE/FUSE 运行时，失败仅告警不阻断。
	if cfg.Placeholder.Mount != "" {
		if err := startPlaceholderMount(cfg, m); err != nil {
			fmt.Fprintf(os.Stderr, "警告: 占位符虚拟层挂载失败: %v\n", err)
		}
	}
	return m.Wait()
}

// startPlaceholderMount 挂载占位符虚拟层（阻塞，独立协程内运行）。
func startPlaceholderMount(cfg *config.Config, m *syncthing.Manager) error {
	lister := placeholder.NewReleaseLister(cfg, m)
	ph := placeholder.NewPlaceholderFS(lister, "", func(rel string) error {
		// 打开占位文件 → 只水合该文件/子路径（单文件级；空子路径=文件夹级）。
		folderID, sub := lister.SplitVirt(rel)
		return placeholder.Hydrate(cfg, m, folderID, sub, 10*time.Minute)
	}).SetResolver(lister.Resolve)
	go func() {
		if err := ph.Mount(cfg.Placeholder.Mount); err != nil {
			fmt.Fprintf(os.Stderr, "警告: 占位符虚拟层已退出: %v\n", err)
		}
	}()
	fmt.Printf("占位符虚拟层已挂载: %s（释放的文件夹将在此显示，访问即水合）\n", cfg.Placeholder.Mount)
	return nil
}

func cmdStatus(configPath string) error {
	cfg, _ := loadOrDie(configPath)
	m := syncthing.New(cfg, exeDir())
	st, err := m.Status()
	if err != nil {
		return err
	}
	if !st.Running {
		fmt.Println("未运行")
		return nil
	}
	fmt.Printf("syncthing %s\n设备 ID: %s\n", st.Version, st.MyID)
	for _, f := range st.Folders {
		fmt.Printf("  [%s] %s  状态=%s 全局=%d 项/%.1f MB  本地=%d 项  待同步=%d\n",
			f.ID, f.Path, f.State, f.GlobalTotalItems, float64(f.GlobalBytes)/1e6, f.LocalTotalItems, f.NeedFiles)
	}
	return nil
}

func cmdStop(configPath string) error {
	cfg, _ := loadOrDie(configPath)
	m := syncthing.New(cfg, exeDir())
	return m.Stop()
}

func cmdConfig(configPath string) error {
	cfg, _ := loadOrDie(configPath)
	b, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(b))
	return nil
}

func cmdEngine(configPath string) error {
	cfg, _ := loadOrDie(configPath)
	m := syncthing.New(cfg, exeDir())
	info, err := m.EngineCheck()
	if err != nil {
		return err
	}
	fmt.Printf("引擎版本对照 / agent 更新说明\n")
	fmt.Printf("  已安装引擎: %s\n", orDash(info.Installed))
	fmt.Printf("  当前推荐:   %s\n", info.Current)
	fmt.Printf("  已验证版本: %s\n", strings.Join(info.Verified, ", "))
	fmt.Println()
	fmt.Println("说明：syncthing 作为 sidecar 内核独立于 asuan，仅通过 REST API 与配置格式交互，")
	fmt.Println("因此引擎小版本升级一般无需改 asuan 代码。升级方式：")
	fmt.Println("  asuan engine-update           # 更新到推荐版本")
	fmt.Println("  asuan engine-update v2.2.0    # 指定版本")
	fmt.Println("升级后建议 `asuan run` + `asuan status` 冒烟确认。")
	fmt.Println("GitHub 下载受限时，用环境变量指定镜像：")
	fmt.Println("  ASUAN_ENGINE_BASE=https://ghproxy.net/https://github.com/syncthing/syncthing/releases/download")
	return nil
}

func cmdEngineUpdate(configPath string, args []string) error {
	cfg, _ := loadOrDie(configPath)
	m := syncthing.New(cfg, exeDir())
	version := ""
	if len(args) > 1 {
		version = args[1]
	}
	v, err := m.DownloadAndUpdate(version)
	if err != nil {
		return err
	}
	fmt.Printf("引擎已更新到 %s（旧版本备份为 %s.bak）\n", v, m.Exe)
	fmt.Println("若同步正在运行，请重启：asuan stop && asuan run")
	return nil
}

// cmdRelease 释放文件夹或单路径：删本地实体不传播，对端内容保留。
// relpath 为空=文件夹级；非空=单文件/单目录级。
func cmdRelease(configPath string, args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("用法: asuan release <folder-id> [relpath]")
	}
	cfg, _ := loadOrDie(configPath)
	m := syncthing.New(cfg, exeDir())
	folderID := args[1]
	relPath := ""
	if len(args) > 2 {
		relPath = args[2]
	}
	if err := placeholder.Release(cfg, m, folderID, relPath); err != nil {
		return err
	}
	if relPath == "" {
		fmt.Printf("已释放文件夹 %s：本地实体已删除，对端内容保留。\n", folderID)
	} else {
		fmt.Printf("已释放 %s 的 %s：本地实体已删除，对端内容保留。\n", folderID, relPath)
	}
	fmt.Println("需要恢复时运行：asuan hydrate " + folderID + " " + relPath)
	return nil
}

// cmdHydrate 水合文件夹或单路径：从对端重新拉回内容。
// relpath 为空=文件夹级；非空=单文件/单目录级。
func cmdHydrate(configPath string, args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("用法: asuan hydrate <folder-id> [relpath]")
	}
	cfg, _ := loadOrDie(configPath)
	m := syncthing.New(cfg, exeDir())
	folderID := args[1]
	relPath := ""
	if len(args) > 2 {
		relPath = args[2]
	}
	if err := placeholder.Hydrate(cfg, m, folderID, relPath, 10*time.Minute); err != nil {
		return err
	}
	if relPath == "" {
		fmt.Printf("已水合文件夹 %s：内容已从对端拉回。\n", folderID)
	} else {
		fmt.Printf("已水合 %s 的 %s：内容已从对端拉回。\n", folderID, relPath)
	}
	return nil
}

func orDash(s string) string {
	if s == "" {
		return "—（引擎未运行，先 asuan run）"
	}
	return s
}
