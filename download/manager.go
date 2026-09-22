package download

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"ws-agent/internal/transportpolicy"

	"github.com/google/uuid"
)

type Sender func(any) error
type SpaceChecker func(string) (uint64, error)
type Config struct {
	Directory        string
	MaxConcurrent    int
	QueueSize        int
	MaxFileSize      int64
	AllowHTTP        bool
	ProgressInterval time.Duration
	HTTPClient       *http.Client
	SpaceChecker     SpaceChecker
}
type queuedJob struct {
	ctx     context.Context
	command Command
}
type Manager struct {
	ctx            context.Context
	cancel         context.CancelFunc
	wg             sync.WaitGroup
	closed         bool
	cfg            Config
	agentID        string
	send           Sender
	jobs           chan queuedJob
	mu             sync.Mutex
	active         map[string]string
	completed      map[string]Result
	completedOrder []string
}

func NewManager(cfg Config, agentID string, send Sender) (*Manager, error) {
	if strings.TrimSpace(agentID) == "" {
		return nil, fmt.Errorf("agent ID is required for file downloads")
	}
	if send == nil {
		return nil, fmt.Errorf("download event sender is required")
	}
	if cfg.MaxConcurrent <= 0 {
		cfg.MaxConcurrent = 2
	}
	if cfg.QueueSize <= 0 {
		cfg.QueueSize = 16
	}
	if cfg.MaxFileSize <= 0 {
		cfg.MaxFileSize = 10 << 30
	}
	if cfg.ProgressInterval <= 0 {
		cfg.ProgressInterval = time.Second
	}
	if cfg.HTTPClient == nil {
		cfg.HTTPClient = &http.Client{Transport: &http.Transport{DialContext: (&net.Dialer{Timeout: 15 * time.Second, KeepAlive: 30 * time.Second}).DialContext, TLSHandshakeTimeout: 15 * time.Second, ResponseHeaderTimeout: 30 * time.Second, IdleConnTimeout: 90 * time.Second}}
	}
	// Copy the injected client so a caller's client is not mutated. Preserve any
	// stricter caller policy after the mandatory transport/token boundary check.
	httpClient := *cfg.HTTPClient
	previousRedirect := httpClient.CheckRedirect
	httpClient.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if err := transportpolicy.CheckRedirect(req, via); err != nil {
			return err
		}
		if previousRedirect != nil {
			return previousRedirect(req, via)
		}
		return nil
	}
	cfg.HTTPClient = &httpClient
	if cfg.SpaceChecker == nil {
		cfg.SpaceChecker = availableDiskSpace
	}
	abs, err := filepath.Abs(cfg.Directory)
	if err != nil {
		return nil, err
	}
	cfg.Directory = abs
	if err := os.MkdirAll(abs, 0700); err != nil {
		return nil, err
	}
	m := &Manager{cfg: cfg, agentID: agentID, send: send, jobs: make(chan queuedJob, cfg.QueueSize), active: make(map[string]string), completed: make(map[string]Result)}
	m.ctx, m.cancel = context.WithCancel(context.Background())
	for i := 0; i < cfg.MaxConcurrent; i++ {
		m.wg.Add(1)
		go func() { defer m.wg.Done(); m.worker() }()
	}
	return m, nil
}

func (m *Manager) Submit(ctx context.Context, cmd Command) {
	if err := m.validate(cmd); err != nil {
		m.fail(cmd, err)
		return
	}
	attemptKey := commandAttemptKey(cmd)
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return
	}
	if result, exists := m.completed[attemptKey]; exists {
		m.mu.Unlock()
		_ = m.send(result)
		return
	}
	if status, exists := m.active[attemptKey]; exists {
		m.mu.Unlock()
		_ = m.send(Status{"FILE_DOWNLOAD_STATUS", cmd.JobID, m.agentID, status})
		return
	}
	m.active[attemptKey] = "QUEUED"
	select {
	case m.jobs <- queuedJob{ctx, cmd}:
		m.mu.Unlock()
		log.Printf("download job received job_id=%s file_id=%s filename=%q agent_id=%s", cmd.JobID, cmd.FileID, cmd.Filename, m.agentID)
	default:
		m.mu.Unlock()
		m.finish(attemptKey)
		m.fail(cmd, &JobError{Code: DownloadFailed, Message: "download queue is full"})
	}
}

