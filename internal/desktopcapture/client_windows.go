//go:build windows

package desktopcapture

import (
	"fmt"
	"io"
	"os"
	"sync"
)

// Client requests frames from the Desktop Helper running in the interactive
// user's session. The service keeps the WebSocket and authentication; the
// helper only captures pixels.
type Client struct {
	mu   sync.Mutex
	pipe *os.File
}

func New() *Client { return &Client{} }

func (c *Client) CaptureScreenJPEG() ([]byte, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if err := c.ensurePipe(); err != nil {
		return nil, err
	}
	if _, err := c.pipe.Write([]byte{captureRequest}); err != nil {
		c.closePipe()
		return nil, fmt.Errorf("write desktop capture request: %w", err)
	}
	header := make([]byte, frameHeaderSize)
	if _, err := io.ReadFull(c.pipe, header); err != nil {
		c.closePipe()
		return nil, fmt.Errorf("read desktop capture response: %w", err)
	}
	length := getLength(header)
	if length <= 0 || length > maxFrameSize {
		c.closePipe()
		return nil, fmt.Errorf("desktop helper returned invalid frame length %d", length)
	}
	frame := make([]byte, length)
	if _, err := io.ReadFull(c.pipe, frame); err != nil {
		c.closePipe()
		return nil, fmt.Errorf("read desktop capture frame: %w", err)
	}
	return frame, nil
}

func (c *Client) ensurePipe() error {
	if c.pipe != nil {
		return nil
	}
	pipe, err := os.OpenFile(pipeName, os.O_RDWR, 0)
	if err != nil {
		return fmt.Errorf("desktop helper unavailable: %w", err)
	}
	c.pipe = pipe
	return nil
}

func (c *Client) closePipe() {
	if c.pipe != nil {
		_ = c.pipe.Close()
		c.pipe = nil
	}
}

func (c *Client) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.closePipe()
}
