# คู่มือทดสอบ Windows Service บน Windows อีกเครื่อง

เอกสารนี้ใช้ทดสอบ implementation ปัจจุบันใน working tree ของ thesis-agent สำหรับโครงการบริหารห้องปฏิบัติการคอมพิวเตอร์มหาวิทยาลัยที่ได้รับอนุญาต ใช้เฉพาะ **development PC หรือ VM ที่ผู้ทดสอบเป็นเจ้าของหรือได้รับอนุญาต** และทดสอบผ่าน Windows Service Control Manager (SCM) กับ Windows ACL ตามปกติ

Administrator ต้องยัง Start / Stop / Restart / Remove ได้อย่างถูกต้อง ไม่ใช้การซ่อน process/Service, privilege escalation, Defender/EDR bypass, injection, kernel driver หรือ persistence ที่ต่อต้าน Administrator

**เอกสารนี้เป็นขั้นตอนทดสอบ ไม่ใช่รายงานว่าทุกข้อผ่านแล้ว** ใช้สถานะเพียงสามแบบ:

| สถานะ | ความหมาย |
| --- | --- |
| PASS | รันจริงในบัญชี/เครื่องที่กำหนด และมีหลักฐานครบตามเกณฑ์ |
| FAIL | รันภายใต้เงื่อนไขที่ถูกต้องแล้วพฤติกรรมผิดจากที่คาด |
| NOT VERIFIED | ยังไม่ได้ทำ ขาดสิทธิ์/เครื่องมือ หรือหลักฐานยังสรุปไม่ได้ |

งานนี้ไม่ใช่คู่มือติดตั้ง RAT-System ตั้งแต่ต้น และไม่ครอบคลุม Screen/Session Worker, protocol/security redesign, final installer หรือ classroom-scale deployment

## 1. วัตถุประสงค์และความหมายของสถานะ

ทดสอบ Windows Service, Auto Start, SCM lifecycle, สิทธิ์ Administrator/Standard User, persistent identity/config, REST outage, WebSocket reconnect, crash recovery และ graceful Stop

แยกสองสถานะนี้ออกจากกันเสมอ:

| สถานะ | สิ่งที่ยืนยันได้ |
| --- | --- |
| Service Running | runtime ฝั่งเครื่องเริ่มต้นทรัพยากรภายในแล้ว และพร้อมทำงาน/retry |
| Agent Online | มีการเชื่อมต่อกับ backend จริง และฝั่ง server รับรู้ Agent นี้ในเวลาที่ตรวจ |

Service สามารถ Running ระหว่าง REST หรือ WebSocket ใช้งานไม่ได้ เป็นพฤติกรรมที่ตั้งใจไว้ ห้ามสรุป Online จาก Get-Service เพียงอย่างเดียว และ log websocket connected เพียงบรรทัดเดียวก็ยังไม่ยืนยันว่าฝั่ง server ยอมรับ registration แล้ว

ค่าใน source ปัจจุบัน:

| รายการ | ค่าบน Windows ที่ใช้ root มาตรฐาน C: |
| --- | --- |
| Service name | ThesisAgentDev |
| Display name | Thesis Agent (Development) |
| Account / startup | LocalSystem / Automatic |
| Executable | C:\Program Files\ThesisAgentDev\thesis-agent.exe |
| Runtime root | C:\ProgramData\ThesisAgentDev |
| Config | C:\ProgramData\ThesisAgentDev\.env |
| Identity | C:\ProgramData\ThesisAgentDev\agent_config.json |
| Enrollment state | C:\ProgramData\ThesisAgentDev\enrollment_state.json |
| Log | C:\ProgramData\ThesisAgentDev\logs\agent.log |
| Downloads | C:\ProgramData\ThesisAgentDev\data\downloads |
| Management script | scripts/dev-service.ps1 |

Paths จริงอ่านจาก Windows Known Folder APIs ไม่ได้บังคับ drive C: ให้ยึดผล --service-info ของเครื่องที่ทดสอบ

## 2. เครื่องที่ต้องใช้

### 2.1 แบบขั้นต่ำ: Windows test PC/VM เครื่องเดียว

เครื่องเดียวใช้ทดสอบ installation, Start/Stop, permissions, reboot, identity stability และ crash recovery ได้ โดยต้องมี Administrator และ Standard User แยกกัน

**เงื่อนไขสำคัญ:** installation ครั้งแรกต้องเข้าถึง REST และทำ administrative provisioning ให้สำเร็จก่อน สคริปต์จะไม่ติดตั้งจนสำเร็จหาก provisioning ล้มเหลว จึงไม่ใช่การติดตั้งครั้งแรกแบบ offline ทั้งหมด Backend อาจอยู่บนเครื่องเดียวกันหรือเป็นระบบทดสอบที่เข้าถึงได้ชั่วคราว หลัง provisioning จึงทดสอบสถานะ offline/retry ได้

### 2.2 แบบครบ: แยก Agent กับ backend

| เครื่อง | หน้าที่ |
| --- | --- |
| Machine A | Windows Agent ที่ทดสอบ Service |
| Machine B | thesis-rat-server + thesis-web-socket + thesis-rat-dashboard และฐานข้อมูลที่ระบบเหล่านี้ใช้อยู่ |

REST, WebSocket และ Dashboard อยู่รวม Machine B ได้ ไม่ต้องใช้สามเครื่องแยกกัน ให้ backend พร้อมใช้งานตาม runbook เดิมก่อนเริ่มคู่มือนี้ รวม migrations ที่ enrollment ปัจจุบันต้องใช้ และให้ REST/WS ชี้ Agent database ชุดเดียวกัน

Machine B ต้องเข้าถึงได้จาก A ตาม endpoints ที่กำหนด ตัวอย่างใช้ 8080 สำหรับ REST และ 8081 สำหรับ WS หากระบบจริงใช้ ports อื่น ให้ใช้ค่าจริงอย่างสอดคล้องกัน ไม่เปลี่ยน listener หรือ protocol เพื่อทำตามตัวอย่าง

Machine B มีประโยชน์ในการสังเกต Online ขณะที่ A ยังค้างอยู่หน้า Windows login และควบคุม outage ของ REST/WS โดยไม่หยุด Agent

## 3. บัญชีที่ต้องใช้

| บัญชี | งาน |
| --- | --- |
| Administrator ของ A | ตรวจ/build source, เตรียม backup/config, provisioning/install, Start/Stop/Restart/Remove, อ่าน protected files, crash test |
| **Actual Standard User** ของ A | ทดสอบปฏิเสธ stop/config/delete Service, terminate process และ file write/delete access |
| ผู้ดูแล backend ของ B | หยุด/เปิดเฉพาะ REST หรือ WS ทดสอบ และสังเกต Online/log |

Build ไม่จำเป็นต้อง elevation แต่ provisioning และ management script ต้องใช้ **Windows PowerShell 5.1 — Run as administrator** ไม่ใช่ PowerShell 7

> Unelevated Administrator ไม่เท่ากับ Actual Standard User แม้จะเห็น Access denied เหมือนกันก็ตาม ต้องทดสอบด้วยบัญชีที่ไม่ได้เป็นสมาชิก Administrators จริง ไม่ยอมรับ UAC elevation ระหว่าง Standard User tests

## 4. เตรียมเครื่องและ source ที่จะทดสอบ

**บัญชี:** Administrator ของ A; ตรวจบัญชี Standard User ในหน้าต่างของบัญชีนั้นแยกต่างหาก

1. เตรียม disposable VM snapshot ก่อน ACL/crash tests เพราะถ้า ACL ผิด การลอง stop/delete/kill อาจสำเร็จจริง
2. ใช้ Windows ที่รองรับ Windows PowerShell 5.1, NTFS สำหรับ persistent state และ Go ตาม go.mod ปัจจุบัน (go 1.26.1)
3. นำ **reviewed source snapshot ปัจจุบัน** มาที่ A รวมไฟล์ใหม่ที่ยัง untracked เช่น internal/, scripts/ และ tests การ clone เฉพาะ commit เดิมจะไม่รวม implementation ที่ยังไม่ commit
4. อย่านำ .env, agent_config.json, logs, downloads หรือ enrollment_state.json จากเครื่องพัฒนามาใช้เป็น state ของเครื่องใหม่ การย้าย identity ทำแยกเฉพาะกรณีในข้อ 8
5. ตัวอย่างต่อไปใช้ C:\Lab\thesis-agent เป็นที่วาง source ให้แก้เป็น absolute path จริงของเครื่องทดสอบก่อนรัน

~~~powershell
Set-Location -LiteralPath 'C:\Lab\thesis-agent'
whoami
whoami /groups
$PSVersionTable
go version
git status --short
git rev-parse HEAD
Get-ExecutionPolicy -List
Get-CimInstance Win32_OperatingSystem | Select-Object Caption, Version, OSArchitecture
Get-Volume | Select-Object DriveLetter, FileSystem
Test-Path -LiteralPath '.\internal\agent\runtime.go'
Test-Path -LiteralPath '.\internal\servicehost\configure_windows.go'
Test-Path -LiteralPath '.\scripts\dev-service.ps1'
~~~

ผล Test-Path ทั้งสามต้อง True และ PSEdition ต้อง Desktop สำหรับ management script ถ้ารับ source archive ที่ไม่มี .git ให้บันทึก commit + snapshot/version ที่มากับชุด source แทน git status/rev-parse ไม่ต้อง git init เพื่อทำให้คำสั่งผ่าน

บันทึก working tree ที่ยังมี changes ตามจริง ไม่ reset/clean และไม่ใช้ commit hash เพียงอย่างเดียวระบุ source ที่ยัง uncommitted ควรแนบ source snapshot identifier และ binary SHA-256 ด้วย

หาก execution policy บล็อก .ps1 ให้ใช้วิธี signing/policy ที่องค์กรอนุมัติ หรือย้ายไปเครื่องพัฒนาที่ได้รับอนุญาต **ไม่ปิด script-security policy ทั้งระบบ** ถ้าไม่พร้อม ให้บันทึกขั้นตอนที่ติดเป็น NOT VERIFIED

