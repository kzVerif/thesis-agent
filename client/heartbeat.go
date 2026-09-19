package client

import (
	"context"
	"log"
	"time"
)

func (c *Client) heartbeat(ctx context.Context, conn *safeConnection) {
	ticker := time.NewTicker(c.heartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			pingCtx, cancel := context.WithTimeout(ctx, c.pingTimeout)
			err := conn.ping(pingCtx)
			cancel()
			if err != nil {
				log.Printf("ping failed: %v", err)
				return
			}
			log.Printf("ping ok")
		case <-ctx.Done():
			return
		}
	}
}
