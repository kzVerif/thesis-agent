package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

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
	wsClient.ConfigurePower(service.MockPowerController{})
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