ใช้ sc.exe แบบเต็มเสมอ เพราะ sc อาจเป็น PowerShell alias

## 5. Build development Agent

**บัญชี:** Administrator ของ A; build ใน source directory ไม่จำเป็นต้อง Run as administrator

~~~powershell
Set-Location -LiteralPath 'C:\Lab\thesis-agent'

go test ./...
if ($LASTEXITCODE -ne 0) { throw 'FAIL: go test' }

go vet ./...
if ($LASTEXITCODE -ne 0) { throw 'FAIL: go vet' }

go build -o build\thesis-agent-dev.exe .
if ($LASTEXITCODE -ne 0) { throw 'FAIL: Windows build' }

Get-FileHash -LiteralPath '.\build\thesis-agent-dev.exe' -Algorithm SHA256
~~~

สำเร็จ: tests แสดง ok หรือ cached ตาม package, vet/build จบ exit code 0 และได้ binary ใหม่ หยุดหากมี error อย่าใช้ executable เก่าปะปนกับผล build ที่ล้มเหลว

**Race test — ทำเมื่อมี Windows C toolchain ที่ใช้กับ Go ได้จริงเท่านั้น:**

~~~powershell
go env GOOS GOARCH CGO_ENABLED CC
Get-Command gcc, clang -ErrorAction SilentlyContinue
~~~

ถ้าไม่มี compiler ที่เข้ากันได้ ให้บันทึก **NOT VERIFIED — required Windows C compiler unavailable** ไม่ถือเป็น FAIL ของ Service และไม่ต้องติดตั้ง toolchain เพิ่มเพียงเพื่อผ่านคู่มือนี้ ถ้ามี toolchain พร้อมแล้ว:

~~~powershell
$previousCGO = $env:CGO_ENABLED
try {
    $env:CGO_ENABLED = '1'
    go test -race ./...
    if ($LASTEXITCODE -ne 0) { throw 'Race check failed; classify toolchain error or reported race from output' }
} finally {
    if ($null -eq $previousCGO) {
        Remove-Item Env:CGO_ENABLED -ErrorAction SilentlyContinue
    } else {
        $env:CGO_ENABLED = $previousCGO
    }
}
~~~

บันทึก exit code/output จริง หาก detector พบ race เป็น FAIL; หาก compiler ใช้งานไม่ได้เป็น NOT VERIFIED พร้อมเหตุผล

## 6. ตรวจ Service metadata ก่อนติดตั้ง

**บัญชี:** Administrator ของ A; คำสั่ง metadata ไม่ต้อง elevation และไม่ติดตั้ง Service

~~~powershell
Set-Location -LiteralPath 'C:\Lab\thesis-agent'
.\build\thesis-agent-dev.exe --service-info
if ($LASTEXITCODE -ne 0) { throw 'Metadata failed' }

$agentExe = (Resolve-Path -LiteralPath '.\build\thesis-agent-dev.exe').ProviderPath
$fromRepo = & $agentExe --service-info
if ($LASTEXITCODE -ne 0) { throw 'Metadata failed' }
$meta = $fromRepo | ConvertFrom-Json

Push-Location -LiteralPath $env:TEMP
try {
    $fromTemp = & $agentExe --service-info
    if ($LASTEXITCODE -ne 0) { throw 'Metadata from alternate CWD failed' }
} finally {
    Pop-Location
}
"Metadata Same: $($fromRepo -ceq $fromTemp)"
$meta | Select-Object name, display_name, executable
$meta.paths | Format-List
~~~

คาดหวัง Metadata Same: True และ paths ตรงตารางข้อ 1 ตาม Known Folders ของ A ไม่ชี้ repository หรือ C:\Windows\System32 ต้องตรวจก่อน Install

หลังติดตั้งแล้ว **ทุกครั้งที่เปิด terminal ใหม่หรือ reboot** ใช้ block นี้เพื่ออ่าน metadata ใหม่ ไม่ต้องพึ่งตัวแปรจาก terminal เดิม:

~~~powershell
$installedAgent = Join-Path ([Environment]::GetFolderPath('ProgramFiles')) 'ThesisAgentDev\thesis-agent.exe'
$metaText = & $installedAgent --service-info
if ($LASTEXITCODE -ne 0) { throw 'Installed Agent metadata failed' }
$meta = $metaText | ConvertFrom-Json
~~~

จะอ้าง block นี้ว่า **โหลด installed metadata** ในข้อถัดไป

## 7. เตรียม configuration สำหรับ Service

**บัญชี:** Administrator ของ A, elevated Windows PowerShell 5.1

เตรียม .env ต้นทางใน directory แยกที่มี ACL เฉพาะ SYSTEM/Administrators สคริปต์ Install จะ copy ไป persistent runtime path อย่าใช้ .env จริงของเครื่องพัฒนาโดยไม่ตรวจ endpoints

สร้างพื้นที่ input/backup เฉพาะรอบทดสอบด้วย ACL ตั้งแต่สร้าง directory และไม่แตะ ACL ของ directory อื่น:

~~~powershell
$testInput = Join-Path ([Environment]::GetFolderPath('CommonApplicationData')) ('ThesisAgentDev-TestInput-' + [guid]::NewGuid().ToString('N'))
$inputACL = New-Object System.Security.AccessControl.DirectorySecurity
$inputACL.SetSecurityDescriptorSddlForm('O:BAG:BAD:P(A;OICI;FA;;;SY)(A;OICI;FA;;;BA)')
[void][IO.Directory]::CreateDirectory($testInput, $inputACL)
$configSource = Join-Path $testInput 'service.env'

@'
AGENT_API_URL=http://SERVER-IP:8080
WS_SERVER_URL=ws://SERVER-IP:8081/ws
POWER_MODE=mock
'@ | Set-Content -LiteralPath $configSource -Encoding UTF8

notepad.exe $configSource
$configSource
icacls.exe $testInput
~~~

**แก้ SERVER-IP เป็น address จริงแล้วบันทึกไฟล์ก่อนทำต่อ** ถ้า backend อยู่ B อย่าใช้ localhost เพราะจาก A จะหมายถึง A เอง บันทึก path ของ $testInput และ $configSource ไว้ใช้เมื่อเปิด terminal ใหม่

POWER_MODE=mock ช่วยไม่ให้ lifecycle test ส่งคำสั่ง shutdown เครื่องจริง ค่า default ใน code เป็น real จึงต้องตรวจ log ว่า **power controller mode: mock** หลัง Service เริ่ม และคู่มือนี้ไม่ต้องส่ง power command

Environment ที่มีอยู่แล้วของ process มีลำดับเหนือ .env; shell ของ Administrator กับ SCM อาจได้ environment ต่างกัน ตรวจ POWER_MODE ของทั้งสอง scope:

~~~powershell
[pscustomobject]@{
    ProcessPowerMode = [Environment]::GetEnvironmentVariable('POWER_MODE', 'Process')
    MachinePowerMode = [Environment]::GetEnvironmentVariable('POWER_MODE', 'Machine')
}
~~~

ถ้ามี override ขัดกัน ให้ผู้ดูแลทบทวนตาม policy ก่อนทดสอบ อย่าอาศัยเพียง $env:POWER_MODE='mock' ใน terminal เพราะไม่ได้รับประกันว่า Service จะเห็นค่านั้น การเปลี่ยน machine environment อาจต้องรอ system restart

ไม่ใส่ enrollment token, password, cookie หรือ private key ใน config, arguments หรือบันทึกผล การตรวจ network ขั้นต้นทำด้วย endpoint ที่ไม่มี credentials:

~~~powershell
$serverHost = 'SERVER-IP' # แก้เป็น host จริง
Test-NetConnection -ComputerName $serverHost -Port 8080
Test-NetConnection -ComputerName $serverHost -Port 8081
~~~

TcpTestSucceeded ยืนยันเพียง TCP reachability ไม่ยืนยัน enrollment หรือ Agent Online

## 8. เลือก identity ให้ถูกกรณี

### 8.1 Existing Agent identity ของ installation นี้

ใช้เมื่อ identity เป็นของ installation บน A จริง เช่นย้ายจาก console mode ของ A ไป Service ไม่ใช่ copy Agent จากอีกเครื่องเพื่อให้เครื่องใหม่ enrollment ผ่าน

ห้าม clone identity เดียวหลายเครื่อง, ลบ identity เพื่อแก้ enrollment, แก้ key fields เอง หรือพิมพ์ ciphertext/private key ลงรายงาน หาก A เป็นเครื่องทดสอบใหม่ที่มีตัวตนแยก ให้ใช้ข้อ 8.2

**บัญชี:** Administrator ของ A, elevated; ทำ backup ก่อน migration ด้วย directory ที่เตรียมในข้อ 7:

~~~powershell
$identitySource = 'D:\ExistingAgent\agent_config.json' # แก้เป็น absolute path จริงของ A
if (-not (Test-Path -LiteralPath $identitySource -PathType Leaf)) { throw 'Identity source missing' }

$sourceSummary = [pscustomobject]@{
    AgentID = (Get-Content -LiteralPath $identitySource -Raw | ConvertFrom-Json).agent_id
    IdentitySHA256 = (Get-FileHash -LiteralPath $identitySource -Algorithm SHA256).Hash
}
$identityBackup = Join-Path $testInput 'agent_config.backup.json'
Copy-Item -LiteralPath $identitySource -Destination $identityBackup -ErrorAction Stop
icacls.exe $identityBackup
$sourceSummary
~~~

Backup มี key material จึงเก็บเฉพาะพื้นที่ที่ตรวจแล้วว่า SYSTEM/Administrators เข้าถึงได้ รายงานใช้เพียง AgentID และ SHA-256 ไม่แนบ backup

Migration จะ copy bytes ไปเมื่อ destination ยังไม่มี เก็บ source เดิมไว้ และไม่ overwrite identity ที่ขัดแย้งกัน ถ้า destination มี identity เดียวกันอยู่แล้ว ไฟล์ destination ยังคงเป็นตัวจริง ไม่จำเป็นต้องเปลี่ยน formatting ให้ตรง source

