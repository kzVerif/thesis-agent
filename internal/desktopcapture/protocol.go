package desktopcapture

import "encoding/binary"

const (
	pipeName        = `\\.\pipe\ThesisAgentDesktop`
	captureRequest  = byte(1)
	maxFrameSize    = 8 << 20
	frameHeaderSize = 4
)

func getLength(src []byte) int {
	return int(binary.LittleEndian.Uint32(src))
}
