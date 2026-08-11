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

	"atomgit.com/suantea/asuan-keeper/internal/config"
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
	stj(w, map[string]any{
		"name":    s.cfg.Name,
		"running": st.Running,
		"version": st.Version,
		"myID":    st.MyID,
		"folders": st.Folders,
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

func stj(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(v)
}
