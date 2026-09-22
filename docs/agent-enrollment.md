# Agent Enrollment และ Identity

เอกสารนี้อธิบายการสมัคร Agent ครั้งแรกกับ Server และการเก็บ identity ของ Agent ในฝั่ง Windows

## ภาพรวม

เมื่อ Agent เริ่มทำงาน จะทำตามลำดับนี้:

```text
โหลด identity เดิม (สร้างได้เฉพาะ interactive provisioning/development)
        |
        v
GET /api/agents/{agent_id}/exists
        |
        +-- 204: Agent มีอยู่แล้ว -> ข้ามการสมัคร
        |
        +-- 404: interactive ขอ token; Service หยุดให้ Administrator provision
                         |
                         v
                 POST /api/agents/register
                         |
                         v
                  เริ่ม runtime (Console ยังคงอยู่ใน terminal)
```

การตรวจสอบ Agent ใช้ `agent_id` เป็นหลัก ไม่ใช้ IP หรือ MAC address เป็นตัวตนถาวรของเครื่อง

## การสร้าง Key Pair

Agent ใช้ algorithm:

```text
Ed25519
```

เมื่อไม่มีไฟล์ identity และกำลังทำ interactive provisioning/development ระบบจะสร้าง:

- `public_key` สำหรับส่งให้ Server
- `private_key` สำหรับเก็บไว้ในเครื่อง Agent

Private key จะไม่ถูกส่งไป Server

หากไฟล์ identity มีอยู่แล้วแต่ข้อมูลไม่ครบหรือผิดรูปแบบ ระบบจะหยุดและแจ้งให้ผู้ดูแลตรวจสอบ โดยไม่สร้าง key ทับ ดูขั้นตอนย้าย identity และ Windows Service ที่ [windows-service.md](windows-service.md)

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

Identity เดิมที่ไม่มี `private_key_protection` และ identity ใหม่ใน Console ใช้ user-scope DPAPI
ส่วน identity ใหม่จาก Service provisioning ใช้ machine-scope DPAPI พร้อม metadata
`"private_key_protection": "dpapi-machine-v1"` และ ACL ที่ให้ SYSTEM/Administrators เท่านั้น
การย้าย key เดิมต้องใช้คำสั่ง explicit แยกต่างหาก ดู [private-key-protection.md](private-key-protection.md)
Service startup ยังคงไม่ decrypt หรือ migrate key อัตโนมัติ

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
| `private_key_protection` | `dpapi-machine-v1` สำหรับ Service; ไม่มี field หมายถึง legacy user scope; version อื่นถูกปฏิเสธ |

ไฟล์นี้ควรมีสิทธิ์อ่าน/เขียนเฉพาะ account ที่ใช้รัน Agent

## API ตรวจสอบ Agent

```http
GET /api/agents/{agent_id}/exists
```

ไม่ใช้ session หรือ permission

เส้นนี้ยืนยันเพียงว่ามี Agent record ตาม ID ไม่ได้พิสูจน์ว่าผู้เรียกถือ private key
และไม่เปรียบเทียบ public key ฝั่ง Agent กับฐานข้อมูล ส่วน enrollment marker เป็น
สถานะ provisioning ในเครื่อง ไม่ใช่หลักฐาน authentication แม้ฟิลด์จะชื่อ `verified`
Phase 4 เพิ่มการพิสูจน์ความเป็นเจ้าของ key ผ่าน WebSocket แล้ว ดู [Agent authentication](agent-authentication.md)

ผลลัพธ์:

| HTTP status | ความหมาย |
|---|---|
| `204 No Content` | พบ Agent แล้ว ข้ามการสมัคร |
| `404 Not Found` | ยังไม่พบ Agent ต้องสมัครครั้งแรก |
| 408 / 429 / 5xx / network failure | Service รอ retry โดยยกเลิกได้ผ่าน context; Console แจ้ง error |
| HTTP error อื่น | แจ้ง configuration/provisioning error และหยุด |

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

