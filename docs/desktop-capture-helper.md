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
ปิด Desktop Helper แล้ว Service จะยังทำงานต่อ เมื่อมีคำขอภาพ Service จะเรียก task ที่ติดตั้งไว้เพื่อเปิด Helper กลับมา

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

ถ้าไม่พบ Named Pipe ของ Desktop Helper ระหว่างมีคำขอภาพ Service จะเรียก
`schtasks.exe /Run /TN \ThesisAgentDesktop` จาก Windows system directory โดยไม่แสดงหน้าต่าง
Task จะเปิด Helper ด้วยบัญชีผู้ใช้ที่ลงทะเบียนไว้และ interactive session ตามการตั้งค่าของ task
Service จะลองเชื่อมต่อ pipe ในรอบจับภาพถัดไป เมื่อ Helper พร้อมจะส่งภาพต่อโดยไม่ต้องสั่ง start_screen ใหม่

การเรียก task จำกัดไม่เกินหนึ่งครั้งต่อ 15 วินาที รวมกรณีเรียกไม่สำเร็จ และมี timeout 3 วินาทีต่อคำสั่ง
กรณี pipe กำลังใช้งานหรือถูกปฏิเสธสิทธิ์ จะไม่เรียก task ซ้ำ หากไม่มีคำขอภาพก็ไม่เปิด Helper
ต้องติดตั้งและเปิดใช้งาน task และผู้ใช้เจ้าของ task ต้องล็อกอินอยู่ ระบบไม่สร้าง task ใหม่หรือเปลี่ยนบัญชีผู้ใช้ให้อัตโนมัติ
หากเรียกไม่สำเร็จจะบันทึกข้อผิดพลาด โดย Service และ WebSocket ยังคงทำงานต่อ

ตรวจสอบบนเครื่องที่ติดตั้ง: อัปเดต Agent ด้วย `install/agent/maintenance/update-agent.bat`,
ปิด process `thesis-agent-desktop.exe` ของผู้ใช้ แล้วสั่ง start_screen จาก server
Helper ควรกลับมาและส่งภาพต่อ ทดสอบอีกครั้งขณะกำลังสตรีมเพื่อยืนยันการเชื่อมต่อใหม่หลัง process ถูกปิด

## ข้อจำกัดปัจจุบัน

- รองรับ primary display ของ user session ที่กำลังใช้งาน
- ถ้าไม่มี user login จะไม่สามารถจับภาพหน้าจอได้
- ยังไม่ได้ออกแบบการเลือก session เมื่อมีหลาย RDP session
- Desktop Helper ไม่ได้ทำ authentication กับ server
- การยืนยันตัวตนและการส่งข้อมูลไป server ยังคงเป็นหน้าที่ของ Windows Service

โครงสร้างนี้เหมาะกับการใช้งานแบบมีผู้ใช้ interactive หนึ่งคนต่อเครื่อง หากต้องการ
รองรับหลาย session หรือเลือกจอของผู้ใช้แต่ละคน ต้องเพิ่มการออกแบบ session routing
ในภายหลัง
