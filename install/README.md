# ชุดติดตั้งและรัน Agent สำหรับ Windows 64-bit

ถ้าต้องการติดตั้งทุกส่วนในครั้งเดียว ให้เปิด BAT ที่ **root ของโปรเจกต์**:

- `install-new-agent.bat` — เครื่องใหม่: ติดตั้งและเริ่ม Agent Service จากนั้นติดตั้งและเริ่ม Desktop Helper พร้อม task ตอน login
- `install-existing-agent.bat` — เครื่องที่มี machine identity แล้ว: อัปเดต/ติดตั้ง Service ใหม่โดยเก็บ identity และ config เดิม จากนั้นติดตั้งและเริ่ม Desktop Helper

ดับเบิลคลิกตามปกติในบัญชีที่จะใช้จับหน้าจอ ไม่ต้องเปิด Run as administrator เอง
ระบบขอ UAC สำหรับ Service และขอ enrollment token เมื่อระบบลงทะเบียนต้องใช้
หากย้ายไปเครื่องอื่น ให้คัดลอก BAT สองไฟล์นี้พร้อมโฟลเดอร์ `install` วางไว้ข้างกัน
ทั้งสองไฟล์ตรวจไฟล์ของทุกส่วนก่อนเริ่ม และหยุดเมื่อขั้นตอนใดล้มเหลว

คัดลอก **ทั้งโฟลเดอร์ `install`** ไปเครื่องปลายทาง แล้วดับเบิลคลิก BAT ที่ต้องการ
ทุกไฟล์หา executable และ config จากตำแหน่งของตัวเอง ไม่ต้องแก้ path หรือรันจาก working directory เฉพาะ
เครื่องปลายทางไม่ต้องติดตั้ง Go; ใช้ Go เฉพาะตอนบิลด์จาก source

```text
install/
  agent/
    setup/          ติดตั้ง Agent ใหม่ หรือใช้ identity เดิม
    run/            เริ่ม หยุด รีสตาร์ต และดูสถานะ Windows Service
    maintenance/    อัปเดต Agent และ Desktop Helper
    uninstall/      ถอน Service และลบไฟล์โปรแกรม พร้อม identity/config/logs/downloads
  desktop-helper/
    setup/          ติดตั้งและเริ่มตัวจับหน้าจอของผู้ใช้ พร้อม task ตอน login
    run/            เริ่ม/หยุดตัวจับหน้าจอใน session ปัจจุบัน
    uninstall/      ยกเลิก task หยุดตัวจับหน้าจอ และลบไฟล์ Helper ของผู้ใช้
  build/
    build.bat       ทดสอบและบิลด์ใหม่ทั้งสองโปรแกรม
    agent/thesis-agent.exe
    desktop-helper/thesis-agent-desktop.exe
    manifest.json   เวลา build, Go version, ขนาด และ SHA-256
  config/service.env
  scripts/          สคริปต์ภายในที่ BAT เรียกใช้
```

## เริ่มใช้งาน

1. เครื่องใหม่: เปิด `agent/setup/install-new-agent.bat` แล้วอนุญาต UAC ระบบใช้ค่า server จาก `config/service.env` ที่เตรียมไว้ให้ และขอ enrollment token ผ่านขั้นตอนลงทะเบียนเดิม ไม่ต้องแก้ BAT
2. เครื่องที่มี machine identity อยู่แล้ว: ใช้ `agent/setup/install-existing-agent.bat` จะรักษา identity และ config ที่ติดตั้งไว้ หากต้องย้าย identity จาก console ให้ใช้ขั้นตอน `-IdentityPath` ใน [คู่มือ Service](../docs/windows-service.md) อย่างชัดเจน
3. ในบัญชีผู้ใช้ที่ต้องการจับหน้าจอ เปิด `desktop-helper/setup/install-desktop-helper-task.bat` แบบปกติ ตัวช่วยเริ่มทันทีและเริ่มใหม่เมื่อ login

Agent ทำงานเป็น Windows Service และเริ่มเมื่อเปิดเครื่อง ส่วน Desktop Helper ต้องมีผู้ใช้ login อยู่
ตัวช่วยถูกคัดลอกไป `%LOCALAPPDATA%\ThesisAgentDesktop` จึงไม่ผูก task กับตำแหน่งชุดติดตั้ง
งาน Service ขอสิทธิ์ Administrator อัตโนมัติ; งาน Desktop Helper ต้องเปิดในบัญชีที่ต้องการจับหน้าจอ
หากนโยบายเครื่องไม่อนุญาตการสร้าง Scheduled Task จะรายงานข้อผิดพลาด

