package client

import (
	"context"
	"log"
	"time"
)

func (c *Client) publishPerformance(
	ctx context.Context,
	conn *safeConnection,
	provider MessageProvider,
	commands <-chan bool,
) {
	c.publishOnCommand(ctx, conn, provider, commands, "performance")
}

func (c *Client) publishOnCommand(
	ctx context.Context,
	conn *safeConnection,
	provider MessageProvider,
	commands <-chan bool,
	messageName string,
) {
	var ticker *time.Ticker
	var ticks <-chan time.Time
	active := false

	stopTicker := func() {
		if ticker != nil {
			ticker.Stop()
			ticker = nil
			ticks = nil
		}
	}
	defer stopTicker()

	send := func() bool {
		message, err := provider()
		if err != nil {
			log.Printf("collect %s failed: %v", messageName, err)
			return true
		}
		if err := writeJSON(ctx, conn, message); err != nil {
			log.Printf("send %s failed: %v", messageName, err)
			return false
		}
		return true
	}

	for {
		select {
		case start := <-commands:
			if start == active {
				continue
			}
			active = start
			if !active {
				stopTicker()
				continue
			}
			if !send() {
				return
			}
			ticker = time.NewTicker(c.performanceInterval)
			ticks = ticker.C
		case <-ticks:
			if !send() {
				return
			}
		case <-ctx.Done():
			return
		}
	}
}
