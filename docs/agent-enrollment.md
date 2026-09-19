# Agent Enrollment และ Identity

เอกสารนี้อธิบายการสมัคร Agent ครั้งแรกกับ Server และการเก็บ identity ของ Agent ในฝั่ง Windows

## ภาพรวม

เมื่อ Agent เริ่มทำงาน จะทำตามลำดับนี้:

```text
โหลดหรือสร้าง agent_config.json
        |
        v
GET /api/agents/{agent_id}/exists
        |
        +-- 204: Agent มีอยู่แล้ว -> ข้ามการสมัคร
        |
        +-- 404: Agent ยังไม่มี -> ขอ registration token จาก CLI
                         |
                         v
                 POST /api/agents/register
                         |
                         v
                 เริ่มทำงานตามปกติแบบ background
```

การตรวจสอบ Agent ใช้ `agent_id` เป็นหลัก ไม่ใช้ IP หรือ MAC address เป็นตัวตนถาวรของเครื่อง

## การสร้าง Key Pair

Agent ใช้ algorithm:

```text
Ed25519
```

เมื่อยังไม่มี key pair ระบบจะสร้าง:

- `public_key` สำหรับส่งให้ Server
- `private_key` สำหรับเก็บไว้ในเครื่อง Agent

Private key จะไม่ถูกส่งไป Server

## การป้องกัน Private Key ด้วย DPAPI

บน Windows private key จะถูกป้องกันด้วย Windows DPAPI ก่อนเขียนลงไฟล์:

```text
Ed25519 private key
        |
        v
CryptProtectData
        |
        v
encrypted_private_key
```

โค้ดอยู่ที่:

```text
service/dpapi_windows.go
```

การทำงานปัจจุบันใช้ DPAPI แบบผูกกับ Windows user/service account ที่รัน Agent อยู่ หากเปลี่ยน account หรือเครื่อง อาจไม่สามารถถอดรหัส private key เดิมได้

## รูปแบบ `agent_config.json`

ตัวอย่างไฟล์:

```json
{
  "agent_id": "11111111-1111-1111-1111-111111111111",
  "algorithm": "Ed25519",
  "public_key": "BASE64_PUBLIC_KEY",
  "encrypted_private_key": "BASE64_DPAPI_CIPHERTEXT"
}
```

ความหมายของฟิลด์:

| ฟิลด์ | รายละเอียด |
|---|---|
| `agent_id` | UUID ถาวรของ Agent |
| `algorithm` | Algorithm ที่ใช้สร้าง key pair |
| `public_key` | Public key ที่ encode เป็น Base64 |
| `encrypted_private_key` | Private key ที่เข้ารหัสด้วย DPAPI และ encode เป็น Base64 |

ไฟล์นี้ควรมีสิทธิ์อ่าน/เขียนเฉพาะ account ที่ใช้รัน Agent

## API ตรวจสอบ Agent

```http
GET /api/agents/{agent_id}/exists
```

ไม่ใช้ session หรือ permission

ผลลัพธ์:

| HTTP status | ความหมาย |
|---|---|
| `204 No Content` | พบ Agent แล้ว ข้ามการสมัคร |
| `404 Not Found` | ยังไม่พบ Agent ต้องสมัครครั้งแรก |
| อื่น ๆ | ถือว่าเกิดข้อผิดพลาดและหยุด startup |

## API สมัครครั้งแรก

```http
POST /api/agents/register
Content-Type: application/json
```

ตัวอย่าง payload:

```json
{
  "token": "plaintext-secret",
  "agent_id": "11111111-1111-1111-1111-111111111111",
  "public_key": "BASE64_PUBLIC_KEY",
  "hostname": "workstation-01",
  "mac_address": "AA:BB:CC:DD:EE:FF",
  "os_info": {
    "name": "windows",
    "edition": "amd64",
    "version": "11"
  }
}
```

ฟิลด์ที่ Agent ส่ง:

- `token`
- `agent_id`
- `public_key`
- `hostname`
- `mac_address`
- `os_info`

`room_id` ยังไม่ได้กำหนดจากฝั่ง Agent ใน implementation ปัจจุบัน และเป็นฟิลด์ optional ของ API

