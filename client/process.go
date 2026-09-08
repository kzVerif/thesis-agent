package client

import (
	"context"
)

func (c *Client) publishProcess(
	ctx context.Context,
	conn *safeConnection,
	provider MessageProvider,
	commands <-chan bool,
) {
	c.publishOnCommand(ctx, conn, provider, commands, "process")
}
