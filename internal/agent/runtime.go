// Package agent owns one runtime shared by console and SCM hosts.
package agent

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"
	"ws-agent/client"
	"ws-agent/config"
	"ws-agent/download"
	"ws-agent/internal/apppaths"
	"ws-agent/internal/desktopcapture"
	"ws-agent/internal/runlock"
	"ws-agent/internal/transportpolicy"
	"ws-agent/service"
)

type Options struct {
	Service         bool
	Provision       bool
	MigrationSource string
}

func Run(ctx context.Context, options Options, ready func()) (result error) {
	machine := options.Service || options.Provision
	var paths apppaths.Paths
	var err error
	if machine {
		paths, err = apppaths.Machine()
	} else {
		var cwd string
		cwd, err = os.Getwd()
		if err == nil {
			paths, err = apppaths.Console(cwd)
		}
	}
	if err != nil {
		return err
	}
	return runWithPaths(ctx, options, paths, ready)
}

func runWithPaths(ctx context.Context, options Options, paths apppaths.Paths, ready func()) (result error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	machine := options.Service || options.Provision
	if machine {
		// Scripts establish NTFS ACLs before any sensitive state is written.
		if info, err := os.Stat(paths.Root); err != nil || !info.IsDir() {
			return fmt.Errorf("protected runtime directory is missing; run scripts/dev-service.ps1 as Administrator")
		}
	}
	unlock, err := runlock.Acquire(filepath.Join(paths.Root, ".runtime.lock"))
	if err != nil {
		return err
	}
	defer unlock()
	cfg, err := config.LoadFile(paths.Config)
	if err != nil {
		return fmt.Errorf("load runtime configuration: %w", err)
	}
	logPath := resolve(paths.Root, os.Getenv("AGENT_LOG_PATH"), paths.Log)
	if machine && !within(filepath.Join(paths.Root, "logs"), logPath) {
		return fmt.Errorf("Service log path must remain inside protected runtime logs directory")
	}
	for _, critical := range []string{paths.Identity, paths.Enrollment, paths.Config, filepath.Join(paths.Root, ".runtime.lock")} {
		if strings.EqualFold(filepath.Clean(logPath), filepath.Clean(critical)) {
			return fmt.Errorf("log path overlaps critical runtime state")
		}
	}
	closeLogs, err := service.InitLoggingAt(logPath, !options.Service)
	if err != nil {
		return fmt.Errorf("initialize file logging: %w", err)
	}
	defer closeLogs()
	if options.Service {
		log.Printf("service starting")
	}
	defer func() {
		if result != nil && ctx.Err() == nil {
			log.Printf("fatal startup/runtime error: %v", result)
		}
		log.Printf("runtime stopped")
		if options.Service {
			log.Printf("service stopped; runtime resources released")
		}
	}()
	log.Printf("runtime starting; service=%t provisioning=%t", options.Service, options.Provision)
	mode, err := transportpolicy.Mode(cfg.TransportMode, machine)
	if err != nil {
		return err
	}
	cfg.TransportMode = mode
	if err := validateEndpoints(cfg); err != nil {
		return err
	}
	log.Printf("transport mode: %s", mode)
	if options.MigrationSource != "" {
		if !options.Provision {
			return fmt.Errorf("identity migration is only available during administrative provisioning")
		}
		if !filepath.IsAbs(options.MigrationSource) {
			return fmt.Errorf("migration source must be an explicit absolute path")
		}
		if err := service.MigrateIdentity(options.MigrationSource, paths.Identity); err != nil {
			return err
		}
		log.Printf("existing identity migration verified")
	}
	loadIdentity := service.LoadIdentity
	if machine {
		loadIdentity = service.LoadServiceIdentity
	}
	identity, err := loadIdentity(paths.Identity, !options.Service)
	if err != nil {
		return err
	}
	log.Printf("agent identity successfully loaded")
	if options.Service && identity.PrivateKeyProtection == "" {
		log.Printf("legacy private-key protection: Service startup continues; explicit migration required for authenticated WebSocket connectivity")
	}
	if _, err := service.ReadEnrollment(paths.Enrollment, identity, cfg.APIBaseURL); err != nil {
		return err
	}
	if options.Provision {
		if _, err := service.EnsureEnrollment(ctx, cfg.APIBaseURL, identity, paths.Enrollment, true); err != nil {
			return err
		}
		log.Printf("administrative provisioning complete")
		return nil
	}
	wsClient := client.New(cfg.WebSocketURL, cfg.HeartbeatInterval, cfg.PerformanceInterval, cfg.PingTimeout)
	wsClient.ConfigureAuthentication(identity.AgentID, func() (ed25519.PrivateKey, error) {
		key, err := service.LoadPrivateKey(paths.Identity)
		if err != nil {
			if identity.PrivateKeyProtection == "" {
				return nil, client.ErrMigrationRequired
			}
			return nil, client.ErrPrivateKeyUnavailable
		}
		// Bind this connection to the identity loaded at startup, even if an
		// administrator changes the on-disk key while this process is running.
		if base64.StdEncoding.EncodeToString(key.Public().(ed25519.PublicKey)) != identity.PublicKey {
			clear(key)
			return nil, client.ErrPrivateKeyUnavailable
		}
		return key, nil
	})
	defer func() { log.Printf("runtime stopping; releasing workers and connections"); wsClient.Close() }()
	var power client.PowerController = &service.WindowsPowerController{Delay: 3 * time.Second}
	if strings.EqualFold(strings.TrimSpace(os.Getenv("POWER_MODE")), "mock") {
		power = service.MockPowerController{}
	}
	wsClient.ConfigurePower(power)
	log.Printf("power controller mode: %s", power.Mode())
	downloadDir := resolve(paths.Root, os.Getenv("DOWNLOAD_DIRECTORY"), paths.Downloads)
	if machine && (!within(filepath.Join(paths.Root, "data"), downloadDir) || filepath.Clean(downloadDir) == filepath.Join(paths.Root, "data")) {
		return fmt.Errorf("Service downloads must be in a subdirectory of protected runtime data")
	}
	if err := wsClient.ConfigureDownloads(download.Config{Directory: downloadDir, MaxConcurrent: cfg.MaxConcurrentDownloads, QueueSize: cfg.DownloadQueueSize, MaxFileSize: cfg.MaxDownloadSize, AllowHTTP: cfg.AllowLocalHTTPDownloads}, identity.AgentID); err != nil {
		return err
	}
	if ready != nil {
		ready()
	}
	info, err := service.EnsureEnrollment(ctx, cfg.APIBaseURL, identity, paths.Enrollment, !options.Service)
	if err != nil {
		if ctx.Err() != nil {
			return nil
		}
		return err
	}
	var capture client.ScreenCapture
	if options.Service {
		helper := desktopcapture.New()
		defer helper.Close()
		capture = helper.CaptureScreenJPEG
		log.Printf("Service mode: screen capture delegated to the interactive Desktop Helper")
	} else {
		capture = service.CaptureScreenJPEG
	}
	return wsClient.Run(ctx, info, func() (any, error) { return service.GetPerformanceInfo() }, func() (any, error) { return service.GetProcessList() }, service.KillProcess, capture)
}

func resolve(root, value, fallback string) string {
	if value == "" {
		return fallback
	}
	if filepath.IsAbs(value) {
		return filepath.Clean(value)
	}
	return filepath.Join(root, value)
}

func within(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
}

func validateEndpoints(cfg config.Config) error {
	production := cfg.TransportMode == "production"
	if production && cfg.AllowLocalHTTPDownloads {
		return fmt.Errorf("production requires ALLOW_LOCAL_HTTP_DOWNLOADS=false")
	}
	if err := transportpolicy.ValidateEndpoint(cfg.APIBaseURL, "AGENT_API_URL", false, production); err != nil {
		return err
	}
	return transportpolicy.ValidateEndpoint(cfg.WebSocketURL, "WS_SERVER_URL", true, production)
}