REST ตรวจ `public_key` ก่อนเริ่ม transaction: รับไม่เกิน 256 bytes ก่อน trim,
ตัด whitespace เฉพาะรอบนอก แล้วรับ standard padded Base64 ความยาว 44 ตัวอักษร
ที่ decode ด้วย strict encoding ได้ public key ขนาด 32 bytes (`ed25519.PublicKeySize`)
ไม่รับ PEM, Base64URL, padding ที่ไม่ถูกต้อง, whitespace ภายใน หรือ Ed25519 private key 64 bytes
จัดเก็บเป็น canonical Base64; รูปแบบผิดตอบ HTTP 400 และไม่ใช้โควตา token
ตัวอย่าง `BASE64_PUBLIC_KEY` เป็น placeholder ต้องแทนด้วย public key จริงที่ Agent สร้าง

Phase 1 คง `DecodeIdentity` เดิม: ตรวจ UUID, algorithm เป็น `Ed25519`,
public key Base64 ขนาด 32 bytes และ encrypted private key Base64 ที่ไม่ว่าง
ไม่ decrypt, re-encrypt, ตรวจคู่ public/private ทางคณิตศาสตร์ หรือเขียนแก้ไฟล์เดิม
หากผิดรูปแบบให้ผู้ดูแลตรวจสอบ ไม่ลบ identity เพื่อให้ระบบสร้างใหม่

Phase 2 เพิ่มการตรวจ protection version ใน structural loader และเพิ่ม
`LoadPrivateKey` แยกสำหรับ private-key use ซึ่ง decrypt และ derive public key จาก seed
แล้วตรวจความสอดคล้องของ key ทั้งคู่ โดยยังไม่เรียกใช้ใน Service startup หรือ WebSocket protocol

หลัง Server ตอบสำเร็จ Agent บันทึก enrollment marker แยกจาก identity โดยผูกกับ ID, public-key fingerprint และ API ที่ตรวจสอบ การเริ่มครั้งถัดไปยังตรวจ `/exists` เสมอ; `204` ข้าม enrollment, `404` ต้อง provisioning ใหม่ และไม่สร้าง identity ใหม่

## Registration Token

Enrollment Token ให้อำนาจสมัครเข้าระบบครั้งแรก ไม่ใช่หลักฐานยืนยันทุก WebSocket connection
Agent ID เป็น identifier ที่ไม่ใช่ secret; Ed25519 ใช้ Sign/Verify ไม่ใช่ Encrypt/Decrypt
private key ต้องอยู่ในเครื่อง Agent เท่านั้น

ถ้า API ตอบ `404` Agent จะขอ token ผ่าน CLI:

```text
Agent registration token:
```

ข้อควรระวัง:

- token จะไม่ถูกเขียนลง `agent_config.json`
- token จะไม่ถูกเขียนลง Agent log
- ห้ามใส่ token ไว้ใน source code หรือ binary
- Service ไม่อ่าน stdin: ต้องทำ administrative provisioning ก่อน หากมี identity แต่ไม่มี marker จะตรวจ API; เมื่อ API ไม่พร้อมจะรอ retry ในสถานะยังไม่ทราบ

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
| `service/console_windows.go` | helper เดิมที่เก็บไว้; startup ปัจจุบันไม่เรียก detach console |
| `config/config.go` | โหลดค่า API และ runtime config |

## การทดสอบเบื้องต้น

คำสั่งตรวจสอบในเครื่องพัฒนา:

```powershell
go test ./...
go vet ./...
go build
```

การทดสอบกับ Server จริงควรตรวจสอบอย่างน้อย:

1. ใช้ไดเรกทอรีทดสอบใหม่ที่ไม่มี identity โดยไม่ลบหรือเปลี่ยนไฟล์ของ installation เดิม
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
- ตรวจยืนยัน NTFS ACL ของ Service ด้วยบัญชี Standard User ตาม checklist
- เพิ่ม integration test กับ Server จริง
- ตรวจยืนยัน Service retry เมื่อ API ล่มระหว่าง boot ตาม checklist
