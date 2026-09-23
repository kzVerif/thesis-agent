//go:build !windows

package desktopcapture

import "fmt"

type Client struct{}

func New() *Client { return &Client{} }

func (c *Client) CaptureScreenJPEG() ([]byte, error) {
	return nil, fmt.Errorf("desktop capture helper is only supported on Windows")
}

func (c *Client) Close() {}