// Reject reports a command that could not be decoded by the WebSocket layer.
func (m *Manager) Reject(cmd Command, message string) {
	m.fail(cmd, &JobError{Code: InvalidCommand, Message: message})
}

func (m *Manager) worker() {
	for job := range m.jobs {
		if m.ctx.Err() == nil {
			ctx, cancel := context.WithCancel(job.ctx)
			stop := context.AfterFunc(m.ctx, cancel)
			m.run(ctx, job.command)
			stop()
			cancel()
		}
		m.finish(commandAttemptKey(job.command))
	}
}

// Close cancels active I/O, discards queued work and joins every worker.
func (m *Manager) Close() {
	m.mu.Lock()
	if !m.closed {
		m.closed = true
		m.cancel()
		close(m.jobs)
	}
	m.mu.Unlock()
	m.wg.Wait()
	m.cfg.HTTPClient.CloseIdleConnections()
}

func commandAttemptKey(cmd Command) string {
	// Hash the credential-bearing URL: attempts remain distinguishable without
	// retaining a temporary token in the completed-job cache.
	sum := sha256.Sum256([]byte(cmd.DownloadURL))
	return cmd.JobID + "\x00" + hex.EncodeToString(sum[:])
}
func (m *Manager) finish(id string) { m.mu.Lock(); delete(m.active, id); m.mu.Unlock() }

func (m *Manager) remember(key string, result Result) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, exists := m.completed[key]; !exists {
		m.completedOrder = append(m.completedOrder, key)
	}
	m.completed[key] = result
	if len(m.completedOrder) > 128 {
		oldest := m.completedOrder[0]
		m.completedOrder = m.completedOrder[1:]
		delete(m.completed, oldest)
	}
}
func (m *Manager) setStatus(cmd Command, status string) {
	m.mu.Lock()
	m.active[commandAttemptKey(cmd)] = status
	m.mu.Unlock()
	_ = m.send(Status{"FILE_DOWNLOAD_STATUS", cmd.JobID, m.agentID, status})
}
func (m *Manager) fail(cmd Command, err error) {
	m.failAt(cmd, err, 0)
}

func (m *Manager) failAt(cmd Command, err error, downloaded int64) {
	code := InternalError
	message := "internal error"
	if e, ok := err.(*JobError); ok {
		code = e.Code
		message = e.Message
	}
	log.Printf("download failed job_id=%s file_id=%s filename=%q agent_id=%s code=%s error=%q", cmd.JobID, cmd.FileID, cmd.Filename, m.agentID, code, message)
	_ = m.send(Result{Type: "FILE_DOWNLOAD_RESULT", JobID: cmd.JobID, AgentID: m.agentID, FileID: cmd.FileID, Status: "FAILED", BytesDownloaded: downloaded, ErrorCode: string(code), ErrorMessage: message})
}

