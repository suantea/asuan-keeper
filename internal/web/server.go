// Package web 提供内置网页控制台：状态总览 + 配置编辑。
// 客户端默认绑 127.0.0.1 仅本机访问，hub 绑 0.0.0.0 供局域网访问。
package web

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"

	"atomgit.com/suantea/asuan-keeper/internal/config"
	"atomgit.com/suantea/asuan-keeper/internal/placeholder"
	"atomgit.com/suantea/asuan-keeper/internal/syncthing"
)

//go:embed index.html
var indexHTML string

type Server struct {
	mu  sync.RWMutex
	cfg *config.Config
	// path 是 asuan.json 路径（保存用），mgr 是当前引擎管理器。
	path string
	mgr  *syncthing.Manager
}

func New(cfg *config.Config, path string, mgr *syncthing.Manager) *Server {
	return &Server{cfg: cfg, path: path, mgr: mgr}
}

func (s *Server) Start() error {
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleIndex)
	mux.HandleFunc("/api/status", s.handleStatus)
	mux.HandleFunc("/api/config", s.handleConfig)
	mux.HandleFunc("/api/reload", s.handleReload)
	mux.HandleFunc("/api/release", s.handleRelease)
	mux.HandleFunc("/api/hydrate", s.handleHydrate)
	mux.HandleFunc("/api/release-many", s.handleReleaseMany)
	mux.HandleFunc("/api/hydrate-many", s.handleHydrateMany)

	ln, err := net.Listen("tcp", s.cfg.Web.Bind)
	if err != nil {
		return fmt.Errorf("web 监听 %s 失败: %w", s.cfg.Web.Bind, err)
	}
	go http.Serve(ln, mux)
	return nil
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, indexHTML)
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	mgr := s.mgr
	s.mu.RUnlock()
	st, err := mgr.Status()
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	// 标记每个文件夹是否处于释放状态（占位符：本地实体已删、对端保留）。
	folders := make([]map[string]any, 0, len(st.Folders))
	for _, f := range st.Folders {
		released, _ := placeholder.IsReleased(f.Path)
		folders = append(folders, map[string]any{
			"id": f.ID, "label": f.Label, "path": f.Path, "state": f.State,
			"globalFiles": f.GlobalFiles, "globalBytes": f.GlobalBytes,
			"localFiles": f.LocalFiles, "localBytes": f.LocalBytes,
			"needFiles": f.NeedFiles, "globalTotalItems": f.GlobalTotalItems,
			"localTotalItems": f.LocalTotalItems, "released": released,
		})
	}
	stj(w, map[string]any{
		"name":    s.cfg.Name,
		"running": st.Running,
		"version": st.Version,
		"myID":    st.MyID,
		"folders": folders,
		"peers":   st.Peers,
	})
}

func (s *Server) handleConfig(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.mu.RLock()
		defer s.mu.RUnlock()
		stj(w, s.cfg)
	case http.MethodPost:
		var nc config.Config
		if err := json.NewDecoder(r.Body).Decode(&nc); err != nil {
			http.Error(w, "配置 JSON 解析失败: "+err.Error(), 400)
			return
		}
		if err := nc.Validate(); err != nil {
			http.Error(w, "配置校验失败: "+err.Error(), 400)
			return
		}
		if nc.Web.Bind == "" {
			nc.Web.Bind = "127.0.0.1:18084"
		}
		if err := nc.Save(s.path); err != nil {
			http.Error(w, "保存失败: "+err.Error(), 500)
			return
		}
		s.mu.Lock()
		s.cfg = &nc
		m := s.mgr
		s.mu.Unlock()
		m.Cfg = &nc
		if err := m.Reload(); err != nil {
			stj(w, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		stj(w, map[string]any{"ok": true})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleReload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	s.mu.RLock()
	mgr := s.mgr
	s.mu.RUnlock()
	if err := mgr.Reload(); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	stj(w, map[string]any{"ok": true})
}

// folderIDFromBody 解析请求体中的 {folder: id, relpath?: path}。
func folderIDFromBody(r *http.Request) (string, string, error) {
	var b struct {
		Folder  string `json:"folder"`
		RelPath string `json:"relpath"`
	}
	if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
		return "", "", err
	}
	if b.Folder == "" {
		return "", "", fmt.Errorf("缺少 folder 参数")
	}
	return b.Folder, b.RelPath, nil
}

// handleRelease 释放文件夹或单路径：删本地实体不传播（占位符），对端保留。
func (s *Server) handleRelease(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	folderID, relPath, err := folderIDFromBody(r)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	s.mu.RLock()
	cfg, mgr := s.cfg, s.mgr
	s.mu.RUnlock()
	if err := placeholder.Release(cfg, mgr, folderID, relPath); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	stj(w, map[string]any{"ok": true})
}

// handleHydrate 水合文件夹或单路径：移除释放规则，从对端重新拉回内容。
func (s *Server) handleHydrate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	folderID, relPath, err := folderIDFromBody(r)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	s.mu.RLock()
	cfg, mgr := s.cfg, s.mgr
	s.mu.RUnlock()
	if err := placeholder.Hydrate(cfg, mgr, folderID, relPath, 10*time.Minute); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	stj(w, map[string]any{"ok": true})
}

func stj(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(v)
}

// foldersFromBody 解析批量操作请求体（folder id 数组）。
func foldersFromBody(r *http.Request) ([]string, error) {
	var b struct {
		Folders []string `json:"folders"`
	}
	if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
		return nil, err
	}
	if len(b.Folders) == 0 {
		return nil, fmt.Errorf("缺少 folders 参数")
	}
	return b.Folders, nil
}

// batchOp 并发执行释放/水合（限并发，汇总每个 folder 的结果）。
func (s *Server) batchOp(w http.ResponseWriter, r *http.Request, hydrate bool) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	folders, err := foldersFromBody(r)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	s.mu.RLock()
	cfg, mgr := s.cfg, s.mgr
	s.mu.RUnlock()

	const maxConcurrent = 3 // 并发上限，避免同时水合压垮对端/本地 IO
	sem := make(chan struct{}, maxConcurrent)
	results := make([]map[string]any, len(folders))
	var wg sync.WaitGroup
	for i, id := range folders {
		wg.Add(1)
		go func(i int, id string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			res := map[string]any{"folder": id}
			var err error
			if hydrate {
				err = placeholder.Hydrate(cfg, mgr, id, "", 10*time.Minute)
			} else {
				err = placeholder.Release(cfg, mgr, id, "")
			}
			if err != nil {
				res["ok"] = false
				res["error"] = err.Error()
			} else {
				res["ok"] = true
			}
			results[i] = res
		}(i, id)
	}
	wg.Wait()
	stj(w, map[string]any{"ok": true, "results": results})
}

// handleReleaseMany 批量释放文件夹。
func (s *Server) handleReleaseMany(w http.ResponseWriter, r *http.Request) {
	s.batchOp(w, r, false)
}

// handleHydrateMany 批量水合文件夹。
func (s *Server) handleHydrateMany(w http.ResponseWriter, r *http.Request) {
	s.batchOp(w, r, true)
}
