สร้างระบบ Screen Streaming ฝั่ง Agent ด้วยภาษา Go สำหรับโปรเจกต์ Remote Administration ที่ใช้เพื่อดูหน้าจอเครื่อง Agent แบบ read-only เท่านั้น ไม่มีความสามารถควบคุมเมาส์หรือคีย์บอร์ด

## เป้าหมาย

Agent เชื่อมต่อกับ WebSocket Server และรอรับคำสั่งควบคุมการ Screen Streaming

Agent **ไม่ต้องส่งภาพหน้าจอตลอดเวลา**

Agent จะเริ่มส่งภาพก็ต่อเมื่อได้รับคำสั่ง:

```text
START_STREAM
```

และหยุดทันทีเมื่อได้รับ:

```text
STOP_STREAM
```

## การทำงาน

เมื่อได้รับ `START_STREAM`

ให้เริ่ม goroutine สำหรับ Screen Capture โดยทำงานประมาณ:

```text
Capture Screen
    ↓
Resize
    ↓
1280x720 หรือต่ำกว่าตาม aspect ratio
    ↓
JPEG Encode
    ↓
WebSocket Binary Message
    ↓
Server
```

Target FPS:

```text
5 FPS
```

หรือประมาณ:

```text
1 frame / 200ms
```

ใช้:

```go
time.NewTicker(200 * time.Millisecond)
```

เป็นตัวควบคุม FPS

## JPEG

ให้ Encode ภาพเป็น JPEG

ค่าเริ่มต้น:

```text
Resolution: สูงสุด 1280x720
JPEG Quality: 60
FPS: 5
```

หลีกเลี่ยงการส่งภาพ Full HD 1920x1080 ถ้าไม่จำเป็น

ก่อน Encode ให้ Resize ภาพเพื่อลด CPU, bandwidth และ memory usage

## WebSocket

JPEG ต้องส่งเป็น:

```text
WebSocket Binary Message
```

ห้าม Convert JPEG เป็น Base64

ตัวอย่าง:

```go
conn.Write(ctx, websocket.MessageBinary, jpegBytes)
```

หรือ API ที่เทียบเท่าตาม WebSocket library ที่เลือกใช้


Control Message ใช้ JSON Text Message ได้ เช่น:

```json
{
  "type": "start_stream"
}
```

```json
{
  "type": "stop_stream"
}
```

ส่วน Screen Frame ให้ใช้ WebSocket Binary

## Streaming State

Agent ต้องมี state เช่น:

```go
streaming bool
```

หรือใช้ context cancellation

ต้องป้องกันไม่ให้ `START_STREAM` ซ้ำแล้วสร้าง goroutine Capture หลายตัว

เช่นถ้ากำลัง stream อยู่:

```text
START_STREAM
START_STREAM
START_STREAM
```

ต้องมี Capture goroutine เพียงตัวเดียว

## STOP_STREAM

เมื่อได้รับ:

```json
{
  "type": "stop_stream"
}
```

ให้ยกเลิก goroutine Capture ผ่าน `context.CancelFunc`

เช่น:

```go
streamCtx, cancel := context.WithCancel(ctx)
```

เมื่อ stop:

```go
cancel()
```

แล้วหยุด:

* Screen Capture
* JPEG Encoding
* Timer
* การส่ง Binary Frame

ทันที

## Connection Lost

ถ้า WebSocket หลุด:

Agent ต้อง:

```text
STOP STREAM
↓
cleanup goroutine
↓
cleanup ticker
↓
cleanup memory
↓
reconnect server
```

ห้าม Capture หน้าจอต่อถ้า Server disconnect ไปแล้ว

## Backpressure

Agent ไม่ควรสะสมภาพใน queue

ถ้าการส่ง Frame ก่อนหน้ายังไม่เสร็จและ Capture Frame ใหม่มาแล้ว ไม่ต้องเก็บ Frame จำนวนมากรอส่ง

ใช้แนวคิด:

```text
latest frame wins
```

หรือจำกัด buffer:

```text
max queue = 1 frame
```

เพื่อป้องกัน memory usage เพิ่มขึ้นเรื่อย ๆ

เป้าหมายคือ realtime ไม่ใช่การได้รับทุก frame

## Security

Agent ต้องรับคำสั่ง Screen Stream จาก Server ที่ผ่าน Authentication แล้วเท่านั้น

ห้ามเปิด WebSocket endpoint ที่ใครก็สามารถส่ง `START_STREAM` มาได้

Screen Streaming เป็นแบบ:

```text
View Only
```

ห้าม implement:

* Mouse control
* Keyboard control
* Input injection
* Remote interaction


แยก Screen Streaming ออกจาก WebSocket connection logic

เช่น:

```go
type ScreenStreamer struct {
    mu        sync.Mutex
    streaming bool
    cancel    context.CancelFunc
}
```

มี method:

```go
Start()
Stop()
IsStreaming()
```

## สิ่งที่ต้องการจากคำตอบ

สร้างตัวอย่าง implementation ที่ใช้งานได้จริง พร้อมอธิบาย:

1. WebSocket Agent connection
2. รับ START_STREAM
3. รับ STOP_STREAM
4. Screen Capture
5. Resize เป็นประมาณ 720p
6. JPEG Encode
7. ส่ง JPEG ด้วย WebSocket Binary
8. จำกัดที่ 5 FPS
9. ป้องกัน duplicate streaming goroutine
10. cleanup เมื่อ disconnect
11. context cancellation
12. error handling
13. graceful shutdown

เน้น code ที่อ่านง่าย แยก responsibility ชัดเจน และไม่ทำให้ CPU/RAM ของ Agent ทำงานหนักเกินความจำเป็น