หลัง Server ตอบสำเร็จ Agent จะไม่สมัครซ้ำในการรันครั้งถัดไป ตราบใดที่ `agent_config.json` และ `agent_id` ยังอยู่

## Registration Token

ถ้า API ตอบ `404` Agent จะขอ token ผ่าน CLI:

```text
Agent registration token:
```

ข้อควรระวัง:

- token จะไม่ถูกเขียนลง `agent_config.json`
- token จะไม่ถูกเขียนลง Agent log
- ห้ามใส่ token ไว้ใน source code หรือ binary
- หากไม่มี stdin เช่น รันเป็น service ตั้งแต่ครั้งแรก การสมัครจะล้มเหลวและต้องทำ enrollment แบบ interactive ก่อน

## ข้อมูลเครื่อง

ข้อมูลเครื่องถูกใช้เป็น metadata ประกอบ Agent:

- hostname
- MAC address ของ network interface หลัก
- OS name
- CPU architecture
- OS version

MAC address และ IP อาจเปลี่ยนได้ จึงไม่ควรใช้เป็น identity หลักของ Agent

## Log สำหรับ Developer

ค่าเริ่มต้น:

```text
logs/agent.log
```

เปลี่ยนตำแหน่งได้ด้วย:

```env
AGENT_LOG_PATH=./logs/agent.log
```

Log จะบันทึกเหตุการณ์ เช่น:

- Agent เริ่มหรือหยุดทำงาน
- ตรวจพบ Agent เดิม
- สมัคร Agent สำเร็จ
- API หรือ WebSocket ผิดพลาด
- การเชื่อมต่อและ reconnect

Log ไม่ควรมีข้อมูลต่อไปนี้:

- registration token
- private key
- ค่า DPAPI plaintext

## Environment Variables

```env
# WebSocket สำหรับการทำงานปกติ
WS_SERVER_URL=ws://localhost:8081/ws

# HTTP API สำหรับตรวจสอบและสมัคร Agent
AGENT_API_URL=http://localhost:8080

# ไฟล์ log
AGENT_LOG_PATH=./logs/agent.log
```

## จุดเริ่มต้นของโค้ด

| ไฟล์ | หน้าที่ |
|---|---|
| `main.go` | เรียก enrollment ก่อนเริ่ม Agent |
| `service/agent_id.go` | โหลด/สร้าง Agent ID และ key pair |
| `service/registration.go` | ตรวจสอบและสมัครกับ API |
| `service/dpapi_windows.go` | เรียก Windows DPAPI |
| `service/logging.go` | ตั้งค่า log file |
| `service/console_windows.go` | detach console หลัง enrollment |
| `config/config.go` | โหลดค่า API และ runtime config |

## การทดสอบเบื้องต้น

คำสั่งตรวจสอบในเครื่องพัฒนา:

```powershell
go test ./...
go vet ./...
go build
```

การทดสอบกับ Server จริงควรตรวจสอบอย่างน้อย:

1. ลบหรือย้าย `agent_config.json`
2. เปิด Agent
3. ตรวจสอบว่า Agent ขอ token ผ่าน CLI
4. ตรวจสอบ request ที่ `/api/agents/register`
5. ตรวจสอบว่า Server บันทึก `public_key`
6. ปิดและเปิด Agent ใหม่
7. ตรวจสอบว่าเรียก `/exists` แล้วได้ `204`
8. ตรวจสอบว่าไม่มีการถาม token ซ้ำ

## งานที่ควรพัฒนาต่อ

ระบบปัจจุบันครอบคลุมเฉพาะ enrollment ขั้นต้น งานต่อไปที่ควรพิจารณา:

- ให้ Agent เซ็น request ด้วย private key
- ให้ Server ตรวจสอบ signature ด้วย public key
- เพิ่ม timestamp และ nonce เพื่อป้องกัน replay
- เพิ่ม key rotation และ revoke
- รองรับ TPM-backed key
- เพิ่มการตั้ง NTFS ACL อย่างชัดเจน
- เพิ่ม integration test กับ Server จริง
- เพิ่ม retry policy สำหรับ API ที่ชั่วคราวล่ม
