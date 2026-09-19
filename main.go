package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"ws-agent/client"
	"ws-agent/config"
	"ws-agent/download"
	"ws-agent/service"
)

func main() {
	cfg := config.Load()
	closeLogs, err := service.InitLogging()
	if err != nil {
		log.Printf("logging initialization failed: %v", err)
	} else {
		defer closeLogs()
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	info, err := service.EnsureRegistered(ctx, cfg.APIBaseURL)
	if err != nil {
		log.Fatal(err)
	}
	service.DetachConsole()

	wsClient := client.New(
		cfg.WebSocketURL,
		cfg.HeartbeatInterval,
		cfg.PerformanceInterval,
		cfg.PingTimeout,
	)

	// Default to real power control. Set POWER_MODE=mock for safe testing.
	var powerController client.PowerController = &service.WindowsPowerController{
		Delay: 3 * time.Second,
	}

	if strings.EqualFold(strings.TrimSpace(os.Getenv("POWER_MODE")), "mock") {
		powerController = service.MockPowerController{}
	}
	wsClient.ConfigurePower(powerController)
	log.Printf("power controller mode: %s", powerController.Mode())

	if err := wsClient.ConfigureDownloads(download.Config{Directory: cfg.DownloadDirectory, MaxConcurrent: cfg.MaxConcurrentDownloads, QueueSize: cfg.DownloadQueueSize, MaxFileSize: cfg.MaxDownloadSize, AllowHTTP: cfg.AllowLocalHTTPDownloads}, info.ID); err != nil {
		log.Fatal(err)
	}
	performanceProvider := func() (any, error) {
		return service.GetPerformanceInfo()
	}
	processProvider := func() (any, error) {
		return service.GetProcessList()
	}
	if err := wsClient.Run(ctx, info, performanceProvider, processProvider, service.KillProcess, service.CaptureScreenJPEG); err != nil {
		log.Fatal(err)
	}
}