DPAPI ciphertext ถูกเก็บแบบ opaque ไม่มี decrypt/re-encrypt และ **ไม่ได้พิสูจน์** ว่า LocalSystem decrypt ข้อมูลที่สร้างโดยบัญชีอื่นได้ runtime ปัจจุบันไม่ต้อง decrypt

### 8.2 Genuinely new Agent

ใช้ -NewIdentity เฉพาะ installation ใหม่ที่ไม่มี persistent identity เดิม การสร้าง identity และรับ token อยู่ใน **interactive administrative provisioning** ก่อนติดตั้ง Service

Service mode จะไม่อ่าน token จาก stdin และจะไม่สร้าง identity ใหม่เงียบ ๆ เมื่อไฟล์หาย/เสีย ไม่ต้องทำ malformed/missing identity test โดยลบไฟล์จริงในการทดสอบนี้

## 9. Install Service

**บัญชี:** Administrator ของ A, elevated Windows PowerShell 5.1; backend ต้องพร้อม provisioning

ตั้ง source directory และ config path ใหม่ให้ครบหากเปิด terminal ใหม่:

~~~powershell
Set-Location -LiteralPath 'C:\Lab\thesis-agent'
$configSource = 'C:\ProgramData\ThesisAgentDev-TestInput-REPLACE\service.env' # แก้เป็น path จริงจากข้อ 7
~~~

เลือก **คำสั่งเดียว** ตาม identity case:

~~~powershell
# Existing identity ของ installation นี้: แก้ IdentityPath ก่อนใช้
.\scripts\dev-service.ps1 -Action Install -ConfigPath $configSource -IdentityPath 'D:\ExistingAgent\agent_config.json'
~~~

หรือ:

~~~powershell
# Installation ใหม่เท่านั้น
.\scripts\dev-service.ps1 -Action Install -ConfigPath $configSource -NewIdentity
~~~

ถ้า persistent identity/config ของ installation นี้มีอยู่แล้ว และเป็นการ repair ที่ได้รับอนุญาต ให้ Stop ก่อน แล้วใช้ -Action Install โดยไม่ใส่ -NewIdentity สคริปต์จะ reuse state เดิม หยุดทันทีถ้ามี identity conflict

ถ้า REST /exists ตอบ 404 provisioning จะถาม **Agent registration token:** กรอกผ่าน prompt ที่เครื่อง A เท่านั้น อย่าเปิด transcript/บันทึกภาพระหว่างพิมพ์ token เพราะ prompt เดิมไม่ได้ซ่อนการพิมพ์แบบ password เมื่อ registered แล้ว REST ตอบ 204 ไม่ต้องกรอกซ้ำ

สคริปต์สร้าง dedicated directories/ACL, copy binary/config, provision, ลง Event Log source, configure SCM แล้ว Start ผลสำเร็จที่คาด:

~~~text
Service is Running. Verify API/WebSocket connectivity in the protected Agent log.
~~~

ตรวจต่อ:

~~~powershell
.\scripts\dev-service.ps1 -Action Status
sc.exe qc ThesisAgentDev
sc.exe queryex ThesisAgentDev
sc.exe qfailure ThesisAgentDev
sc.exe qfailureflag ThesisAgentDev
sc.exe sdshow ThesisAgentDev
~~~

| รายการ | คาดหวัง |
| --- | --- |
| START_TYPE | 2 AUTO_START; ไม่ใช่ delayed automatic |
| SERVICE_START_NAME | LocalSystem |
| STATE | 4 RUNNING |
| BINARY_PATH_NAME | executable จาก --service-info ครอบด้วย quotes ตามด้วย --service |
| Recovery | Restart 5000 ms, Restart 30000 ms, No action; reset 86400 seconds |
| Non-crash failures flag | Disabled / 0 |
| Service DACL | SYSTEM/Administrators ควบคุมได้; Users query ได้ แต่ไม่มีสิทธิ์ mutation |

SDDL ที่ implementation ตั้งคือ:

~~~text
O:BAG:BAD:(A;;GA;;;SY)(A;;GA;;;BA)(A;;CCLCSWLORC;;;BU)
~~~

Windows อาจแสดง SDDL ที่จัดรูปใหม่ได้ ให้พิจารณาสิทธิ์จริงร่วมกับ Standard User tests การอ่าน DACL อย่างเดียวไม่ใช่หลักฐานว่าทดสอบ effective permissions ผ่านแล้ว

Install ที่ล้มเหลวอาจเหลือ directories หรือ stopped registration ไม่ใช่ transactional installer อย่าแก้โดยลบ persistent state แล้วเริ่มใหม่

## 10. ตรวจ Service หลังติดตั้งครั้งแรก

**บัญชี:** Administrator ของ A, elevated; โหลด installed metadata ตามข้อ 6 ก่อน

~~~powershell
Get-Service -Name ThesisAgentDev
sc.exe queryex ThesisAgentDev
Get-Content -LiteralPath $meta.paths.log -Tail 100
~~~

เทียบเท่าการอ่าน C:\ProgramData\ThesisAgentDev\logs\agent.log บนเครื่องมาตรฐาน ตรวจ log ใหม่ของรอบที่เพิ่งเริ่ม ไม่ใช้บรรทัดจากรอบก่อน:

~~~text
agent starting
service starting
runtime starting; service=true provisioning=false
agent identity successfully loaded
service running; application connectivity is reported separately
verifying enrollment
enrollment verified
websocket connecting
websocket connected
ping ok
~~~

บางบรรทัดมี suffix/timestamps เพิ่ม และ service running กับ verifying enrollment อาจสลับลำดับเพราะทำงานคนละ goroutine เมื่อ backend offline จะเห็น retry แทน connectivity lines; ping ตาม config ปัจจุบันมี interval 20 วินาที จึงอาจต้องรอ

ตรวจ power controller mode: mock และให้ฝั่ง B ยืนยัน connection ของ Agent ID ที่ถูกต้อง ถ้า log ระบุ real ให้หยุด acceptance run และทบทวน config/environment ก่อนทดสอบต่อ

สำหรับข้อ 11–23 ให้บันทึก account, เวลา (พร้อม timezone), คำสั่ง, exit/error และหลักฐานของแต่ละข้อ ไม่ใช้ผลจากเครื่องพัฒนาเดิมเป็นผลของ A

## 11. Administrator Start / Stop / Restart

**วัตถุประสงค์:** ยืนยันว่า Administrator จัดการ Service ได้ และ intentional Stop ไม่ถูก recovery เปิดกลับ
**บัญชี:** Administrator ของ A, elevated

~~~powershell
Start-Service -Name ThesisAgentDev
Get-Service -Name ThesisAgentDev

Restart-Service -Name ThesisAgentDev
Get-Service -Name ThesisAgentDev

Stop-Service -Name ThesisAgentDev
Start-Sleep -Seconds 40
Get-Service -Name ThesisAgentDev
sc.exe queryex ThesisAgentDev
~~~

**คาดหวัง / PASS:** Start/Restart กลับ Running; หลัง Stop และรอ 40 วินาทีเป็น STOPPED, PID=0 ไม่มี restart เอง ตรวจ stop log ของรอบนี้ตามข้อ 21

**FAIL:** Administrator ถูกปฏิเสธทั้งที่ elevated ถูกต้อง, Start/Restart ล้มเหลวเพราะ implementation หรือ Service เปิดกลับหลัง intentional Stop
**NOT VERIFIED:** ยังไม่มี installation ที่ใช้งานได้ หรือไม่มี elevated terminal

**หลักฐาน:** output แต่ละคำสั่ง, เวลาสั่ง Stop/เวลาตรวจซ้ำ, queryex และ lifecycle log

จบแล้วเปิด Service สำหรับข้อ 12–13:

~~~powershell
Start-Service -Name ThesisAgentDev
sc.exe queryex ThesisAgentDev
~~~

## 12. Standard User — ป้องกันการ terminate process

**วัตถุประสงค์:** ตรวจว่าบัญชี Standard User terminate process ของ Service ไม่ได้
**บัญชี:** Actual Standard User ของ A; ไม่ elevation

ตรวจ whoami และ whoami /groups ในบัญชีนี้ก่อน จากนั้น:

~~~powershell
$agentService = Get-CimInstance Win32_Service -Filter "Name='ThesisAgentDev'"
if ($null -eq $agentService -or $agentService.State -ne 'Running' -or $agentService.ProcessId -eq 0) {
    throw 'NOT VERIFIED: Service must already be Running'
}
$beforePid = $agentService.ProcessId
$agentService | Select-Object Name, State, ProcessId, PathName
~~~

ตรวจว่าเป็น ThesisAgentDev และ path ของ installed Agent แล้วรันคำสั่ง terminate เพียงครั้งเดียว:

~~~powershell
Stop-Process -Id $agentService.ProcessId -Force -ErrorAction Stop
~~~

คาดหวัง Access is denied หรือข้อความปฏิเสธสิทธิ์ตามภาษาของ Windows จากนั้นรันแยก:

~~~powershell
sc.exe queryex ThesisAgentDev
$afterService = Get-CimInstance Win32_Service -Filter "Name='ThesisAgentDev'"
"Same PID: $($afterService.ProcessId -eq $beforePid)"
$afterService | Select-Object State, ProcessId
~~~

**PASS:** terminate ถูกปฏิเสธและ Service ยัง Running ด้วย PID เดิม ฝั่ง server/log ยังทำงาน
**FAIL:** terminate สำเร็จ แม้ SCM recovery จะเปิด process ใหม่กลับมา ก็ยังถือว่า protection ล้มเหลว
**NOT VERIFIED:** อ่าน Service/PID ไม่ได้, ใช้บัญชีผิด, Service ไม่ Running หรือมี crash/restart อื่นคั่นจนพิสูจน์เหตุการณ์ไม่ได้

**หลักฐาน:** account/groups, PID ก่อน/หลัง, denial และ queryex หากใช้ Task Manager เพิ่ม ให้ระบุ process จาก PID เดียวกัน เลือก End task แบบไม่ elevation และบันทึก denial แยก

