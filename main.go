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
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	wsClient := client.New(
		cfg.WebSocketURL,
		cfg.HeartbeatInterval,
		cfg.PerformanceInterval,
		cfg.PingTimeout,
	)

	// Safe default: without POWER_MODE=real the Agent always stays in mock mode.
	var powerController client.PowerController = service.MockPowerController{}
	if strings.EqualFold(strings.TrimSpace(os.Getenv("POWER_MODE")), "real") {
		powerController = &service.WindowsPowerController{Delay: 3 * time.Second}
	}
	wsClient.ConfigurePower(powerController)
	log.Printf("power controller mode: %s", powerController.Mode())

	info := service.GetSystemInfo()
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
