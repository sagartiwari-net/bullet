package runner

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"authchecker/internal/cf"
	"authchecker/internal/config"
)

type Stats struct {
	Total    int64 `json:"total"`
	Checked  int64 `json:"checked"`
	Hits     int64 `json:"hits"`
	Fails    int64 `json:"fails"`
	Retries  int64 `json:"retries"`
	Errors   int64 `json:"errors"`
	CPM      int64 `json:"cpm"`
	Running  bool  `json:"running"`
	Started  int64 `json:"started_at"`
}

type Engine struct {
	cfg       *config.Config
	cfManager *cf.Manager
	maxBots   int

	mu      sync.Mutex
	stats   Stats
	results []CheckResult
	cancel  context.CancelFunc
	onHit   func(CheckResult)
}

func NewEngine(cfg *config.Config, cfManager *cf.Manager, maxBots int) *Engine {
	if maxBots <= 0 {
		maxBots = 10
	}
	return &Engine{
		cfg:       cfg,
		cfManager: cfManager,
		maxBots:   maxBots,
	}
}

func (e *Engine) SetOnHit(fn func(CheckResult)) {
	e.onHit = fn
}

func (e *Engine) Stats() Stats {
	e.mu.Lock()
	defer e.mu.Unlock()
	s := e.stats
	if s.Running && s.Started > 0 {
		elapsed := time.Now().Unix() - s.Started
		if elapsed > 0 {
			s.CPM = (s.Checked * 60) / elapsed
		}
	}
	return s
}

func (e *Engine) Results() []CheckResult {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := make([]CheckResult, len(e.results))
	copy(out, e.results)
	return out
}

func (e *Engine) IsRunning() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.stats.Running
}

func (e *Engine) Stop() {
	e.mu.Lock()
	cancel := e.cancel
	e.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

func (e *Engine) Run(ctx context.Context, wordlistPath string, proxies []string) error {
	e.mu.Lock()
	if e.stats.Running {
		e.mu.Unlock()
		return fmt.Errorf("already running")
	}
	ctx, cancel := context.WithCancel(ctx)
	e.cancel = cancel
	e.stats = Stats{Running: true, Started: time.Now().Unix()}
	e.results = nil
	e.mu.Unlock()

	lines, err := loadWordlist(wordlistPath)
	if err != nil {
		e.finish()
		return err
	}

	e.mu.Lock()
	e.stats.Total = int64(len(lines))
	e.mu.Unlock()

	sem := make(chan struct{}, e.maxBots)
	var wg sync.WaitGroup
	bot := NewBot(e.cfg, e.cfManager)

	for i, line := range lines {
		select {
		case <-ctx.Done():
			wg.Wait()
			e.finish()
			return nil
		default:
		}

		email, password, ok := config.ParseWordlistLine(line, e.cfg.WordlistFormat)
		if !ok {
			atomic.AddInt64(&e.stats.Checked, 1)
			continue
		}

		proxyURL := ""
		if len(proxies) > 0 {
			proxyURL = proxies[i%len(proxies)]
		}

		wg.Add(1)
		sem <- struct{}{}
		go func(email, password, proxy string, lineNum int) {
			defer wg.Done()
			defer func() { <-sem }()

			input := CheckInput{
				Email:    email,
				Password: password,
				ProxyURL: proxy,
				LineNum:  lineNum,
			}

			result := bot.Check(ctx, input)
			e.record(result)
		}(email, password, proxyURL, i+1)
	}

	wg.Wait()
	e.finish()
	return nil
}

func (e *Engine) record(r CheckResult) {
	atomic.AddInt64(&e.stats.Checked, 1)
	switch r.Status {
	case "HIT":
		atomic.AddInt64(&e.stats.Hits, 1)
	case "FAIL":
		atomic.AddInt64(&e.stats.Fails, 1)
	case "RETRY":
		atomic.AddInt64(&e.stats.Retries, 1)
	default:
		if r.Status == "ERROR" {
			atomic.AddInt64(&e.stats.Errors, 1)
		}
	}

	e.mu.Lock()
	if r.Status == "HIT" {
		e.results = append(e.results, r)
	}
	e.mu.Unlock()

	if r.Status == "HIT" && e.onHit != nil {
		e.onHit(r)
	}
}

func (e *Engine) finish() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.stats.Running = false
	e.cancel = nil
}

func loadWordlist(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var lines []string
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "" || strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}
		lines = append(lines, line)
	}
	return lines, scanner.Err()
}

func LoadProxies(path string) ([]string, error) {
	if path == "" {
		return nil, nil
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var proxies []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" && !strings.HasPrefix(line, "#") {
			proxies = append(proxies, line)
		}
	}
	return proxies, scanner.Err()
}