> หาก mutation ใดสำเร็จผิดคาด ให้หยุด acceptance run เก็บหลักฐานและตรวจ permissions อย่า reinstall ทับเพื่อซ่อนผล FAIL

## 13. Standard User — ป้องกัน Service control

**วัตถุประสงค์:** ป้องกัน Standard User Stop, Disable และ Delete Service
**บัญชี:** Actual Standard User ของ A; Service ต้อง Running ก่อนเริ่ม

รันแต่ละคำสั่งแยกกันและบันทึก error ก่อนทำคำสั่งถัดไป:

~~~powershell
Stop-Service -Name ThesisAgentDev -ErrorAction Stop
~~~

~~~powershell
sc.exe stop ThesisAgentDev
$LASTEXITCODE
~~~

~~~powershell
sc.exe config ThesisAgentDev start= disabled
$LASTEXITCODE
~~~

~~~powershell
sc.exe delete ThesisAgentDev
$LASTEXITCODE
~~~

สำหรับ sc.exe คาดหวัง FAILED 5 / Access is denied ไม่ใช่ exit 0 ตรวจหลังแต่ละ attempt ว่า Service ยังมีอยู่:

~~~powershell
sc.exe queryex ThesisAgentDev
sc.exe qc ThesisAgentDev
~~~

**คาดหวัง / PASS:** ทุก mutation ถูกปฏิเสธ, Service ยังคง Running และ AUTO_START ไม่มีการลบ registration
**FAIL:** Stop/Disable/Delete สำเร็จข้อใดข้อหนึ่ง ให้หยุดทันที ไม่รันคำสั่งถัดไป
**NOT VERIFIED:** ไม่ได้ทดสอบด้วย Standard User, Service ไม่ได้ติดตั้ง/ไม่ Running หรือ error เกิดจาก syntax/ชื่อผิด ไม่ใช่ permission

**หลักฐาน:** ผลแยกของทั้งสี่คำสั่ง, native exit codes, queryex และ qc หลังทดสอบ

คำว่า **STOPPABLE** ใน queryex หมายถึง Service รับ SCM Stop control ได้ ไม่ได้หมายความว่าทุกบัญชีมีสิทธิ์ส่ง Stop หาก Services UI แสดงปุ่ม Stop ให้ลองแบบไม่ elevation; ผลต้องถูกปฏิเสธเช่นเดียวกัน

## 14. Standard User — protected-file write/delete access

**วัตถุประสงค์:** ตรวจ write และ delete access ของ binary, identity, config และ enrollment state โดย **ไม่เขียน ไม่ truncate และไม่ลบไฟล์จริง**

### 14.1 เตรียมโดย Administrator

**บัญชี:** Administrator ของ A, elevated; โหลด installed metadata ตามข้อ 6

~~~powershell
Stop-Service -Name ThesisAgentDev
(Get-Service -Name ThesisAgentDev).WaitForStatus([ServiceProcess.ServiceControllerStatus]::Stopped, [TimeSpan]::FromSeconds(30))

$targets = @($meta.executable, $meta.paths.identity, $meta.paths.config, $meta.paths.enrollment)
foreach ($target in $targets) {
    if (-not (Test-Path -LiteralPath $target -PathType Leaf)) { throw "NOT VERIFIED: missing file $target" }
    icacls.exe $target
}
icacls.exe $meta.paths.install
icacls.exe $meta.paths.root
~~~

หยุดเพื่อไม่ให้ file locks ทำให้เข้าใจผิดว่า ACL ป้องกันสำเร็จ ต้องยืนยันจาก Admin ว่าไฟล์ครบก่อน Standard User probe

### 14.2 Probe โดย Actual Standard User

**บัญชี:** Actual Standard User ของ A, Windows PowerShell 5.1; ไม่ elevation

ใช้ CreateFileW แบบ OPEN_EXISTING ไม่มี delete-on-close และปิด handle ทันที ข้อมูลไฟล์ไม่ถูกอ่าน/เขียน เป้าหมายคือขอสิทธิ์ GENERIC_WRITE=0x40000000 กับ DELETE=0x00010000 แยกกัน

~~~powershell
if (-not ('AgentFileAccessProbe' -as [type])) {
Add-Type @'
using System;
using System.Runtime.InteropServices;
using Microsoft.Win32.SafeHandles;
public static class AgentFileAccessProbe {
    [DllImport("kernel32.dll", CharSet=CharSet.Unicode, SetLastError=true)]
    public static extern SafeFileHandle CreateFileW(
        string path, uint access, uint share, IntPtr security,
        uint creation, uint flags, IntPtr template);
}
'@
}

$programs = Join-Path ([Environment]::GetFolderPath('ProgramFiles')) 'ThesisAgentDev'
$runtime = Join-Path ([Environment]::GetFolderPath('CommonApplicationData')) 'ThesisAgentDev'
$targets = @(
    (Join-Path $programs 'thesis-agent.exe'),
    (Join-Path $runtime 'agent_config.json'),
    (Join-Path $runtime '.env'),
    (Join-Path $runtime 'enrollment_state.json')
)
$requests = @(
    [pscustomobject]@{Name='GENERIC_WRITE'; Mask=[uint32]0x40000000},
    [pscustomobject]@{Name='DELETE'; Mask=[uint32]0x00010000}
)
$results = foreach ($target in $targets) {
    foreach ($request in $requests) {
        $handle = [AgentFileAccessProbe]::CreateFileW(
            $target, $request.Mask, 7, [IntPtr]::Zero, 3, 128, [IntPtr]::Zero)
        $win32Error = [Runtime.InteropServices.Marshal]::GetLastWin32Error()
        try {
            if (-not $handle.IsInvalid) {
                $result = 'FAIL'
                $win32Error = 0
            } elseif ($win32Error -eq 5) {
                $result = 'PASS'
            } else {
                $result = 'NOT VERIFIED'
            }
            [pscustomobject]@{
                Path=$target
                Access=$request.Name
                Win32Error=$win32Error
                Result=$result
            }
        } finally {
            $handle.Dispose()
        }
    }
}
$results | Format-Table -AutoSize
~~~

| ผล | การตีความ |
| --- | --- |
| Win32Error=5 | Access denied — คาดหวัง |
| Win32Error=2 หรือ 3 | File/path missing — NOT VERIFIED |
| Win32Error=32 | Sharing violation — NOT VERIFIED; ทบทวนการ Stop |
| ได้ valid handle | ได้ write/delete access — FAIL แม้ยังไม่ได้แก้ไฟล์ |
| Add-Type ถูก policy บล็อก | NOT VERIFIED; ไม่ลด policy เพื่อผ่าน test |

**PASS:** ทั้ง 8 แถวเป็น error 5 และมีหลักฐานจาก Admin ว่าเป้าหมายครบ
**FAIL:** ได้สิทธิ์อย่างใดอย่างหนึ่งในไฟล์ใดก็ตาม; หยุดและตรวจ ACL
**NOT VERIFIED:** error อื่นหรือรันไม่ครบ ห้ามนับการเปิดไฟล์ไม่ได้ทุกกรณีเป็น PASS

**หลักฐาน:** account, รายการไฟล์ที่ Admin ตรวจ, icacls และผล 8 แถว ไม่ต้องแนบ contents

