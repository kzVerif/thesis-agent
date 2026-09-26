# Private-key protection สำหรับ Windows Service

**Phase 5B.1:** Service ตรวจ runtime ACL ก่อนเปิด `.runtime.lock`/`.env`/identity
และถือ lock ก่อนซ่อมเฉพาะ trusted drift; `-Action Repair` ใช้เกณฑ์เดียวกัน
unsafe owner/ACE/path ต้องปฏิเสธและให้ผู้ดูแล review/recovery ไม่มี force override
การซ่อมไม่อ่านหรือเขียน identity/key/enrollment contents ไม่ decrypt/re-encrypt
และไม่เปลี่ยน `dpapi-machine-v1` ดู [runtime ACL security](runtime-acl-security.md)
สำหรับ diagnostics, Event Log fallback, TOCTOU และสถานะ NOT VERIFIED ของ Lab
Startup ตรวจเฉพาะ root, critical state, known identity backup และ container directories
ไม่ scan historical downloads/logs แบบ recursive; dynamic objects ตรวจเมื่อใช้งาน
ผ่าน Service-only Boundary และไม่ซ่อม ACL ของ historical file โดยอัตโนมัติ

**Phase 4:** ตอนนี้ WebSocket ต้องใช้ Ed25519 signing แล้ว แม้ legacy identity
ยังเริ่ม Service ได้ แต่จะ authenticate ไม่ผ่านจนกว่าจะ migrate อย่างชัดเจน
ข้อจำกัดของ loader ใช้กับ Console ด้วย ดู [Agent authentication](agent-authentication.md).
ข้อความด้านล่างอธิบายขอบเขตการเปลี่ยนแปลงของ Phase 2.

Phase 2 เปลี่ยนเฉพาะการเก็บ/โหลด Ed25519 private key ไม่เพิ่ม WebSocket authentication
และไม่เปลี่ยน REST, enrollment, public key ที่ Server เก็บ หรือ Service lifecycle

## รูปแบบและขอบเขต

- ไม่มี `private_key_protection`: legacy user-scope DPAPI; ไม่ตีความว่าเป็น machine scope
- `"private_key_protection": "dpapi-machine-v1"`: machine-scope DPAPI
- Version ที่ไม่รู้จัก: fail closed โดยไม่สร้าง identity ทดแทน
- Service startup โหลดโครงสร้างเท่านั้น; legacy ยังใช้ features เดิมได้พร้อม log แจ้ง migration
- `service.LoadPrivateKey(path)` เป็น authoritative loader สำหรับ Service private-key use
  ต้องใช้ machine metadata, protected ACL, decrypt สำเร็จ, private key ยาว 64 bytes
  และ key ที่ derive จาก seed ต้องตรงทั้ง 64 bytes รวมถึง stored public key
- Caller ต้องถือ runtime lock และ `clear(key)` เมื่อใช้เสร็จ ห้าม log key
- Console สร้าง identity ใหม่แบบ user scope ตามเดิม เพราะ directory ของ Console
  อาจไม่ได้มี ACL สำหรับ machine-scope secret
- Provisioning identity ใหม่ของ Service ใช้ machine scope ตั้งแต่แรก ตรวจ ACL ก่อนสร้าง
  Existing identity ไม่ถูกเปลี่ยนจากการ start/provision ซ้ำ

