package tray

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"time"

	"github.com/getlantern/systray"

	"github.com/suantea/asuan-keeper/internal/syncthing"
)

// Actions 托盘可执行的动作（由 cmd/asuan 注入，避免本包反向依赖 web 等）。
type Actions struct {
	// OpenConsole 打开网页控制台（进度总览页）。
	OpenConsole func()
	// OpenConfig 打开配置界面。
	OpenConfig func()
	// TogglePause 切换暂停/继续同步，返回当前是否暂停。
	TogglePause func() bool
	// OnExit 托盘退出时回调（如停止引擎、结束进程）。
	OnExit func()
}

// Enabled 返回当前环境是否支持系统托盘。
// Windows/macOS 恒 true；Linux（含 NAS/Docker 无显示器环境）需存在
// DISPLAY/WAYLAND_DISPLAY，否则 systray(GTK) 初始化失败导致进程退出
// （QNAP 容器曾因此崩溃重启循环）。
func Enabled() bool {
	switch runtime.GOOS {
	case "windows", "darwin":
		return true
	default:
		return os.Getenv("DISPLAY") != "" || os.Getenv("WAYLAND_DISPLAY") != ""
	}
}

// Run 启动托盘事件循环（阻塞，需在独立 goroutine 调用）。
// 菜单：打开控制台 / 打开配置 / 暂停-同步 / 退出。
// 交互：左键单击→查看进度（打开控制台），左键双击→打开配置，右键→菜单。
func Run(actions Actions) {
	systray.Run(func() {
		setup(actions)
	}, func() {
		if actions.OnExit != nil {
			actions.OnExit()
		}
	})
}

func setup(a Actions) {
	ico, png, err := Icon()
	if err == nil {
		if runtime.GOOS == "windows" {
			systray.SetIcon(ico)
		} else {
			systray.SetTemplateIcon(png, png)
		}
	}
	systray.SetTooltip("asuan 同步")

	// 右键菜单。
	mProgress := systray.AddMenuItem("查看同步进度", "打开网页控制台，查看正在同步的文件进度")
	mConfig := systray.AddMenuItem("打开配置", "打开配置界面")
	systray.AddSeparator()
	mPause := systray.AddMenuItemCheckbox("暂停同步", "暂停 / 继续同步", false)
	systray.AddSeparator()
	mQuit := systray.AddMenuItem("退出", "退出 asuan")

	// 左键单击/双击回调（Windows 扩展）。
	systray.SetLeftClickFn(func() {
		if a.OpenConsole != nil {
			a.OpenConsole()
		}
	})
	systray.SetDClickFn(func() {
		if a.OpenConfig != nil {
			a.OpenConfig()
		}
	})

	// 菜单事件。
	go func() {
		for range mProgress.ClickedCh {
			if a.OpenConsole != nil {
				a.OpenConsole()
			}
		}
	}()
	go func() {
		for range mConfig.ClickedCh {
			if a.OpenConfig != nil {
				a.OpenConfig()
			}
		}
	}()
	go func() {
		for range mPause.ClickedCh {
			if a.TogglePause != nil {
				p := a.TogglePause()
				SetPaused(p)
				if p {
					mPause.Check()
				} else {
					mPause.Uncheck()
				}
			}
		}
	}()
	go func() {
		for range mQuit.ClickedCh {
			systray.Quit()
		}
	}()

	// 状态 tooltip 定时刷新（同步中/已暂停/待同步）。
	go refreshTooltip(mPause)
}

// refreshTooltip 每 5 秒更新托盘 tooltip 为同步状态摘要。
func refreshTooltip(mPause *systray.MenuItem) {
	tk := time.NewTicker(5 * time.Second)
	defer tk.Stop()
	for range tk.C {
		st, err := lastStatus()
		if err != nil {
			continue
		}
		need := int64(0)
		for _, f := range st.Folders {
			need += f.NeedFiles
		}
		text := fmt.Sprintf("asuan 同步 — 文件夹 %d，待同步 %d", len(st.Folders), need)
		if paused {
			text += "（已暂停）"
			mPause.Check()
		} else {
			mPause.Uncheck()
		}
		systray.SetTooltip(text)
	}
}

// OpenBrowser 用系统默认浏览器打开 URL（跨平台）。
func OpenBrowser(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	return cmd.Start()
}

// --- 状态获取（包内共享） ---

var (
	lastSt  *syncthing.Status
	lastErr error
	paused  bool
)

// SetStatusSink 供 cmd/asuan 注入状态获取函数（避免本包持有 Manager 引用）。
var statusFn func() (*syncthing.Status, error)

// SetStatusFn 注册状态获取函数（cmd/asuan 注入）。
func SetStatusFn(fn func() (*syncthing.Status, error)) {
	statusFn = fn
}

func lastStatus() (*syncthing.Status, error) {
	if statusFn != nil {
		st, err := statusFn()
		lastSt, lastErr = st, err
		return st, err
	}
	return lastSt, lastErr
}

// SetPaused 更新暂停状态（TogglePause 时由 cmd/asuan 调用）。
func SetPaused(p bool) {
	paused = p
}

// Paused 返回当前是否处于暂停状态（供 TogglePause 决策）。
func Paused() bool {
	return paused
}