Probe นี้เป็นหลักฐานการขอสิทธิ์ ไม่ใช่การลบจริง ไม่จำเป็นต้องลอง Remove-Item กับ identity/config เพิ่ม อ่านรายละเอียด API ได้ที่ [Microsoft CreateFileW](https://learn.microsoft.com/en-us/windows/win32/api/fileapi/nf-fileapi-createfilew)

หลังจบ ให้ Administrator เปิด Service กลับ:

~~~powershell
Start-Service -Name ThesisAgentDev
Get-Service -Name ThesisAgentDev
~~~

## 15. บันทึก identity/config baseline ก่อน reboot

**วัตถุประสงค์:** เก็บค่าอ้างอิงที่ไม่เปิดเผย key material สำหรับเปรียบเทียบหลัง reboot
**บัญชี:** Administrator ของ A, elevated; โหลด installed metadata ตามข้อ 6

~~~powershell
$baselinePath = Join-Path $meta.paths.root 'external-machine-test-baseline.json'
if (Test-Path -LiteralPath $baselinePath) {
    throw 'Baseline already exists; preserve it and select the correct test round before continuing'
}
$baseline = [pscustomobject]@{
    AgentID = (Get-Content -LiteralPath $meta.paths.identity -Raw | ConvertFrom-Json).agent_id
    IdentitySHA256 = (Get-FileHash -LiteralPath $meta.paths.identity -Algorithm SHA256).Hash
    ConfigSHA256 = (Get-FileHash -LiteralPath $meta.paths.config -Algorithm SHA256).Hash
    RecordedAtUTC = [DateTime]::UtcNow.ToString('o')
}
$baseline | ConvertTo-Json | Set-Content -LiteralPath $baselinePath -Encoding UTF8
icacls.exe $baselinePath
$baseline | Select-Object AgentID, IdentitySHA256, ConfigSHA256, RecordedAtUTC
~~~

ถ้ามี baseline ของรอบก่อน ให้เก็บไว้ เลือกชื่อใหม่สำหรับรอบใหม่และใช้ชื่อนั้นเหมือนกันในข้อ 17–18 อย่า overwrite หลักฐานก่อนเปรียบเทียบ

**คาดหวัง / PASS:** มี Agent ID และ SHA-256 สองรายการ บันทึกได้ใน protected runtime directory อ่านกลับตรงกัน
**FAIL:** พบ identity/config เปลี่ยนไปตั้งแต่ก่อน reboot โดยไม่มีการเปลี่ยนที่อนุญาต
**NOT VERIFIED:** อ่านไฟล์ไม่ได้หรือไม่มี baseline ที่ระบุรอบทดสอบชัดเจน

**หลักฐาน:** summary นี้เท่านั้น ไม่ใช้ full agent_config.json เป็นรายงาน และไม่ใช้ enrollment-state hash เป็นตัวตัดสิน identity เพราะ marker สามารถถูกเขียนใหม่หลัง verification ได้

สำหรับ migration ไป destination ที่ยังไม่มี identity ให้เทียบ SHA-256 source/installed ด้วย ต้องเหมือนกันทุก byte; หาก destination เดิมมี identity เดียวกันอยู่แล้ว ให้ใช้ destination baseline ตามพฤติกรรมข้อ 8

## 16. Reboot / Auto Start

**วัตถุประสงค์:** ตรวจ Service เริ่มเองหลัง Windows boot
**บัญชี:** Administrator ของ A; ถ้าทำ before-login proof ให้ผู้สังเกตบน B พร้อมตามข้อ 22 ก่อน reboot ครั้งนี้

ก่อน reboot:

~~~powershell
sc.exe qc ThesisAgentDev
Get-Service -Name ThesisAgentDev
Get-Date -Format o
~~~

ยืนยัน AUTO_START และ installation/provisioning ผ่านแล้ว จากนั้นผู้ทดสอบเลือก **Start menu > Power > Restart** ตามเวลาที่ตกลง ไม่สั่ง Start-Service หรือเปิด Agent เองหลัง boot ก่อนเก็บหลักฐาน

หลัง boot ให้ login เพื่อเก็บหลักฐานเมื่อพร้อม ถ้าทำข้อ 22 ด้วย ให้รอการสังเกตจาก B ให้เสร็จก่อน login เปิด elevated PowerShell ใหม่และโหลด installed metadata ตามข้อ 6:

~~~powershell
Get-Service -Name ThesisAgentDev
sc.exe queryex ThesisAgentDev
(Get-CimInstance Win32_OperatingSystem).LastBootUpTime
(Get-CimInstance Win32_OperatingSystem).LastBootUpTime.ToUniversalTime()
Get-Content -LiteralPath $meta.paths.log -Tail 100
~~~

**คาดหวัง / PASS:** Service Running โดยไม่มีการ Start ด้วยมือ มี lifecycle log ของ boot นี้ และ startup config เป็น AUTO_START
**FAIL:** ไม่เริ่มเองจากการติดตั้งที่ถูกต้อง หรือกลายเป็น Stopped เพราะ lifecycle ผิดพลาด
**NOT VERIFIED:** มีคนเปิด Agent/Start-Service ก่อนตรวจ หรือไม่มีหลักฐานเวลาที่แยก boot นี้ได้

**หลักฐาน:** qc, queryex, boot time และ log ของ boot นี้ (Agent log ใช้ UTC)

ถ้ายังไม่มีการสังเกตบน B ขณะ A อยู่หน้า login ให้บันทึกแยก:

~~~text
Auto Start after boot: PASS
Auto Start before interactive login: NOT VERIFIED
~~~

สถานะ Automatic อย่างเดียวไม่พิสูจน์ข้อใดข้อหนึ่งว่าได้ทดสอบผ่านจริง

## 17. Identity/config stability หลัง reboot

**วัตถุประสงค์:** ตรวจว่า runtime ไม่สร้าง identity หรือเปลี่ยน config เงียบ ๆ
**บัญชี:** Administrator ของ A, elevated; โหลด installed metadata ตามข้อ 6 ใหม่

~~~powershell
$baselinePath = Join-Path $meta.paths.root 'external-machine-test-baseline.json'
$baseline = Get-Content -LiteralPath $baselinePath -Raw | ConvertFrom-Json
$currentID = (Get-Content -LiteralPath $meta.paths.identity -Raw | ConvertFrom-Json).agent_id
$currentIdentityHash = (Get-FileHash -LiteralPath $meta.paths.identity -Algorithm SHA256).Hash
$currentConfigHash = (Get-FileHash -LiteralPath $meta.paths.config -Algorithm SHA256).Hash

"AgentID Same: $($currentID -ceq $baseline.AgentID)"
"Identity Same: $($currentIdentityHash -eq $baseline.IdentitySHA256)"
"Config Same: $($currentConfigHash -eq $baseline.ConfigSHA256)"
~~~

**คาดหวัง / PASS:**

~~~text
AgentID Same: True
Identity Same: True
Config Same: True
~~~

**FAIL:** มี False ทั้งที่ไม่มี authorized edit/migration ระหว่าง baseline กับ reboot; เก็บทั้งไฟล์ไว้ในพื้นที่ป้องกันเพื่อวิเคราะห์ ไม่ regenerate
**NOT VERIFIED:** baseline หาย/ผิดรอบ หรือมีการแก้ config โดยตั้งใจก่อนเทียบ จนแยกสาเหตุไม่ได้

**หลักฐาน:** ผล Boolean ทั้งสามกับเวลาและ Agent ID ไม่พิมพ์ key fields การ hash เท่ากันพิสูจน์ byte stability ไม่ได้พิสูจน์ DPAPI decryption ภายใต้ LocalSystem

## 18. REST/API unavailable ตอน boot และฟื้นตัวเอง

**วัตถุประสงค์:** ตรวจว่า Service ที่ provisioned แล้วคง Running และ retry จน API กลับมา โดยไม่ต้อง restart Agent
**บัญชี:** Administrator ของ A และผู้ดูแล test backend บน B

ใช้ backend ทดสอบเท่านั้น ต้องมี baseline และ Agent row ที่ลงทะเบียนแล้ว ห้ามลบ row หรือ token เพื่อจำลอง outage เพราะ 404 เป็นคนละกรณี

1. บน B หยุด **เฉพาะ REST** ด้วยวิธีเดิมที่ใช้เปิด backend หากเปิดแบบ foreground ให้กด Ctrl+C ใน terminal ของ thesis-rat-server เท่านั้น คง WS/DB ไว้
2. บน A ยืนยันว่า configured REST เข้าไม่ได้ แล้ว reboot ผ่าน Windows UI ขณะ REST ยัง down
3. หลัง boot เปิด elevated terminal ใหม่ โหลด installed metadata และตรวจ:

~~~powershell
Get-Service -Name ThesisAgentDev
sc.exe queryex ThesisAgentDev
$outagePid = (Get-CimInstance Win32_Service -Filter "Name='ThesisAgentDev'").ProcessId
Get-Content -LiteralPath $meta.paths.log -Tail 100
~~~

คาดหวัง Running และบรรทัดลักษณะนี้หลายครั้ง:

~~~text
API unavailable; enrollment verification pending; retry scheduled in ...
~~~

ไม่มี token prompt จาก Service และ identity ยังเท่า baseline (รันข้อ 17 ซ้ำ) รอให้เห็น retries จริง อย่า restart Agent วนไปมา

4. เปิด REST บน B กลับด้วย **คำสั่ง/environment เดิม** ตัวอย่างสำหรับ backend ที่เตรียมไว้และรัน foreground ด้วย Go อยู่แล้ว:

~~~powershell
# บน B เท่านั้น; แก้ path เป็น repository ของ REST ที่เตรียมไว้
Set-Location -LiteralPath 'C:\Lab\thesis-rat-server'
go run .
~~~

ถ้า backend ใช้ container/service manager ให้ใช้ operation ของ test deployment เดิมแทนตัวอย่าง Go ไม่เปิด server อีกชุดทับ port เดิม

5. บน A ไม่เรียก Start/Restart; เฝ้า log และ query PID:

~~~powershell
sc.exe queryex ThesisAgentDev
$restRecovered = Get-CimInstance Win32_Service -Filter "Name='ThesisAgentDev'"
"Same PID: $($restRecovered.ProcessId -eq $outagePid)"
Get-Content -LiteralPath $meta.paths.log -Wait -Tail 30
~~~

กด Ctrl+C เพื่อหยุด tail เท่านั้น ไม่หยุด Service คาดหวัง enrollment verified, websocket connecting, websocket connected และ ping ok เมื่อ WS พร้อมด้วย

**PASS:** Running ต่อเนื่องระหว่าง API outage, identity ไม่เปลี่ยน, ไม่ prompt, หลัง API กลับมาจึง Online เองด้วย PID เดิมของ boot นี้
**FAIL:** temporary API outage ทำ Service จบถาวร, เปลี่ยน identity หรือฟื้นได้เฉพาะหลัง restart ด้วยมือ
**NOT VERIFIED:** API ยัง down, restore แล้วไม่ตอบ /exists เป็น 204, WS ไม่พร้อม หรือ outage ไม่ได้เกิดตอน boot

**หลักฐาน:** เวลาหยุด/เปิด REST บน B, boot time, PID, retry/recovery log, baseline comparison และ Online ล่าสุดจาก server

REST 404 หมายถึง ID ไม่อยู่ใน configured server แม้เคยมี marker ก็ต้องหยุดและ provision โดย Admin ตามเดิม ส่วน 401/403 ให้ทบทวน configuration ไม่ใช่ temporary outage ที่คาดว่าจะ retry ไม่สิ้นสุด

## 19. WebSocket outage และ reconnect

**วัตถุประสงค์:** ตรวจ reconnect/backoff โดย process Service ไม่ต้อง restart
**บัญชี:** Administrator ของ A และผู้ดูแล WS บน B; REST ต้องพร้อมและ Agent Online ก่อนเริ่ม

บน A โหลด installed metadata แล้วบันทึก PID:

~~~powershell
$wsBefore = Get-CimInstance Win32_Service -Filter "Name='ThesisAgentDev'"
if ($wsBefore.State -ne 'Running' -or $wsBefore.ProcessId -eq 0) { throw 'NOT VERIFIED: Service is not Running' }
$wsBefore | Select-Object State, ProcessId
Get-Date -Format o
Get-Content -LiteralPath $meta.paths.log -Wait -Tail 30
~~~

บน B หยุดเฉพาะ test WS server ด้วย operation เดิม หรือ Ctrl+C ใน terminal ของ thesis-web-socket ไม่หยุด REST/DB/Dashboard รอให้เห็น retry หลายรอบ:

~~~text
websocket disconnected; reconnect scheduled in ...
websocket connecting
~~~

ปัจจุบันใช้ equal jitter ช่วงรอ 0.5–1, 1–2, 2–4, 4–8, 8–16 และ 15–30 วินาที แล้วคงช่วง 15–30 วินาที Backoff reset เมื่อ connection หลังส่ง initial message อยู่ได้อย่างน้อย 30 วินาที

ค่ารอที่สุ่มได้ **ไม่จำเป็นต้องเพิ่มทุกครั้ง** และเวลาเทียบ log รวม dial/heartbeat timeout ด้วย อย่าใช้ช่วงเวลาระหว่าง log รวมทั้งหมดตัดสินว่าค่า backoff เกิน 30 วินาที ให้ดูค่า reconnect scheduled in โดยตรง

เปิด WS บน B กลับด้วย command/environment เดิม ตัวอย่าง foreground Go ของ deployment ที่พร้อมแล้ว:

~~~powershell
# บน B เท่านั้น; แก้ path ให้ตรง WS repository
Set-Location -LiteralPath 'C:\Lab\thesis-web-socket'
go run .
~~~

บน A หยุด tail ด้วย Ctrl+C แล้วตรวจโดยไม่ Start/Restart Agent:

~~~powershell
sc.exe queryex ThesisAgentDev
$wsAfter = Get-CimInstance Win32_Service -Filter "Name='ThesisAgentDev'"
"Same PID: $($wsAfter.ProcessId -eq $wsBefore.ProcessId)"
Get-Content -LiteralPath $meta.paths.log -Tail 100
~~~

**คาดหวัง / PASS:** reconnect เอง, PID เดิม, Agent ID เดิม, server ยืนยัน connection ล่าสุด และ log มี websocket connected, websocket JSON sent, ping ok
**FAIL:** retry ไม่เกิด, Service restart จาก network outage หรือจำเป็นต้อง restart ด้วยมือเพื่อกลับมา
**NOT VERIFIED:** WS ยังเปิดไม่ได้, outage สั้นจนไม่เห็น reconnect หรือมี Administrator restart แทรก

**หลักฐาน:** PID ก่อน/ระหว่าง/หลัง, เวลาปิด/เปิด WS, retry delays หลายค่า และ server-side Online/connection ของ ID เดิม

## 20. SCM Crash Recovery

**วัตถุประสงค์:** ตรวจ recovery จาก process crash ตาม policy ของ SCM
**บัญชี:** Administrator ของ A, elevated; ทำเฉพาะ designated test PC/VM ที่มี snapshot เท่านั้น

ก่อนเริ่มต้อง restore REST/WS ให้พร้อม Service Running และ POWER_MODE=mock ไม่มี Administrator/เครื่องมืออื่น restart Service ระหว่างทดสอบ ตรวจ policy ปัจจุบันก่อนทุกชุด:

~~~powershell
sc.exe qfailure ThesisAgentDev
sc.exe qfailureflag ThesisAgentDev
sc.exe qc ThesisAgentDev
~~~

Source กำหนดลำดับ:

~~~text
1st failure -> restart after approximately 5 seconds
2nd failure -> restart after approximately 30 seconds
3rd failure -> no action
Reset period -> 86400 seconds
Non-crash failures -> disabled
~~~

เริ่มจาก test installation/VM snapshot ที่ทราบว่าไม่มี crash ก่อนหน้าในรอบ counter นี้ ถ้าเคย crash มาแล้วต้องบันทึกและคำนึงถึง counter; การ Stop/Start ไม่ใช่หลักฐานว่า counter ถูก reset ค่า 86400 วินาทีหมายถึงช่วงไม่มี failure ที่กำหนดไว้ อย่าแก้ recovery policy เพียงเพื่อให้ผลตรงตาราง หากไม่ทราบ counter ให้ระบุ NOT VERIFIED สำหรับลำดับแทนการเดา

สำหรับเครื่องที่เคยตั้ง non-crash flag ต่างออกไป Microsoft ระบุว่าการเปลี่ยน flag มีผลเมื่อ system start ครั้งถัดไป ให้ตรวจอีกครั้งหลัง reboot ตามข้อ 16 ([failure-action flags](https://learn.microsoft.com/en-us/windows/win32/api/winsvc/ns-winsvc-service_failure_actions_flag))

### 20.1 เตรียมคำสั่งตรวจ PID/path ก่อน crash ทุกครั้ง

เปิด elevated PowerShell ของ A แล้วประกาศ helper สำหรับรอบนี้ Helper จะ query Service และ process **ใหม่ทุกครั้ง** ตรวจ path ก่อนใช้ Stop-Process และไม่ใช้ PID ที่บันทึกจากรอบเก่า:

~~~powershell
function Stop-VerifiedAgentForCrashTest {
    $installedAgent = Join-Path ([Environment]::GetFolderPath('ProgramFiles')) 'ThesisAgentDev\thesis-agent.exe'
    $metaText = & $installedAgent --service-info
    if ($LASTEXITCODE -ne 0) { throw 'Cannot verify installed metadata' }
    $meta = $metaText | ConvertFrom-Json

    $agentService = Get-CimInstance Win32_Service -Filter "Name='ThesisAgentDev'"
    if ($null -eq $agentService -or $agentService.State -ne 'Running' -or $agentService.ProcessId -eq 0) {
        throw 'Service must be Running before a crash test'
    }
    $agentProcess = Get-Process -Id $agentService.ProcessId -ErrorAction Stop
    $agentService | Select-Object Name, State, ProcessId, PathName | Format-List | Out-Host
    $agentProcess | Select-Object Id, Path | Format-List | Out-Host
    if ($agentProcess.Path -ine $meta.executable) {
        throw 'PID/path mismatch: refusing to terminate'
    }

    $record = [pscustomobject]@{
        PreviousPid = $agentProcess.Id
        CrashTimeUTC = [DateTime]::UtcNow.ToString('o')
    }
    Stop-Process -Id $agentProcess.Id -Force -ErrorAction Stop
    return $record
}
~~~

การประกาศ helper ยังไม่ kill process แต่แต่ละ invocation ด้านล่างจะ kill จริง ให้ทำทีละรอบ ไม่ใส่ใน loop อัตโนมัติ

### 20.2 Crash #1

~~~powershell
$crash1 = Stop-VerifiedAgentForCrashTest
$crash1
Start-Sleep -Seconds 8
(Get-Service -Name ThesisAgentDev).WaitForStatus([ServiceProcess.ServiceControllerStatus]::Running, [TimeSpan]::FromSeconds(30))
sc.exe queryex ThesisAgentDev
$afterCrash1 = Get-CimInstance Win32_Service -Filter "Name='ThesisAgentDev'"
"New PID: $($afterCrash1.ProcessId -ne 0 -and $afterCrash1.ProcessId -ne $crash1.PreviousPid)"
~~~

**คาดหวัง / PASS:** SCM เริ่ม recovery หลังประมาณ 5 วินาที แล้ว Service Running ด้วย PID ใหม่และ identity เดิม การเริ่ม runtime อาจใช้เวลาเพิ่มจึงรอ Running ต่อได้

### 20.3 Crash #2

ทำเมื่อรอบแรกกลับ Running แล้วเท่านั้น Helper จะไม่ reuse PID ของ Crash #1:

~~~powershell
$crash2 = Stop-VerifiedAgentForCrashTest
$crash2
Start-Sleep -Seconds 35
(Get-Service -Name ThesisAgentDev).WaitForStatus([ServiceProcess.ServiceControllerStatus]::Running, [TimeSpan]::FromSeconds(30))
sc.exe queryex ThesisAgentDev
$afterCrash2 = Get-CimInstance Win32_Service -Filter "Name='ThesisAgentDev'"
"New PID: $($afterCrash2.ProcessId -ne 0 -and $afterCrash2.ProcessId -ne $crash2.PreviousPid)"
~~~

**คาดหวัง / PASS:** restart ตาม delay ประมาณ 30 วินาที แล้ว Running ด้วย PID ใหม่อีกครั้ง

### 20.4 Crash #3

~~~powershell
$crash3 = Stop-VerifiedAgentForCrashTest
$crash3
Start-Sleep -Seconds 40
sc.exe queryex ThesisAgentDev
Get-Service -Name ThesisAgentDev
~~~

**คาดหวัง / PASS:** STOPPED, PID=0 ไม่มี agent starting ใหม่หลัง CrashTimeUTC ของรอบที่สาม การตรวจ log ให้เทียบ timestamps ไม่ใช่เห็น agent starting เก่าจาก Tail แล้วสรุปว่า restart

**FAIL ของข้อ 20:** counter/policy ถูกต้องแต่ไม่ restart ในสองรอบแรก, restart หลังรอบสาม, restart รวดเร็ววนไม่สิ้นสุด หรือ identity เปลี่ยน
**NOT VERIFIED:** ไม่ทราบ counter, policy ยังไม่ใช่ค่าที่ตั้งใจ, PID/path ตรวจไม่ผ่าน, ไม่มี elevated rights หรือ backend/config error ทำให้ process ใหม่เริ่มแล้วจบทันทีจนยังแยกสาเหตุไม่ได้ ให้ตรวจ evidence ก่อนตัดสิน SCM

**หลักฐาน:** qfailure/qfailureflag, PID/path ที่ตรวจทุกครั้ง, CrashTimeUTC, PID หลัง recovery, agent starting ของแต่ละรอบ และ Windows System log ที่เกี่ยวกับ Service Control Manager ถ้าจำเป็น รัน identity comparison ข้อ 17 หลัง recovery ด้วย

จบ Crash #3 ไม่ต้องคาดหวัง WIN32_EXIT_CODE=0 เพราะเป็น crash ให้ไปตรวจ graceful Stop แยกในข้อ 21

## 21. Intentional Administrator Stop หลัง crash tests

**วัตถุประสงค์:** ยืนยัน recovery ไม่ต่อต้าน legitimate Administrator Stop
**บัญชี:** Administrator ของ A, elevated; โหลด installed metadata ตามข้อ 6

~~~powershell
Start-Service -Name ThesisAgentDev
(Get-Service -Name ThesisAgentDev).WaitForStatus([ServiceProcess.ServiceControllerStatus]::Running, [TimeSpan]::FromSeconds(30))
$stopTimeUTC = [DateTime]::UtcNow.ToString('o')
Stop-Service -Name ThesisAgentDev
Start-Sleep -Seconds 40
sc.exe queryex ThesisAgentDev
Get-Content -LiteralPath $meta.paths.log -Tail 100
$stopTimeUTC
~~~

คาดหวัง:

~~~text
STOPPED
PID = 0
WIN32_EXIT_CODE = 0
~~~

และ lifecycle log ของรอบนี้มีข้อความเทียบเท่า:

~~~text
service stopping; shutdown requested
runtime stopping; releasing workers and connections
runtime stopped
service stopped; runtime resources released
agent stopping
~~~

**PASS:** ออกจาก runtime ตามปกติ, exit code 0, ไม่มี agent starting ใหม่หลัง stop และยัง Stopped หลัง 40 วินาที
**FAIL:** เปิดกลับเอง, cleanup timeout/error ใน normal test หรือ exit ไม่สะอาด
**NOT VERIFIED:** ไม่มี Running instance ให้ Stop หรือมี automation อื่นจัดการ Service คั่น

**หลักฐาน:** เวลา Stop, queryex หลังรอ และ stop log โดยไม่สับสนกับ error ของ crash ก่อนหน้า ข้อนี้ยืนยันการหยุดปกติ ไม่ใช่การ terminate process

## 22. Optional: พิสูจน์ Online ก่อน interactive login

**วัตถุประสงค์:** พิสูจน์พฤติกรรมที่เข้มกว่าการเห็น Automatic หรือ Running หลัง login
**บัญชี:** Administrator ของ A สำหรับเตรียม/reboot และผู้สังเกตที่ได้รับสิทธิ์บน B

ต้องมี Machine B ที่ REST/WS/DB พร้อมและดู Dashboard หรือ server-side connection ของ Agent ID นี้ได้ อาจใช้หลักฐานจาก reboot ในข้อ 16 ถ้าได้ทำครบแล้ว ไม่จำเป็นต้อง reboot ซ้ำ

ถ้าทำหลังข้อ 21 ให้เริ่ม Service กลับเพื่อยืนยัน configuration ก่อน แล้ว reboot ผ่าน Windows UI:

~~~powershell
# A, Administrator
Start-Service -Name ThesisAgentDev
sc.exe qc ThesisAgentDev
Get-Date -Format o
~~~

ลำดับการสังเกต:

1. บันทึก Agent ID ที่ต้องติดตามจาก baseline และตรวจว่าไม่มีเครื่องอื่นใช้ ID ซ้ำ
2. B เปิด Dashboard/server log ที่แสดง **connection ล่าสุด** ไม่ใช่แถว ONLINE เก่าจากฐานข้อมูล
3. Reboot A ตามเวลาที่ตกลง แล้วค้างไว้ที่หน้า login ห้าม login และต้องไม่มี automatic login
4. B ยืนยัน Agent ID เดิมมี connection ใหม่/สถานะสดเป็น Online ขณะที่ A ยังไม่ได้ login; บันทึกเวลา
5. เมื่อได้หลักฐานแล้วจึง login A เปิด elevated terminal และโหลด installed metadata ตามข้อ 6
6. เทียบ boot time กับ runtime/connection log และเวลาที่ login ครั้งแรก

~~~powershell
# B: บันทึกเวลาที่สังเกตได้จริง พร้อม timezone
Get-Date -Format o
[DateTime]::UtcNow.ToString('o')
~~~

~~~powershell
# A: หลัง login แล้วและโหลด installed metadata
(Get-CimInstance Win32_OperatingSystem).LastBootUpTime
(Get-CimInstance Win32_OperatingSystem).LastBootUpTime.ToUniversalTime()
Get-Content -LiteralPath $meta.paths.log -Tail 100
sc.exe queryex ThesisAgentDev
~~~

**PASS:** มีหลักฐานว่า Agent Online จาก B จริงขณะที่ A ยังอยู่หน้า login รวม timestamp/identity ที่สอดคล้องกัน
**FAIL:** ในสภาพแวดล้อมที่ backend/network พร้อมก่อน login แต่ Agent เริ่ม/เชื่อมได้เฉพาะหลัง login และตรวจยืนยันว่าเกิดจาก Agent lifecycle
**NOT VERIFIED:** ไม่ได้สังเกตจาก B, มี automatic login, เวลาเครื่องต่างกันจนเทียบไม่ได้, เห็นเพียง ONLINE เก่า หรือเครือข่ายต้องรอ user VPN/Wi-Fi sign-in

**หลักฐาน:** ภาพหรือ log จาก B พร้อม ID/เวลา, บันทึกเวลาที่ A อยู่หน้า login/เริ่ม login และ boot/runtime timestamps ปิดบังข้อมูลผู้ใช้ที่ไม่จำเป็น

ถ้าข้าม observation นี้ ให้บันทึก **Start before login: NOT VERIFIED** แม้ข้อ 16 จะ PASS

## 23. Remove development Service หลังทดสอบ

**วัตถุประสงค์:** ตรวจ Administrator ถอน Service registration ได้ โดยเก็บ persistent identity/config ตาม design
**บัญชี:** Administrator ของ A, elevated Windows PowerShell 5.1

~~~powershell
Set-Location -LiteralPath 'C:\Lab\thesis-agent'
$installedAgent = Join-Path ([Environment]::GetFolderPath('ProgramFiles')) 'ThesisAgentDev\thesis-agent.exe'
.\scripts\dev-service.ps1 -Action Remove -Executable $installedAgent
sc.exe query ThesisAgentDev
$LASTEXITCODE
~~~

-Executable เป็น parameter ที่มีอยู่จริง ใช้เพื่ออ่าน metadata จาก installed binary หากไม่มี build executable อยู่ใน source แล้ว ถ้ายังมี build ที่ถูกต้องใช้เพียง -Action Remove ได้

คาดหวัง script รายงาน retained data และ sc.exe ตอบ FAILED 1060 / The specified service does not exist as an installed service หลัง Remove

**PASS:** registration หาย, process หยุด และ identity/config/logs/runtime data/binaries/Event Log source ยังอยู่ตาม design
**FAIL:** Administrator ลบ registration ไม่ได้หลังตรวจสิทธิ์ถูกต้อง หรือ persistent data ถูกลบผิด design
**NOT VERIFIED:** ไม่ได้รัน Remove, script ถูก policy บล็อก หรือ service ยัง marked for deletion เพราะมี handle เปิดอยู่ ให้ปิด Services UI/handle แล้ว query ใหม่ก่อนสรุป

**หลักฐาน:** output ของ Remove, exit 1060 หลังลบ และ Admin ตรวจ Test-Path/hash ของ retained identity/config ด้วย installed --service-info ซึ่งยังใช้ได้เพราะ binary ถูกเก็บไว้

ไม่สั่ง recursive delete หรือ cleanup persistent Agent data ในคู่มือนี้ พื้นที่ test input/backup จากข้อ 7 ก็ยังคงอยู่ใน ACL ที่ป้องกันไว้ ให้จัดการภายใต้ retention policy ของผู้ดูแลแยกต่างหาก

## 24. ตารางบันทึกผลการทดสอบ

เริ่มทุกแถวเป็น NOT VERIFIED แล้วแก้เฉพาะเมื่อได้หลักฐานจริง ระบุข้อ 14 ว่าเป็น **access probe** ไม่ใช่ actual delete และระบุ test ที่ทำบน A/B ให้ชัด

| Test | Result | Evidence / Notes |
| --- | --- | --- |
| Build / binary SHA-256 | NOT VERIFIED | |
| Metadata และ CWD independence | NOT VERIFIED | |
| Service installation | NOT VERIFIED | |
| Automatic startup after boot | NOT VERIFIED | |
| Start before login | NOT VERIFIED | |
| Administrator Start/Stop/Restart | NOT VERIFIED | |
| Standard User process termination | NOT VERIFIED | |
| Standard User Stop-Service | NOT VERIFIED | |
| Standard User sc.exe stop | NOT VERIFIED | |
| Standard User Disable Service | NOT VERIFIED | |
| Standard User Delete Service | NOT VERIFIED | |
| Protected binary write/delete access | NOT VERIFIED | |
| Protected identity write/delete access | NOT VERIFIED | |
| Protected config write/delete access | NOT VERIFIED | |
| Protected enrollment-state write/delete access | NOT VERIFIED | |
| Identity stability after reboot | NOT VERIFIED | |
| Config stability after reboot | NOT VERIFIED | |
| REST outage recovery at boot | NOT VERIFIED | |
| WebSocket reconnect / same PID | NOT VERIFIED | |
| Crash recovery #1 | NOT VERIFIED | |
| Crash recovery #2 | NOT VERIFIED | |
| Crash recovery #3 | NOT VERIFIED | |
| Intentional Admin Stop | NOT VERIFIED | |
| Administrator Remove / retained state | NOT VERIFIED | |
| go test ./... | NOT VERIFIED | |
| go vet ./... | NOT VERIFIED | |
| go test -race ./... | NOT VERIFIED | |

ถ้า Standard User mutation สำเร็จ บันทึก FAIL และหยุด run ไม่แก้ ACL/reinstall/สร้าง identity ใหม่แล้วเขียนทับผลเดิม หากทดสอบใหม่ภายหลัง ให้เปิดรอบใหม่พร้อมระบุสิ่งที่แก้และเก็บ evidence ของรอบที่ล้มเหลว

## 25. แบบสรุป acceptance

กรอกบนผลจริงของ **เครื่องทดสอบนี้** ไม่ copy ผล PASS จาก verification report ของเครื่องพัฒนา ระบุ source snapshot เพิ่มถ้า commit เดียวไม่รวม uncommitted changes:

~~~text
Windows Service Acceptance Summary

Machine:
Windows Version:
Administrator test account:
Standard User test account:
Agent Commit:
Working-tree / source snapshot identifier:
Agent Binary SHA-256:
Backend test environment / versions:
Test Date and timezone:

PASS:
- ...

FAIL:
- ...

NOT VERIFIED:
- ...

Known limitations:
- ...

Evidence location (access-controlled):
- ...
~~~

ห้ามแนบ private keys, encrypted_private_key, enrollment tokens, passwords, cookies หรือ credentials อื่น Log/ภาพหน้าจอที่แนบให้ตรวจและปิดบังข้อมูลอ่อนไหวก่อนแชร์ โดยเฉพาะ interactive provisioning ห้ามเก็บ transcript ของ token prompt

การมี NOT VERIFIED ใน before-login/race ไม่ต้องเขียนว่า test อื่น FAIL แต่ก็ห้ามสรุป production verification ครบทั้งหมด

## 26. Troubleshooting เฉพาะการทดสอบ Service

### 26.1 Service is not installed / error 1060

~~~powershell
sc.exe query ThesisAgentDev
~~~

1060 ก่อน Install คือยังไม่มี registration; หลัง Remove คือผลที่คาด หาก Install เพิ่งล้มเหลว ตรวจ provisioning output ก่อน ไม่ใช้ --service รันจาก terminal แทนการติดตั้ง เพราะ flag นี้ยอมรับเฉพาะ SCM

### 26.2 Service เริ่มแล้วหยุดทันที

**Administrator, elevated:** โหลด installed metadata ตามข้อ 6 แล้วตรวจ:

~~~powershell
Get-Content -LiteralPath $meta.paths.log -Tail 100
Get-WinEvent -FilterHashtable @{LogName='Application'; ProviderName='ThesisAgentDev'} -MaxEvents 20
sc.exe queryex ThesisAgentDev
sc.exe qc ThesisAgentDev
~~~

ตรวจ config/endpoints, permissions, identity, malformed enrollment marker และ startup diagnostics จาก Application Event Log หาก logging ยังเริ่มไม่ได้ อาจมีเฉพาะ Event Log; ถ้า Event Log source ยังไม่เคยติดตั้ง จะ query provider ไม่พบ ไม่ใช่หลักฐานว่ามี log ผ่านแล้ว

Startup ภายในมี timeout 30 วินาที ส่วน stop มี fallback 20 วินาที ถ้าเจอ timeout ให้เก็บ error และตรวจ native provider/runtime ไม่จัดเป็น graceful Stop PASS

### 26.3 Identity missing / malformed

Service จะไม่สร้าง replacement เอง คืน identity ของ installation นี้จาก Administrator-only backup ที่ตรวจแล้ว หรือใช้ administrative provisioning ตามข้อ 8–9 เมื่อเป็น installation ใหม่จริงเท่านั้น

ไฟล์ malformed/partial ต้องวิเคราะห์โดย Administrator ห้ามใช้ -NewIdentity เพื่อกลบปัญหา และห้ามลบไฟล์เพื่อบังคับให้สร้างใหม่

### 26.4 Identity conflict ระหว่าง migration

หยุด ไม่ overwrite source หรือ destination เก็บทั้งสองไว้ในพื้นที่ป้องกัน ตรวจ Agent ID/ownership ของ installation นี้กับ hash ก่อนตัดสินใจ คู่มือนี้ไม่อนุญาตให้เลือก identity ใหม่แทนโดยอัตโนมัติ

### 26.5 API unavailable / HTTP status

Service อาจ Running และ retry ตามปกติสำหรับ network failure, 408, 429 และ 5xx ตรวจ address, test network และ REST/DB availability โดยไม่แก้ identity

204 ยืนยัน ID มีอยู่; 404 เป็น authoritative absence ต้อง provision โดย Admin ด้วย identity เดิม ไม่ใช่ outage retry; 401/403 และ error ถาวรอื่นให้ตรวจ config ก่อน

### 26.6 WebSocket unavailable / Online ไม่ขึ้น

Running กับ retry ไม่ใช่ crash ตรวจ WS URL/port, server log และว่า REST/WS ใช้ database เดียวกัน ถ้าเห็น websocket connected แล้ว disconnect ทันทีให้ตรวจการยอมรับ Agent ID ฝั่ง server ไม่สรุป Online จาก handshake

ถ้าต้องดู log ต่อเนื่อง:

~~~powershell
Get-Content -LiteralPath $meta.paths.log -Wait -Tail 30
~~~

Ctrl+C ปิด tail ไม่ได้สั่ง Stop Service

### 26.7 Standard User mutation สำเร็จผิดคาด

บันทึก **FAIL** หยุด acceptance run และให้ Admin ตรวจ Service DACL กับ NTFS ACL:

~~~powershell
sc.exe sdshow ThesisAgentDev
icacls.exe $meta.paths.install
icacls.exe $meta.paths.root
icacls.exe $meta.executable
icacls.exe $meta.paths.identity
~~~

ตรวจด้วยว่าใช้บัญชี Standard User จริงหรือเผลอ elevation ไม่ซ่อน failure ด้วย reinstall หรือ regenerate identity และไม่เพิ่ม anti-Administrator mechanism

### 26.8 PowerShell execution policy / Add-Type ถูกบล็อก

~~~powershell
Get-ExecutionPolicy -List
$PSVersionTable
~~~

ใช้ Windows PowerShell 5.1 Desktop สำหรับ script ถ้า organizational/local policy ไม่อนุญาต ให้ใช้ approved signing/policy หรือเครื่องพัฒนาที่องค์กรอนุญาต ไม่แนะนำให้ปิด policy ถาวร ไม่ใช้ bypass เพื่อทำให้ acceptance ผ่าน ข้อที่ยังรันไม่ได้เป็น NOT VERIFIED

### 26.9 ยังเป็น real power mode หรือ environment ไม่ตรง

ตรวจ persistent .env ที่เลือกตอน Install กับ process/machine environment ตามข้อ 7 และ log ของ **Service instance ปัจจุบัน** ไม่อาศัย log ของ provisioning process ซึ่งอาจรับ shell environment คนละชุด

ถ้า Service ไม่ใช่ mock ให้หยุดทดสอบจนแก้ config/environment อย่างถูกต้อง ไม่ส่ง real shutdown เพื่อทดลอง lifecycle

### 26.10 Crash recovery ไม่ตรงลำดับ

ตรวจ qfailure, qfailureflag, จำนวน crash ก่อนหน้า, เวลาที่ผ่าน reset window และ startup error ของ process ที่ SCM เปิดใหม่ Recovery delay ไม่เท่ากับเวลา runtime/network พร้อมทั้งหมด อย่า reset policy หรือ delete identity เพื่อทำให้ตัวเลขตรง

### 26.11 State already in use / Remove ยังไม่หาย

Runtime/provisioner ที่ใช้ persistent state เดียวกันมี lock ป้องกัน concurrent writers หยุด instance เดิมอย่างถูกต้องก่อน repair ไม่ลบ lock ขณะที่ process ยังทำงาน ไฟล์ lock ที่เหลือหลัง crash ไม่ได้แปลว่ามี process ค้างเสมอไป

หาก Remove marked for deletion ให้ปิด Services UI/handle ของ service แล้ว query ซ้ำ ห้ามลบ directories เพื่อแก้ service registration

## 27. ที่มาของคำสั่งและข้อจำกัดของคู่มือ

อ้างอิง working tree ที่ตรวจขณะเขียนเอกสาร:

| ไฟล์/function | สิ่งที่ใช้กำหนดขั้นตอน |
| --- | --- |
| [main.go](../main.go) | --service-info, --service, --provision, --migrate-identity, --configure-service |
| [internal/agent/runtime.go](../internal/agent/runtime.go) | runtime ร่วม, persistent paths, readiness, logging, Service ไม่มี stdin enrollment |
| [internal/servicehost/host_windows.go](../internal/servicehost/host_windows.go) | SCM controls และ lifecycle/timeouts |
| [internal/servicehost/configure_windows.go](../internal/servicehost/configure_windows.go) | LocalSystem, Automatic, Service DACL และ recovery 5s/30s/NoAction |
| [internal/apppaths/paths.go](../internal/apppaths/paths.go) | ชื่อและตำแหน่งไฟล์ |
| [internal/apppaths/paths_windows.go](../internal/apppaths/paths_windows.go) | Windows Known Folder paths |
| [service/agent_id.go](../service/agent_id.go) | LoadIdentity, MigrateIdentity และไม่ regenerate corrupt identity |
| [service/registration.go](../service/registration.go) | EnsureEnrollment, status handling และ interactive prompt |
| [service/enrollment_state.go](../service/enrollment_state.go) | marker ผูก ID/public-key fingerprint/API |
| [scripts/dev-service.ps1](../scripts/dev-service.ps1) | Action, Executable, ConfigPath, IdentityPath, NewIdentity และ retained-data Remove |
| [windows-service.md](windows-service.md) | architecture และข้อจำกัด |
| [windows-service-acceptance.md](windows-service-acceptance.md) | access probe และ manual acceptance เดิม |
| [windows-service-verification.md](windows-service-verification.md) | ผลเครื่องพัฒนาก่อนหน้า ซึ่งไม่ใช่ผลเครื่องใหม่ |

ไม่มีการเปลี่ยน CLI/protocol/implementation เพื่อให้คำสั่งในคู่มือนี้ใช้ได้ ตัวอย่าง C: ใช้ประกอบความเข้าใจเท่านั้น; metadata จริงเป็นแหล่งอ้างอิงบนเครื่องทดสอบ

สิ่งที่ยังขึ้นกับ environment ได้แก่สิทธิ์ Admin/Standard User, NTFS/policy, C compiler สำหรับ race, backend availability และ network ก่อน login การพิสูจน์ Online ก่อน login ต้องมี observation จาก B จริง ส่วนเครื่องเดียวทำ local lifecycle ได้หลัง provisioning สำเร็จแล้ว

Screen ใน Session 0, DPAPI cross-account decryption และ production security/installer ไม่ได้อยู่ใน acceptance นี้ ไม่ใช้ผล Service tests ไปอ้างว่าตรวจคุณสมบัติเหล่านั้นแล้ว