func (m *Manager) validate(c Command) error {
	if !strings.EqualFold(strings.TrimSpace(c.Type), "DOWNLOAD_FILE") {
		return &JobError{Code: InvalidCommand, Message: "unsupported command type"}
	}
	if _, err := uuid.Parse(c.JobID); err != nil {
		return &JobError{Code: InvalidCommand, Message: "job_id must be a valid UUID"}
	}
	if _, err := uuid.Parse(c.FileID); err != nil {
		return &JobError{Code: InvalidCommand, Message: "file_id must be a valid UUID"}
	}
	if c.Size < 0 || c.Size > m.cfg.MaxFileSize {
		return &JobError{Code: InvalidCommand, Message: "invalid job_id, file_id, or file size"}
	}
	if c.ExpiresAt.IsZero() || !time.Now().Before(c.ExpiresAt) {
		return &JobError{Code: URLExpired, Message: "temporary download URL has expired"}
	}
	if len(c.SHA256) != 64 {
		return &JobError{Code: InvalidCommand, Message: "sha256 must contain 64 hexadecimal characters"}
	}
	if _, err := hex.DecodeString(c.SHA256); err != nil {
		return &JobError{Code: InvalidCommand, Message: "sha256 is not hexadecimal"}
	}
	if c.Filename == "" || filepath.Base(c.Filename) != c.Filename || c.Filename == "." || c.Filename == ".." || strings.ContainsAny(c.Filename, "/\\") {
		return &JobError{Code: InvalidCommand, Message: "filename must be a plain file name"}
	}
	u, err := url.Parse(c.DownloadURL)
	if err != nil || u.Host == "" || u.User != nil || u.Fragment != "" {
		return &JobError{Code: InvalidCommand, Message: "download_url is invalid"}
	}
	if u.Scheme != "https" && !(u.Scheme == "http" && m.cfg.AllowHTTP && isLoopbackHost(u.Hostname())) {
		return &JobError{Code: InvalidCommand, Message: "download_url must use HTTPS (local loopback HTTP can be enabled by configuration)"}
	}
	return nil
}

func isLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func (m *Manager) run(ctx context.Context, c Command) {
	space, err := m.cfg.SpaceChecker(m.cfg.Directory)
	if err != nil {
		m.fail(c, &JobError{Code: InternalError, Message: "could not check available disk space", Err: err})
		return
	}
	if uint64(c.Size) > space {
		m.fail(c, &JobError{Code: InsufficientDiskSpace, Message: "insufficient disk space"})
		return
	}
	dest := filepath.Join(m.cfg.Directory, c.Filename)
	rel, err := filepath.Rel(m.cfg.Directory, dest)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		m.fail(c, &JobError{Code: InvalidCommand, Message: "destination escapes download directory"})
		return
	}
	m.setStatus(c, "DOWNLOADING")
	log.Printf("download started job_id=%s file_id=%s filename=%q agent_id=%s", c.JobID, c.FileID, c.Filename, m.agentID)
	resp, err := m.doRequest(ctx, c)
	if err != nil {
		if jobErr, ok := err.(*JobError); ok {
			m.fail(c, jobErr)
			return
		}
		code := DownloadFailed
		if ctx.Err() != nil {
			code = Cancelled
		}
		m.fail(c, &JobError{Code: code, Message: "HTTP download failed", Err: err})
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
		m.fail(c, &JobError{Code: HTTPError, Message: fmt.Sprintf("unexpected HTTP status %d", resp.StatusCode)})
		return
	}
	if resp.ContentLength >= 0 && resp.ContentLength != c.Size {
		m.fail(c, &JobError{Code: SizeMismatch, Message: "Content-Length does not match expected size"})
		return
	}
	f, err := os.CreateTemp(m.cfg.Directory, "."+c.Filename+"-*.part")
	if err != nil {
		m.fail(c, &JobError{Code: DiskWriteFailed, Message: "could not create temporary file", Err: err})
		return
	}
	part := f.Name()
	defer os.Remove(part)
	w := &progressWriter{writer: f, total: c.Size, interval: m.cfg.ProgressInterval, report: func(n int64, p int) { _ = m.send(Progress{"FILE_DOWNLOAD_PROGRESS", c.JobID, m.agentID, n, c.Size, p}) }}
	n, copyErr := io.Copy(w, resp.Body)
	closeErr := f.Close()
	if copyErr != nil {
		code := DownloadFailed
		if ctx.Err() != nil {
			code = Cancelled
		}
		_ = os.Remove(part)
		m.failAt(c, &JobError{Code: code, Message: "download interrupted", Err: copyErr}, n)
		return
	}
	if closeErr != nil {
		_ = os.Remove(part)
		m.failAt(c, &JobError{Code: DiskWriteFailed, Message: "could not flush temporary file", Err: closeErr}, n)
		return
	}
	w.final(n)
	if n != c.Size {
		_ = os.Remove(part)
		m.failAt(c, &JobError{Code: SizeMismatch, Message: "downloaded file size does not match"}, n)
		return
	}
	m.setStatus(c, "VERIFYING")
	actual, err := fileSHA256Context(ctx, part)
	if err != nil {
		_ = os.Remove(part)
		m.failAt(c, &JobError{Code: InternalError, Message: "could not calculate checksum", Err: err}, n)
		return
	}
	expected, _ := hex.DecodeString(c.SHA256)
	if subtle.ConstantTimeCompare(actual, expected) != 1 {
		log.Printf("hash verification failed job_id=%s file_id=%s filename=%q", c.JobID, c.FileID, c.Filename)
		_ = os.Remove(part)
		m.failAt(c, &JobError{Code: HashMismatch, Message: "downloaded file checksum does not match"}, n)
		return
	}
	if ctx.Err() != nil {
		m.failAt(c, &JobError{Code: Cancelled, Message: "download cancelled"}, n)
		return
	}
	if err := finalizeTemporary(part, dest); err != nil {
		_ = os.Remove(part)
		m.failAt(c, &JobError{Code: DiskWriteFailed, Message: "could not finalize downloaded file", Err: err}, n)
		return
	}
	hash := hex.EncodeToString(actual)
	log.Printf("download completed job_id=%s file_id=%s filename=%q agent_id=%s", c.JobID, c.FileID, c.Filename, m.agentID)
	result := Result{Type: "FILE_DOWNLOAD_RESULT", JobID: c.JobID, AgentID: m.agentID, FileID: c.FileID, Status: "COMPLETED", BytesDownloaded: n, SHA256: hash}
	m.remember(commandAttemptKey(c), result)
	_ = m.send(result)
}

