//go:build windows

package desktopcapture

import (
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"os"
	"unsafe"
	"ws-agent/service"

	"golang.org/x/sys/windows"
)

// Run starts the per-user Desktop Helper. It intentionally has no server
// credentials and accepts only the fixed capture request over the local pipe.
func Run() error {
	log.Printf("desktop capture helper listening on %s", pipeName)
	for {
		if err := serveConnection(); err != nil {
			log.Printf("desktop helper connection ended: %v", err)
		}
	}
}

func serveConnection() error {
	name, err := windows.UTF16PtrFromString(pipeName)
	if err != nil {
		return err
	}
	sd, err := windows.SecurityDescriptorFromString("D:P(A;;GA;;;SY)(A;;GA;;;BA)(A;;GA;;;IU)")
	if err != nil {
		return fmt.Errorf("create desktop helper ACL: %w", err)
	}
	sa := &windows.SecurityAttributes{
		Length:             uint32(unsafe.Sizeof(windows.SecurityAttributes{})),
		SecurityDescriptor: sd,
	}
	handle, err := windows.CreateNamedPipe(
		name,
		windows.PIPE_ACCESS_DUPLEX,
		windows.PIPE_TYPE_BYTE|windows.PIPE_READMODE_BYTE|windows.PIPE_WAIT,
		1,
		maxFrameSize,
		frameHeaderSize,
		0,
		sa,
	)
	if err != nil {
		return fmt.Errorf("create desktop helper pipe: %w", err)
	}
	file := os.NewFile(uintptr(handle), "thesis-agent-desktop-pipe")
	if file == nil {
		_ = windows.CloseHandle(handle)
		return fmt.Errorf("create desktop helper pipe file")
	}
	defer file.Close()

	err = windows.ConnectNamedPipe(handle, nil)
	if err != nil && err != windows.ERROR_PIPE_CONNECTED {
		return fmt.Errorf("wait for Service: %w", err)
	}
	request := []byte{0}
	header := make([]byte, frameHeaderSize)
	for {
		if _, err := io.ReadFull(file, request); err != nil {
			return err
		}
		if request[0] != captureRequest {
			return fmt.Errorf("unsupported desktop helper request %d", request[0])
		}
		frame, err := service.CaptureScreenJPEG()
		if err != nil || len(frame) == 0 || len(frame) > maxFrameSize {
			binary.LittleEndian.PutUint32(header, 0)
			if _, writeErr := file.Write(header); writeErr != nil {
				return writeErr
			}
			if err != nil {
				log.Printf("desktop capture failed: %v", err)
			}
			continue
		}
		binary.LittleEndian.PutUint32(header, uint32(len(frame)))
		if _, err := file.Write(header); err != nil {
			return err
		}
		if _, err := file.Write(frame); err != nil {
			return err
		}
	}
}
