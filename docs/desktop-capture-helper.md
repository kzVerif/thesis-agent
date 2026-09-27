# Desktop Capture Helper

Windows Service ของ Agent ทำงานด้วยบัญชี LocalSystem ใน Session 0 จึงไม่สามารถ
เข้าถึงหน้าจอของผู้ใช้ที่ล็อกอินอยู่ได้ โปรเจกต์นี้จึงแยกโปรแกรมจับภาพหน้าจอเป็น
Desktop Helper ซึ่งทำงานใน user session ของผู้ใช้

โครงสร้างการทำงาน:

```text
ThesisAgentDev Service
(LocalSystem, เชื่อมต่อ WebSocket)
        │
        │ Named Pipe: \\.\pipe\ThesisAgentDesktop
        ▼
Desktop Capture Helper
(user session, จับภาพหน้าจอ)
```

Service ยังคงเป็นโปรแกรมเดียวที่เชื่อมต่อ Agent WebSocket และถือ machine
identity/private key ส่วน Desktop Helper ไม่มี credential ของ server และมีหน้าที่
รับคำขอจับภาพผ่าน Named Pipe แล้วส่ง JPEG กลับมาเท่านั้น

## การอัปเดต executable

บิลด์ด้วย `install/build/build.bat` หรือใช้ executable ที่เตรียมไว้ในชุดติดตั้ง:

```text
install/build/agent/thesis-agent.exe
install/build/desktop-helper/thesis-agent-desktop.exe
```

จากนั้นใช้ไฟล์สำหรับอัปเดต:

```text
install/agent/maintenance/update-agent.bat
```

เปิดไฟล์นี้จากบัญชีผู้ใช้ที่ต้องการจับหน้าจอ ระบบขอ UAC เฉพาะงาน Service
และจะทำสิ่งต่อไปนี้ โดยไม่เรียกคำสั่ง build หรือ compile:

1. ตรวจสอบ executable ใหม่
2. หยุด Windows Service
3. ติดตั้ง Service executable ใหม่ โดยใช้ identity และ configuration เดิม
4. หยุด Desktop Helper ใน session ของผู้ใช้ปัจจุบัน
5. คัดลอก helper ไป `%LOCALAPPDATA%\ThesisAgentDesktop`
6. ลงทะเบียนและเริ่ม Desktop Helper task กลับมา

## การทดสอบแบบเปิดเอง

หลังวาง executable แล้ว ให้เปิด Desktop Helper ใน user session ที่ต้องการจับภาพ
แบบ background:

```powershell
.\install\desktop-helper\run\run-desktop-helper.bat
```

คำสั่งนี้จะเรียกผ่าน `wscript.exe` แบบซ่อนหน้าต่างของ helper หากยังไม่เคยติดตั้งจะติดตั้งให้ก่อน หาก
ปิด Desktop Helper แล้ว Service จะยังทำงานต่อ แต่การจับภาพจะไม่พร้อมใช้งาน

## เริ่มอัตโนมัติตอน login

ลงทะเบียน Task Scheduler สำหรับ user ปัจจุบัน:

```powershell
.\install\desktop-helper\setup\install-desktop-helper-task.bat
```

Task จะตั้งเป็น `At log on` และ `Run only when user is logged on` เพื่อให้ Helper
ทำงานใน desktop session ที่ถูกต้อง ไม่ใช่ Session 0

การเริ่มผ่าน Task Scheduler ใช้ `wscript.exe` เป็นตัว launcher แบบซ่อนหน้าต่าง
ดังนั้นจะไม่มีหน้าต่าง Command Prompt แสดงขึ้นมา และ Desktop Helper จะทำงานอยู่
เบื้องหลัง

ยกเลิก task:

```powershell
.\install\desktop-helper\uninstall\uninstall-desktop-helper-task.bat
```

## การส่งภาพ

Service ใช้คำสั่ง screen stream เดิมจาก server แล้วทำงานตามลำดับนี้:

```text
รับคำสั่ง start_screen จาก WebSocket
→ ขอภาพจาก Desktop Helper ผ่าน Named Pipe
→ รับ JPEG ใน memory
→ ส่ง JPEG เป็น WebSocket binary frame
```

การจับภาพยังใช้ช่วงเวลาเดิมประมาณ 200 มิลลิวินาที หรือประมาณ 5 FPS ไม่มีการสร้าง
ไฟล์ภาพค้างบน disk และมีคำขอจับภาพครั้งละหนึ่งรายการ

ถ้า Desktop Helper ไม่ทำงาน Service จะรายงานว่าไม่สามารถรับ frame ได้ชั่วคราว แต่
Service และ WebSocket จะยังทำงานต่อ เมื่อ Helper กลับมา การจับภาพจะเริ่มได้อีกครั้ง

## ข้อจำกัดปัจจุบัน

- รองรับ primary display ของ user session ที่กำลังใช้งาน
- ถ้าไม่มี user login จะไม่สามารถจับภาพหน้าจอได้
- ยังไม่ได้ออกแบบการเลือก session เมื่อมีหลาย RDP session
- Desktop Helper ไม่ได้ทำ authentication กับ server
- การยืนยันตัวตนและการส่งข้อมูลไป server ยังคงเป็นหน้าที่ของ Windows Service

โครงสร้างนี้เหมาะกับการใช้งานแบบมีผู้ใช้ interactive หนึ่งคนต่อเครื่อง หากต้องการ
รองรับหลาย session หรือเลือกจอของผู้ใช้แต่ละคน ต้องเพิ่มการออกแบบ session routing
ในภายหลัง