func (m *Manager) doRequest(ctx context.Context, c Command) (*http.Response, error) {
	const maxAttempts = 3
	backoff := 500 * time.Millisecond
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		if !time.Now().Before(c.ExpiresAt) {
			return nil, &JobError{Code: URLExpired, Message: "temporary download URL expired before HTTP request"}
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.DownloadURL, nil)
		if err != nil {
			return nil, &JobError{Code: InvalidCommand, Message: "invalid download request", Err: err}
		}
		req.Header.Set("X-Agent-ID", m.agentID)
		resp, err := m.cfg.HTTPClient.Do(req)
		if err != nil {
			log.Printf("download transport failed (%s)", transportpolicy.FailureKind(err))
			return nil, err
		}
		if resp.StatusCode < 500 || resp.StatusCode > 599 || attempt == maxAttempts {
			return resp, nil
		}
		_ = resp.Body.Close()
		delay := backoff
		if remaining := time.Until(c.ExpiresAt); remaining <= delay {
			return nil, &JobError{Code: URLExpired, Message: "temporary download URL expired during retry backoff"}
		}
		timer := time.NewTimer(delay)
		select {
		case <-timer.C:
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		}
		backoff *= 2
	}
	return nil, &JobError{Code: InternalError, Message: "HTTP retry loop ended unexpectedly"}
}

func fileSHA256(path string) ([]byte, error) {
	return fileSHA256Context(context.Background(), path)
}

func fileSHA256Context(ctx context.Context, path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	h := sha256.New()
	if _, err = io.Copy(h, contextReader{ctx, f}); err != nil {
		return nil, err
	}
	return h.Sum(nil), nil
}

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r contextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(p)
}

type progressWriter struct {
	writer   io.Writer
	total    int64
	written  int64
	last     time.Time
	interval time.Duration
	report   func(int64, int)
}

func (p *progressWriter) Write(b []byte) (int, error) {
	n, err := p.writer.Write(b)
	p.written += int64(n)
	now := time.Now()
	if now.Sub(p.last) >= p.interval {
		p.emit()
		p.last = now
	}
	return n, err
}
func (p *progressWriter) emit() {
	percent := 0
	if p.total > 0 {
		percent = int(p.written * 100 / p.total)
	}
	if percent < 0 {
		percent = 0
	}
	if percent > 100 {
		percent = 100
	}
	p.report(p.written, percent)
}
func (p *progressWriter) final(n int64) { p.written = n; p.emit() }