Machine scope เลือกที่ `CryptProtectData` ด้วย LOCAL_MACHINE + UI_FORBIDDEN;
`CryptUnprotectData` ใช้ UI_FORBIDDEN โดยไม่ใส่ LOCAL_MACHINE
ผู้ใช้ในเครื่องที่อ่าน machine ciphertext ได้อาจ decrypt ได้ จึงต้องรักษา ACL:
owner SYSTEM/Administrators, อนุญาตเฉพาะสองกลุ่มนี้และสืบทอดสิทธิ์ให้ child files
ตัวตรวจ ACL ปฏิเสธรูปแบบที่ไม่ตรงกับ profile ที่ provisioning script ใช้
รวมถึง reparse points ตัว ValidateFile/ValidateDirectory และ private-key loader
ยังตรวจอย่างเดียว ไม่เปลี่ยน ACL; startup repair เป็นขั้นตอนแยกที่ชัดเจน
ดู [Microsoft CryptProtectData](https://learn.microsoft.com/en-us/windows/win32/api/dpapi/nf-dpapi-cryptprotectdata)
และ [CryptUnprotectData](https://learn.microsoft.com/en-us/windows/win32/api/dpapi/nf-dpapi-cryptunprotectdata)

## คำสั่ง maintenance

```powershell
& $phase2Executable --migrate-private-key-protection
& $phase2Executable --verify-private-key
```

เลือกทีละคำสั่ง ห้ามรวมกับ `--service`, `--provision`, `--migrate-identity`,
`--configure-service` หรือ `--service-info`
ใช้ binary ที่ build จาก Phase 2 และตั้งตัวแปร `$phase2Executable` เป็น absolute path

ทั้งสองคำสั่งต้องใช้ elevated Administrator terminal, ตรวจว่า SCM Service หยุดแล้ว
และถือ `.runtime.lock` คำสั่งไม่หยุด/kill Service ให้ ไม่เชื่อมต่อ Server ไม่ใช้ token
และไม่แก้ enrollment state

`--migrate-identity PATH` เดิมยังเป็นการ copy identity ทุก byte ไป Service runtime
ไม่มีการ decrypt/re-encrypt หาก import legacy identity ให้ทำ crypto migration แยกภายหลัง
ห้าม import legacy source ซ้ำทับ identity ที่ migrate แล้ว; ใช้ runtime copy เป็นตัวจริง

Migration อ่านเฉพาะ identity ใน runtime ที่ resolve จาก Windows Known Folders:
`%ProgramData%\ThesisAgentDev\agent_config.json`
ไม่รับ target path ที่ Desktop, Temp หรือ Downloads

## Migration และ backup

1. ตรวจ ACL/runtime lock, อ่านและ validate legacy identity
2. Decrypt ใน context ที่สร้าง blob เดิม แล้วตรวจ key pair จาก seed
3. ตรวจ enrollment marker ถ้ามี: Agent ID และ SHA-256 ของ public key ต้องตรง
4. สร้าง backup `agent_config.json.dpapi-user-v1.bak` ใน protected runtime เดียวกัน
   ด้วย exclusive publication; backup เป็น ciphertext/JSON เดิมทุก byte
5. ถ้ามี backup อยู่แล้ว ใช้ซ้ำได้เฉพาะเมื่อ bytes ตรงกับ identity ก่อน migration ทุก byte
   ถ้าต่างหรืออ่านไม่ได้ให้หยุด ไม่ overwrite backup
6. Protect private key เดิมด้วย machine scope แล้ว decrypt/validate candidate ใน memory
7. ตรวจ identity/enrollment อีกครั้งก่อน atomic replacement
8. ใช้ statefile เดิม: encrypted candidate อยู่ใน `.state-*.tmp` ใต้ protected directory,
   flush แล้ว atomic replace; ไม่มี plaintext private key ลงไฟล์
9. อ่านกลับ/decrypt/ตรวจ key pair และความคงเดิมของ ID, algorithm, public key และ private key

Unknown contributor JSON fields ยังคงค่าเดิม แต่ formatting/order ของ JSON อาจเปลี่ยน
Backup เก็บต้นฉบับทุก byte; enrollment marker ไม่ถูกเขียนใหม่
Identity ที่ migrate แล้วจะตรวจสอบและจบโดยไม่ rewrite หรือสร้าง backup เพิ่ม

## Failure และ recovery

- Failure ก่อน replacement: original identity ไม่เปลี่ยน; backup ที่สร้างแล้วเก็บไว้
- Failure หลัง replacement: พยายาม atomic rollback แล้วอ่านเทียบ original ทุก byte
- ข้อความ `original identity restored and verified` หมายถึงตรวจ bytes แล้วจริง
- ถ้า restore/readback ไม่สำเร็จ แจ้ง `rollback could not be verified` และตำแหน่ง backup
  ห้ามลบ backup หรือให้ระบบสร้าง identity ใหม่
- การถูกหยุดก่อน/หลัง atomic publication อาจเหลือ original หรือ candidate ที่ครบชุด
  โดยมี encrypted original backup อยู่ใน protected directory
- การทดสอบ interruption เป็น fault injection ไม่ใช่หลักฐานว่าทดสอบไฟดับ/ดิสก์เสียจริง
  การรับประกันนี้ไม่ครอบคลุม hardware/storage corruption ที่ทำลายทั้งสองสำเนา

หากต้อง manual recovery ให้ Administrator หยุด Service และ runtimes ทั้งหมด
ตรวจ backup กับ hash ก่อน migration และใช้เครื่องมือกู้คืนที่ถือ `.runtime.lock`
เพื่อเขียนสำเนา backup ลง staging file ใน protected directory, flush, atomic replace,
แล้วตรวจ identity hash ว่าตรง backup ห้าม restore ด้วยการเขียนทับไฟล์ทีละส่วน
Backup ยังเป็น user-scope เดิม ต้องเก็บ account/profile เดิมไว้เพื่อ decrypt
หากไม่ได้เตรียมเครื่องมือ recovery ที่ถือ lock ให้หยุดไว้และให้ผู้ดูแลตรวจสอบก่อน

## Manual Windows verification

ทำกับเครื่องทดสอบก่อน โดยใช้ elevated terminal ของ account ที่ decrypt legacy blob ได้
Administrator อีกบัญชีไม่ได้ทำให้ decrypt user-scope blob เดิมได้โดยอัตโนมัติ
หาก context เดิมใช้ไม่ได้ ให้หยุดตรวจสอบ ไม่ regenerate/re-enroll หรือผ่อน ACL

1. Build binary Phase 2 แยกจาก installed executable; อย่าทับ Service ที่กำลังรัน
2. อ่านชื่อ/path จาก `--service-info`, หยุด Service แล้วรอ Stopped
3. บันทึก Agent ID, algorithm, public-key SHA-256, identity-file SHA-256
   และ enrollment-file SHA-256 หากมี โดยไม่พิมพ์ JSON/ciphertext/private key
4. รัน `--migrate-private-key-protection` แล้วตรวจ exit code
5. ตรวจ ID, algorithm และ public key ตรงเดิม; protection เป็น dpapi-machine-v1
   ciphertext เปลี่ยน; backup hash ตรง identity hash ก่อน migration
   และ enrollment-file hash ไม่เปลี่ยน
6. รัน `--verify-private-key` ใน elevated original-account context
7. **ตรวจ LocalSystem แยกต่างหาก**: ขณะ Service หยุด ให้ผู้ดูแลรันคำสั่ง verify
   จาก one-shot task ที่ตั้งเป็น SYSTEM และใช้ binary/path ที่ตรวจสอบแล้ว
   ตรวจ exit code และข้อความสำเร็จ โดยไม่ส่ง key ผ่าน arguments/output
   การ verify ในบัญชี Administrator เพียงอย่างเดียวไม่พิสูจน์ LocalSystem compatibility
8. รัน migration ซ้ำ: ต้องแจ้ง already migrated และ identity/backup hashes ไม่เปลี่ยน
9. Deploy binary ตาม workflow เดิม แล้ว Start-Service ตรวจ feature เดิม/reconnect/Stop-Service
10. Reboot ตรวจ Auto Start และ identity/backup/enrollment hashes คงเดิม
11. ตรวจ Standard User ยังอ่าน identity/backup หรือแก้ runtime directory ไม่ได้

Service startup สำเร็จไม่ใช่หลักฐานว่า decrypt ได้ เพราะ Phase 2 จงใจไม่เพิ่ม
private-key use เป็น startup requirement และยังไม่มี Phase 4 challenge/signing

## Automated checks

```powershell
go test ./...
go vet ./...
go build ./...
git diff --check
git diff --cached --check
```

Windows tests ใช้ DPAPI จริงกับ key ที่สร้างสำหรับ tests ใน memory
Migration fault-injection ใช้ filesystem จำลองใน memory จึงไม่สร้าง backup ของ key ใน Temp
Statefile tests ใช้ marker data ที่ไม่ใช่ key เพื่อทดสอบ atomic writes/sharing violations
SCM deployment, protected production paths, LocalSystem context และ reboot ต้องทดสอบแยก
