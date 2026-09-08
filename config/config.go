package config

import (
	"bufio"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const defaultWebSocketURL = "ws://localhost:8081/ws"

type Config struct {
	WebSocketURL            string
	HeartbeatInterval       time.Duration
	PerformanceInterval     time.Duration
	PingTimeout             time.Duration
	DownloadDirectory       string
	MaxConcurrentDownloads  int
	DownloadQueueSize       int
	MaxDownloadSize         int64
	AllowLocalHTTPDownloads bool
}

func Load() Config {
	// Load developer settings from .env without replacing variables explicitly
	// supplied by the process, service manager, container, or CI environment.
	_ = loadDotEnv(".env")
	url := os.Getenv("WS_SERVER_URL")
	if url == "" {
		url = defaultWebSocketURL
	}

	maxSize := int64(10 << 30)
	if value, err := strconv.ParseInt(os.Getenv("MAX_DOWNLOAD_SIZE_BYTES"), 10, 64); err == nil && value > 0 {
		maxSize = value
	}
	concurrency := 2
	if value, err := strconv.Atoi(os.Getenv("MAX_CONCURRENT_DOWNLOADS")); err == nil && value > 0 {
		concurrency = value
	}
	queueSize := 16
	if value, err := strconv.Atoi(os.Getenv("DOWNLOAD_QUEUE_SIZE")); err == nil && value > 0 {
		queueSize = value
	}
	downloadDir := os.Getenv("DOWNLOAD_DIRECTORY")
	if downloadDir == "" {
		downloadDir = filepath.Join("data", "downloads")
	}
	return Config{
		WebSocketURL:            url,
		HeartbeatInterval:       20 * time.Second,
		PerformanceInterval:     5 * time.Second,
		PingTimeout:             5 * time.Second,
		DownloadDirectory:       downloadDir,
		MaxConcurrentDownloads:  concurrency,
		DownloadQueueSize:       queueSize,
		MaxDownloadSize:         maxSize,
		AllowLocalHTTPDownloads: parseBool(os.Getenv("ALLOW_LOCAL_HTTP_DOWNLOADS")),
	}
}

func parseBool(value string) bool {
	enabled, err := strconv.ParseBool(strings.TrimSpace(value))
	return err == nil && enabled
}

func loadDotEnv(path string) error {
	file, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimSpace(strings.TrimPrefix(line, "export "))
		key, value, found := strings.Cut(line, "=")
		key = strings.TrimSpace(key)
		if !found || key == "" {
			continue
		}
		value = strings.TrimSpace(value)
		if len(value) >= 2 && ((value[0] == '\'' && value[len(value)-1] == '\'') || (value[0] == '"' && value[len(value)-1] == '"')) {
			value = value[1 : len(value)-1]
		}
		if _, exists := os.LookupEnv(key); !exists {
			_ = os.Setenv(key, value)
		}
	}
	return scanner.Err()
}