## ใช้งานประจำ

| งาน | ไฟล์ BAT |
| --- | --- |
| เริ่ม Agent Service | `agent/run/start-agent.bat` |
| ดูสถานะ Service | `agent/run/status-agent.bat` |
| หยุด Service | `agent/run/stop-agent.bat` |
| รีสตาร์ต Service | `agent/run/restart-agent.bat` |
| อัปเดต executable ทั้งสอง โดยรักษา identity/config | `agent/maintenance/update-agent.bat` |
| เริ่มตัวจับหน้าจอ | `desktop-helper/run/run-desktop-helper.bat` |
| หยุดตัวจับหน้าจอ | `desktop-helper/run/stop-desktop-helper.bat` |
| ยกเลิกตัวจับหน้าจออัตโนมัติ | `desktop-helper/uninstall/uninstall-desktop-helper-task.bat` |
| ถอนทะเบียน Agent Service | `agent/uninstall/uninstall-agent.bat` |
| ทดสอบและบิลด์ใหม่ | `build/build.bat` |

ใช้ `uninstall-agent.bat` ที่ root ของโปรเจกต์เพื่อถอน Agent และ Desktop Helper ในครั้งเดียว เปิดตามปกติในบัญชีที่ติดตั้ง Helper; ขั้นตอน Service จะขอ UAC เอง
ตัวถอน Agent ลบ Service, Event Log source, `%ProgramFiles%\ThesisAgentDev` และ `%ProgramData%\ThesisAgentDev` ทั้งหมด รวม `.env`, identity, enrollment state, logs และ downloads การติดตั้งครั้งถัดไปต้องลงทะเบียน identity ใหม่
ตัวถอน Helper ลบ task และ `%LOCALAPPDATA%\ThesisAgentDesktop` ของผู้ใช้ที่เปิด BAT หากใช้ BAT ในโฟลเดอร์ `install` จะถอนเฉพาะส่วนนั้น
เก็บชุดติดตั้งและไฟล์ใน source repository ไว้ รวมถึง `install/config/service.env` และ `.env` ของ Console ข้อมูลบน server และ Helper ของผู้ใช้บัญชีอื่นไม่ถูกลบ
การถอนตรวจขอบเขตโฟลเดอร์และปฏิเสธ symlink/junction หากลบไม่สำเร็จจะแสดงข้อผิดพลาดและคืน exit code ที่ไม่ใช่ศูนย์ สามารถรันซ้ำเพื่อลบส่วนที่เหลือได้
การอัปเดต helper มีผลกับบัญชีผู้ใช้ที่เปิด BAT; เครื่องที่มีหลายบัญชีต้องติดตั้ง helper ในแต่ละบัญชีตามนโยบายเครื่อง

## บิลด์และตรวจสอบ

เปิด `build/build.bat` จาก repository ที่มี Go ตาม `go.mod` สคริปต์จะรัน `go test ./...`
และบิลด์ Windows amd64 ทั้ง Agent และ Desktop Helper ก่อนแทนที่ executable ใน `build`
ถ้าทดสอบหรือ compile ไม่ผ่านจะเก็บ executable ชุดเดิม ไม่แก้ config และไม่แตะ Service ที่ติดตั้งอยู่

ทุก BAT รองรับ `--check` สำหรับตรวจไฟล์และ path โดยไม่ติดตั้ง หยุด Service หรือเริ่มโปรแกรม เช่น:

```powershell
.\install\agent\setup\install-new-agent.bat --check
.\install\build\build.bat --check
```

`config/service.env` เป็น config ของชุดติดตั้งนี้และไม่ถูก commit เช่นเดียวกับ executable
การติดตั้งใหม่ใช้ config นี้ ส่วน update ใช้ config ของเครื่องเดิม
การกรอก token, อนุญาต UAC และการเชื่อมต่อ server ยังจำเป็นตามระบบเดิม
อย่าแนบไฟล์ identity, private key หรือ enrollment state ของเครื่องต้นทางไปกับชุดติดตั้ง
