
เอกสารรับคำสั่งสแกนไวรัส quick / custom / full และส่งผลผ่าน WebSocket: [docs/virus-scan.md](docs/virus-scan.md)

• จัดโครงสร้างไฟล์ใหม่ทั้งระบบเรียบร้อยแล้ว เพื่อแยกหน้าที่และพัฒนาต่อได้ง่ายขึ้น:

  ws-agent/
  ├── main.go
  ├── agent_config.json
  ├── client/
  │   ├── client.go
  │   ├── heartbeat.go
  │   └── message.go
  ├── config/
  │   └── config.go
  ├── model/
  │   ├── system_info.go
  │   └── system_info_test.go
  └── service/
      ├── agent_id.go
      ├── network.go
      ├── system_info.go
      ├── os_version_windows.go
      └── os_version_other.go

  หน้าที่แต่ละส่วน:

  - main.go ประกอบและเริ่มระบบเท่านั้น
  - client/ จัดการ WebSocket, JSON message และ heartbeat
  - config/ เก็บค่า WebSocket URL และ timeout
  - model/ เก็บ JSON schema
  - service/ อ่าน ID, hostname, network และ OS
  - แยกการอ่าน OS version ระหว่าง Windows และระบบอื่น
  - รองรับการตั้ง server URL ผ่าน environment variable WS_SERVER_URL
  - ลบ comment ที่ encoding เสียออกแล้ว
  - JSON schema ยังคงเหมือนเดิมทุกประการ

  ตรวจสอบผ่านทั้งหมดแล้ว:

  - go test ./...
  - go vet ./...
  - go build
"# thesis-agent" 
