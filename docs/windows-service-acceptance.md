<a id="remaining-windows-acceptance-procedures"></a>

# ขั้นตอนทดสอบยอมรับบน Windows ที่ยังเหลืออยู่

สถานะ ณ 2026-09-21: **ยังไม่ได้ตรวจยืนยันด้วยตนเอง (NOT YET VERIFIED MANUALLY)**
ให้ทำขั้นตอนเหล่านี้บนเครื่องพัฒนา/VM ที่กำหนด เซสชันที่พัฒนาไม่ได้ติดตั้ง Service
รีบูต สลับบัญชี หรือยุติโปรเซส ให้เก็บ snapshot ของ VM ก่อนทดสอบที่จงใจลองปิดใช้งาน/ลบ/ยุติ Service
เพราะ ACL ที่ผิดอาจทำให้การกระทำที่ควรถูกปฏิเสธสำเร็จได้

<a id="1-prepare-and-install-as-administrator"></a>

## 1. เตรียมและติดตั้งด้วยสิทธิ์ Administrator

ใช้ Windows PowerShell 5.1 แบบ Run as Administrator ตรวจ Get-ExecutionPolicy -List:
เครื่องพัฒนาขณะนั้นบล็อกการเรียก .ps1 ก่อนสคริปต์เริ่มทำงาน หากจำเป็น ให้ผู้ดูแลอนุญาตสคริปต์
ที่ตรวจทานแล้วผ่านนโยบาย/กระบวนการลงลายเซ็นที่องค์กรรับรอง
เครื่องมือเตรียมติดตั้งและคำแนะนำนี้ไม่เปลี่ยน execution policy

Build ซอร์สที่ตรวจทานแล้ว แล้วเลือกข้อมูลประจำตัวให้ตรงกับการติดตั้งนี้
ห้ามใช้ข้อมูลประจำตัวของเครื่องอื่นหรือลบข้อมูลประจำตัวเพื่อแก้ปัญหาลงทะเบียน
ก่อนย้าย ให้สำรองข้อมูลประจำตัว/การตั้งค่าเดิมนอกตำแหน่งติดตั้งโดยให้เฉพาะ Administrator เข้าถึงได้
ห้ามแสดงฟิลด์กุญแจส่วนตัวบนเทอร์มินัล

สร้าง .env ที่ตรวจทานแล้วนอกระบบควบคุมเวอร์ชัน ระบุปลายทางที่ถูกต้องและ POWER_MODE=mock
ให้ใส่ POWER_MODE ในไฟล์ถาวรนี้ ไม่ใช่เพียงสภาพแวดล้อม shell ของผู้ดูแล เพราะ SCM ใช้สภาพแวดล้อมต่างกัน

จาก repository ของ Agent หลังเปลี่ยนพาธต้นทางตัวอย่างทั้งสองเป็นพาธจริง:

    .\scripts\dev-service.ps1 -Action Install -ConfigPath 'D:\PrivateConfig\service.env' -IdentityPath 'D:\ExistingAgent\agent_config.json'
    .\scripts\dev-service.ps1 -Action Status
    sc.exe qc ThesisAgentDev
    sc.exe queryex ThesisAgentDev
    sc.exe qfailure ThesisAgentDev
    sc.exe qfailureflag ThesisAgentDev
    sc.exe sdshow ThesisAgentDev

เฉพาะการติดตั้งใหม่จริงเท่านั้นที่เปลี่ยน -IdentityPath เป็น -NewIdentity
การลงทะเบียนอาจถามโทเคนลงทะเบียนเดิมเมื่อ REST คืน 404
ห้ามใส่โทเคนในอาร์กิวเมนต์ .env บันทึกการทดสอบ หรือล็อก

คาดหวัง Automatic, LocalSystem, พาธ executable ที่ติดตั้งแล้วในเครื่องหมายคำพูดอย่างชัดเจน และ Running
จากนั้นตรวจการลงทะเบียนและการเชื่อมต่อ WebSocket ในล็อก Agent
Running อย่างเดียวไม่พิสูจน์ว่า Online ค่ากู้คืนที่คาดหวังคือหน่วง 5 วินาที 30 วินาที
แล้วไม่ดำเนินการ และปิดการกู้คืนข้อผิดพลาดที่ไม่ใช่โปรเซสล่ม
หากการติดตั้งเดิมปรับมาจากค่ากู้คืนอื่น Microsoft ระบุว่าการเปลี่ยนแฟล็ก failure-action
มีผลเมื่อเริ่มระบบครั้งถัดไป ให้ยืนยันหลังรีบูตตามแผน
([แฟล็กการดำเนินการเมื่อผิดพลาด](https://learn.microsoft.com/en-us/windows/win32/api/winsvc/ns-winsvc-service_failure_actions_flag))

<a id="2-record-paths-permissions-and-identity-baseline"></a>

## 2. บันทึกค่าฐานของพาธ สิทธิ์ และข้อมูลประจำตัว

ในหน้าต่าง PowerShell แบบ elevated เดิม:

    $meta = .\build\thesis-agent.exe --service-info | ConvertFrom-Json
    icacls.exe $meta.paths.install
    icacls.exe $meta.executable
    icacls.exe $meta.paths.root
    icacls.exe $meta.paths.identity
    icacls.exe $meta.paths.config
    icacls.exe $meta.paths.enrollment
    Get-Content -LiteralPath $meta.paths.log -Tail 60

คาดหวัง SYSTEM และ Administrators มี FullControl กลุ่ม Builtin Users อ่าน/เรียกทำงานในต้นไม้โปรแกรมได้
แต่แก้ไขไม่ได้ ส่วนต้นไม้รันไทม์จำกัดเฉพาะ SYSTEM/Administrators
ตรวจเจ้าของและการป้องกันการสืบทอดด้วย ไม่ใช่ดูเฉพาะ ACE
การตรวจนี้ไม่ทดแทนการทดสอบภายใต้บัญชี Standard User จริง

บันทึกเฉพาะ ID และ hash ห้ามบันทึก JSON ข้อมูลประจำตัวทั้งไฟล์:

    $baseline = [pscustomobject]@{
        AgentID = (Get-Content -LiteralPath $meta.paths.identity -Raw | ConvertFrom-Json).agent_id
        IdentitySHA256 = (Get-FileHash -LiteralPath $meta.paths.identity -Algorithm SHA256).Hash
        ConfigSHA256 = (Get-FileHash -LiteralPath $meta.paths.config -Algorithm SHA256).Hash
    }
    $baseline | ConvertTo-Json | Set-Content -LiteralPath (Join-Path $meta.paths.root 'acceptance-baseline.json')

สำหรับการย้าย ให้เปรียบเทียบ Get-FileHash ของไฟล์ข้อมูลประจำตัวต้นทางที่ระบุ
กับไฟล์ที่ติดตั้ง การย้ายครั้งแรกไปยังปลายทางว่างต้องเก็บทุกไบต์ไว้
หากปลายทางมีข้อมูลประจำตัวตรงกันอยู่แล้ว ให้ถือไฟล์นั้นเป็นข้อมูลหลัก

<a id="3-administrator-control-and-graceful-stop"></a>

## 3. การควบคุมของผู้ดูแลและการหยุดตามขั้นตอน

    .\scripts\dev-service.ps1 -Action Stop
    Start-Sleep -Seconds 40
    Get-Service -Name ThesisAgentDev
    .\scripts\dev-service.ps1 -Action Start
    .\scripts\dev-service.ps1 -Action Restart
    Get-Content -LiteralPath $meta.paths.log -Tail 100

คาดหวังว่า Stop แล้วยังคง Stopped หลัง 40 วินาที Start/Restart ทำงานได้
และมีล็อกหยุด/คืนทรัพยากรตามปกติ ทดลองยกเลิกขณะดาวน์โหลดหรือสแกนที่ควบคุมได้กำลังทำงาน
ใช้คำสั่งพลังงานแบบ mock เท่านั้น ห้ามสรุปว่าปิด Windows สำเร็จหรือยกเลิกบริการ Defender จริงได้
จากการทดสอบ mock เพียงอย่างเดียว

<a id="4-standard-user-serviceprocess-protection"></a>

## 4. การป้องกัน Service/โปรเซสจาก Standard User

สลับบัญชีด้วยตนเองเมื่อพร้อม ใช้ Standard User จริง ไม่ใช่เพียง Administrator ที่ยังไม่ได้ยกระดับสิทธิ์
ห้ามยกระดับสิทธิ์ในการทดสอบเหล่านี้:

    sc.exe queryex ThesisAgentDev
    Stop-Service -Name ThesisAgentDev -ErrorAction Stop
    sc.exe stop ThesisAgentDev
    sc.exe config ThesisAgentDev start= disabled
    sc.exe delete ThesisAgentDev

ลองทีละคำสั่งและบันทึกผล การแก้ไขต้องได้ Access denied และ Service ต้องยังทำงานต่อ
ในหน้าจอ Services -> ThesisAgentDev -> Stop ต้องเลือกไม่ได้หรือถูกปฏิเสธ
ใน Task Manager -> Details ให้ระบุโปรเซส Service นี้ให้แน่นอน ลอง End task แล้วต้องได้ Access denied

สำหรับการทดสอบโปรเซสผ่าน PowerShell ให้อ่าน PID จากข้อมูล Service
ยืนยันว่ายังเป็น Service เดิม และใช้ค่านั้น ห้ามเดา PID:

    $agentService = Get-CimInstance Win32_Service -Filter "Name='ThesisAgentDev'"
    if ($null -eq $agentService -or $agentService.ProcessId -eq 0) { throw 'Service is not running' }
    Stop-Process -Id $agentService.ProcessId -Force -ErrorAction Stop

คาดหวัง Access denied หากการทดสอบใดสำเร็จโดยไม่คาดหวัง ให้หยุดทดสอบยอมรับและเก็บหลักฐานความล้มเหลว
ห้ามระบุว่าการตรวจการป้องกันอื่น ๆ ผ่าน

<a id="5-standard-user-file-protection-without-changing-identity"></a>

## 5. การป้องกันไฟล์จาก Standard User โดยไม่เปลี่ยนข้อมูลประจำตัว

เพื่อไม่ทำลายข้อมูลประจำตัวหากสิทธิ์ผิด ให้ขอสิทธิ์เขียน/ลบโดยไม่เขียน ตัดเนื้อหา เปลี่ยนชื่อ หรือลบจริง
ผู้ดูแลควรหยุด Service ก่อนและยืนยันว่าไฟล์เป้าหมายทั้งสี่มีอยู่
เพื่อไม่ตีความ sharing violation ผิดว่าเป็นการปฏิเสธจาก ACL แล้วจึงใช้เซสชัน Standard User

คำสั่งด้านล่างใช้ OPEN_EXISTING โดยไม่มีแฟล็กลบเมื่อปิด handle ให้ปิด handle ทันทีทุกครั้ง
เป็นการตรวจสิทธิ์ที่ได้รับ ไม่ใช่ทดสอบขั้นตอนลบจริง
([CreateFileW](https://learn.microsoft.com/en-us/windows/win32/api/fileapi/nf-fileapi-createfilew))

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
    $programs = Join-Path ([Environment]::GetFolderPath('ProgramFiles')) 'ThesisAgentDev'
    $runtime = Join-Path ([Environment]::GetFolderPath('CommonApplicationData')) 'ThesisAgentDev'
    $targets = @(
        (Join-Path $programs 'thesis-agent.exe'),
        (Join-Path $runtime 'agent_config.json'),
        (Join-Path $runtime '.env'),
        (Join-Path $runtime 'enrollment_state.json')
    )
    foreach ($target in $targets) {
        foreach ($access in @([uint32]0x40000000, [uint32]0x10000)) {
            $handle = [AgentFileAccessProbe]::CreateFileW($target, $access, 7, [IntPtr]::Zero, 3, 128, [IntPtr]::Zero)
            $win32Error = [Runtime.InteropServices.Marshal]::GetLastWin32Error()
            $wasDenied = $handle.IsInvalid -and $win32Error -eq 5
            $handle.Dispose()
            if (-not $wasDenied) { throw "FAIL or inconclusive: path=$target access=$access error=$win32Error" }
            "PASS access denied: $target access=$access"
        }
    }

คาดหวังข้อผิดพลาด 5 (Access denied) สำหรับทั้ง GENERIC_WRITE และ DELETE
ข้อผิดพลาด 2 (ไม่มีไฟล์) หรือ 32 (sharing violation) ยังสรุปไม่ได้ ไม่ถือว่าผ่าน

สำหรับทดสอบแก้/แทนที่/เปลี่ยนชื่อ/ลบจริงผ่าน Explorer ให้ใช้ snapshot ของ VM ที่ใช้ทิ้งได้
และข้อมูลสำรองที่เฉพาะ Administrator เข้าถึงได้ ลองแต่ละการกระทำกับ binary ที่ติดตั้ง
ข้อมูลประจำตัว .env และสถานะลงทะเบียนด้วย Standard User ทุกกรณีต้องถูกปฏิเสธ
หากกรณีใดสำเร็จ ให้หยุดแล้วคืน snapshot/ข้อมูลสำรอง
ห้ามเตรียมข้อมูลประจำตัวใหม่เพื่อกลบความล้มเหลว

จากนั้นให้ผู้ดูแลเริ่ม Service อีกครั้งและตรวจ hash ของข้อมูลประจำตัว

<a id="6-reboot-auto-start-and-identity-stability"></a>

## 6. การรีบูต การเริ่มอัตโนมัติ และความคงเดิมของข้อมูลประจำตัว

เลือกเวลารีบูตด้วยตนเอง ปล่อย Windows ค้างที่หน้าเข้าสู่ระบบนานพอ
ให้สังเกตจากเครื่องผู้ดูแลอีกเครื่องว่า Agent ที่ลงทะเบียนไว้เปลี่ยนเป็น Online
บันทึกเวลาจากฝั่งนั้น แล้วเข้าสู่ระบบเพื่อเทียบกับล็อก Service และเวลาบูต:

    (Get-CimInstance Win32_OperatingSystem).LastBootUpTime
    Get-Service -Name ThesisAgentDev
    Get-Content -LiteralPath $meta.paths.log -Tail 100
    $baseline = Get-Content -LiteralPath (Join-Path $meta.paths.root 'acceptance-baseline.json') -Raw | ConvertFrom-Json
    $currentID = (Get-Content -LiteralPath $meta.paths.identity -Raw | ConvertFrom-Json).agent_id
    if ($currentID -ne $baseline.AgentID) { throw 'Agent ID changed' }
    if ((Get-FileHash -LiteralPath $meta.paths.identity -Algorithm SHA256).Hash -ne $baseline.IdentitySHA256) { throw 'Identity bytes changed' }
    if ((Get-FileHash -LiteralPath $meta.paths.config -Algorithm SHA256).Hash -ne $baseline.ConfigSHA256) { throw 'Config changed' }

หากเปิดเทอร์มินัลใหม่ ให้ตั้ง $meta อีกครั้งด้วย --service-info ของ executable ที่ติดตั้งแล้วตามจำเป็น
เวลาล็อก Agent เป็น UTC การเห็น Service เป็น Running หลังล็อกอิน
ไม่เพียงพอที่จะพิสูจน์ว่าเริ่มทำงานก่อนล็อกอิน

<a id="7-infrastructure-outage-and-reconnect"></a>

## 7. ระบบภายนอกขัดข้องและการเชื่อมต่อใหม่

บนระบบทดสอบเฉพาะ ให้ทำให้ REST ใช้งานไม่ได้ แล้วรีบูตเครื่อง Agent ที่เตรียมไว้ด้วยตนเอง
คาดหวังสถานะ Running ในเครื่องพร้อมล็อกลองใหม่ ไม่มีการถามโทเคน และข้อมูลประจำตัวไม่เปลี่ยน
เมื่อคืน REST แล้ว /exists ต้องคืน 204 และ Agent เชื่อมต่อ WS เอง
หากจงใจให้คืน 404 ต้องให้ผู้ดูแลเตรียมติดตั้งด้วยข้อมูลประจำตัวเดิม

เริ่มเซิร์ฟเวอร์ WebSocket ทดสอบใหม่ขณะเชื่อมต่อ คาดหวังเวลาลองใหม่ที่สุ่มและมีขอบเขต
แล้วเชื่อมต่อด้วย Agent ID เดิม หาก TCP/WS handshake ล้มเหลวโดยไม่มีการตอบกลับ
เวลารอเชื่อมต่อ/heartbeat หมดอายุจะมีผลต่อเวลารวมที่ใช้กู้คืนด้วย

สำหรับหลายเครื่อง ให้เตรียมแต่ละเครื่องด้วยข้อมูลประจำตัวของตนเองและ URL/พอร์ต WS เดียวกัน
เริ่มพร้อมกันและตรวจว่ามีรายการ Agent แยกกันและเชื่อมต่อใหม่อย่างอิสระ
การโคลนข้อมูลประจำตัวเดียวกันเป็นการทดสอบแทนที่เซสชันซ้ำ ไม่ใช่ทดสอบรองรับหลาย Agent

<a id="8-controlled-crash-recovery"></a>

## 8. การกู้คืนจากการล่มแบบควบคุม

ทำเฉพาะเครื่องพัฒนาที่กำหนดและใช้ทิ้งได้ ด้วยสิทธิ์ Administrator หลังเตรียมติดตั้งสำเร็จ
ยืนยันโปรเซส Service ให้ตรงก่อนยุติ:

    $agentService = Get-CimInstance Win32_Service -Filter "Name='ThesisAgentDev'"
    if ($null -eq $agentService -or $agentService.ProcessId -eq 0) { throw 'Service is not running' }
    $agentProcess = Get-Process -Id $agentService.ProcessId -ErrorAction Stop
    if ($agentProcess.Path -ine $meta.executable) { throw 'PID does not match installed Agent' }
    Stop-Process -Id $agentProcess.Id -Force -ErrorAction Stop
    Start-Sleep -Seconds 8
    sc.exe queryex ThesisAgentDev

คาดหวัง PID ใหม่หลังเวลาหน่วงครั้งแรก 5 วินาทีที่ตั้งไว้
เมื่อล่มครั้งที่สองในช่วงนับความล้มเหลวเดียวกัน ให้รออย่างน้อย 35 วินาทีสำหรับการเริ่มใหม่หลัง 30 วินาที
ครั้งที่สามควรคงสถานะหยุด สอบถามและยืนยัน PID/พาธใหม่ก่อนทุกครั้ง ห้ามใช้ PID เก่าซ้ำ
ตัวนับรีเซ็ตเมื่อไม่มีความล้มเหลวครบ 24 ชั่วโมง จึงต้องบันทึกการล่มก่อนหน้าประกอบการแปลผลลำดับด้วย

หลังตรวจสอบสถานะหยุดสุดท้ายแล้ว ให้ผู้ดูแล Start เอง
ทดสอบ Stop โดยตั้งใจแยกอีกครั้ง โดยต้องคงสถานะหยุด

<a id="9-console-regression-and-removal"></a>

## 9. ตรวจผลกระทบต่อ Console และถอนทะเบียน

ใช้การติดตั้ง Console แยกพร้อมข้อมูลประจำตัว/การตั้งค่าเดิมที่ถูกต้อง
รัน go run . ตรวจล็อกเทอร์มินัลและไฟล์ Ctrl+C และขั้นตอนเดิมของข้อมูลประสิทธิภาพ/โปรเซส/
ดาวน์โหลด/สแกน/พลังงานแบบ mock ตรวจการส่งภาพ JPEG ใน Console
สำหรับ Service ที่มีผู้ใช้ล็อกอินและ Desktop Helper ให้ตรวจการส่ง JPEG ผ่าน Named Pipe
ต่อไปยัง WebSocket ที่ Service ดูแล หากไม่มี Helper คำขอต้องไม่ทำให้ Service หยุด
และต้องกลับมาทำงานได้เมื่อ Helper กลับมา

เมื่อใช้งาน Service สำหรับพัฒนานี้เสร็จ:

    .\scripts\dev-service.ps1 -Action Remove
    sc.exe query ThesisAgentDev

หลังถอนทะเบียนคาดหวัง Service-not-installed (1060) โดยยังเก็บข้อมูลประจำตัว/การตั้งค่า/ล็อก
และแหล่ง Event Log ไว้ Remove ไม่มีการลบข้อมูลแบบเวียนซ้ำ
บันทึกผลจริงของแต่ละสถานการณ์ เอกสารนี้เป็นขั้นตอน ไม่ใช่หลักฐานว่าสถานการณ์ใดผ่านแล้ว

<a id="phase-5b1-acl-acceptance-on-one-lab-pc"></a>

## การทดสอบยอมรับ ACL ของ Phase 5B.1 บนเครื่อง Lab หนึ่งเครื่อง

สถานะ: **ยังไม่ได้ยืนยัน (NOT VERIFIED)** ทำเฉพาะเครื่อง Lab/VM ที่กำหนดและใช้ทิ้งได้
หลังอ่าน [ความปลอดภัย ACL ของรันไทม์](runtime-acl-security.md)
ห้ามจงใจเปิดกุญแจส่วนตัวจริงของเครื่องให้ Users/Everyone อ่านเพื่อทดสอบ
ใช้ข้อมูลประจำตัวสำหรับทดสอบที่ใช้ทิ้งได้ และเก็บ snapshot/ข้อมูลสำรองที่เฉพาะ Administrator เข้าถึงได้
ก่อนเปลี่ยน ACL

1. บนเครื่อง Lab หากมี Go ให้รัน `go test -v ./internal/protectedpath` จากเทอร์มินัลทดสอบแบบ elevated
   ยืนยันว่า `TestTrustedStartupScopePreflightAndRepair` และ
   `TestRepairedFixtureReachesAuthoritativePrivateKeyLoader` ทำงานจริงโดยไม่ SKIP
   ทั้งคู่ใช้เฉพาะข้อมูลทดสอบชั่วคราว เซสชันที่พัฒนายังไม่ได้ยืนยันชุดทดสอบร่วมสองชุดนี้ที่ต้องใช้สิทธิ์ elevated
2. ติดตั้ง `build/thesis-agent.exe` และ `build/thesis-agent-desktop.exe` ที่ตรวจทานแล้วผ่านขั้นตอนอัปเดตเดิม
   อ่านข้อมูลกำกับด้วย `--service-info` บันทึก SHA-256 ของข้อมูลประจำตัว การตั้งค่า และสถานะลงทะเบียน
   โดยไม่พิมพ์เนื้อหา พร้อม Agent ID/ลายนิ้วมือกุญแจสาธารณะ ตรวจ startup ปกติและ `acl_canonical`
3. หยุด Service ของระบบนี้ บนการติดตั้งที่ใช้ทิ้งได้ให้เปลี่ยน DACL ที่มีเฉพาะผู้รับสิทธิ์ที่เชื่อถือได้
   เช่น ลบ ACE ของ SYSTEM แต่เก็บ Administrators Full Control และเจ้าของที่เชื่อถือได้ไว้
   เริ่ม Service แล้วคาดหวังความคลาดเคลื่อน → repair_started → repair_success
   ตามด้วยการยืนยันตัวตนปกติและ hash ข้อมูลประจำตัว/การตั้งค่าที่ไม่เปลี่ยน
   เทียบไบต์สถานะลงทะเบียนก่อนและหลัง Repair เฉพาะ ACL ทันที
   เพราะการตรวจลงทะเบียนระหว่างรันตามปกติอาจปรับไฟล์สถานะตามพฤติกรรมเดิม
4. เริ่มใหม่อีกครั้ง ต้องตรวจผ่านตามมาตรฐานโดยไม่ซ่อม
   ยืนยันว่า Standard User อ่านไฟล์รันไทม์สำคัญหรือแก้ต้นไม้ที่ป้องกันไว้ไม่ได้
5. ขณะ Service หยุด รัน `scripts/dev-service.ps1 -Action Repair` ด้วย binary ที่ตรวจทานแล้ว
   ต้องคง hash และสถานะหยุด หาก Service เป็น Running คำสั่งเดียวกันต้องปฏิเสธโดยไม่หยุด Service
6. คืน snapshot ระหว่างสถานการณ์ไม่ปลอดภัย ขณะ Service หยุด ให้ทดสอบเจ้าของที่ไม่น่าเชื่อถือ
   ACE ที่ให้ Users อ่าน, reparse/junction, พาธไฟล์สำคัญที่กลายเป็นไดเรกทอรี และ ACL ที่อ่านไม่ได้/ไม่รองรับ
   ทั้ง startup และ Repair ต้องปฏิเสธ ระบุเหตุผลตรงกรณี ไม่รีเซ็ต ACL ไม่เปลี่ยนเนื้อหา
   และคำสั่งบำรุงรักษาต้องคืน exit status ที่ไม่ใช่ศูนย์
   หยุดเพื่อให้ผู้ดูแลตรวจสอบความปลอดภัย ห้ามลงทะเบียน/สร้างข้อมูลใหม่เพื่อกลบความล้มเหลว
   ยืนยันว่าการทดสอบ reparse ไม่เปลี่ยนพาธอื่นที่ไม่เกี่ยวข้อง
7. ตรวจ Application events จาก `ThesisAgentDev` เมื่อถูกปฏิเสธก่อนเริ่มทำงาน
   รวมถึงกรณีเริ่มระบบล็อกไฟล์ไม่ได้ หลัง startup สำเร็จอย่างปลอดภัย ให้ตรวจการเขียนผลซ่อมที่พักไว้ลง agent.log
   แม้ส่ง Event Log ไม่ได้ ระบบยังต้องปฏิเสธตามเดิม
8. ขณะ Desktop Helper ทำงานในเซสชันผู้ใช้ ให้ตรวจภาพหน้าจอ/สตรีม JPEG, WebSocket ที่ยืนยันตัวตน,
   heartbeat/ข้อมูลประสิทธิภาพ/โปรเซส/ดาวน์โหลด/พลังงานแบบ mock และการลงทะเบียน task ตามขั้นตอนอัปเดตเดิม
   รีบูตเฉพาะเครื่อง Lab เมื่อผู้ดูแลเครื่องอนุมัติ ตรวจ Service เริ่มอัตโนมัติและข้อมูลประจำตัวคงเดิม

บันทึกผลจริงแยกต่างหาก Build/unit test ที่ผ่านไม่ยืนยันพฤติกรรม LocalSystem
การแยกสิทธิ์ผู้ใช้ที่มีผลจริง การส่ง Event Viewer หรือความเข้ากันได้ของ Desktop Capture/การอัปเดตจริง

<a id="bounded-startup-refinement-acceptance"></a>

### การทดสอบยอมรับการปรับขอบเขต startup ให้จำกัด

สถานะ: **ยังไม่ได้ยืนยันบนเครื่อง Lab (NOT VERIFIED on a Lab PC)**
การทดสอบดาวน์โหลดจำนวนมากแบบอัตโนมัติโดยไม่ยกระดับสิทธิ์ใช้ข้อมูลกำกับจำลอง
(รายการลูก 20,000/50,000 รายการ) ไม่ใช่ไฟล์จริงหลายพันไฟล์บนดิสก์

บนการติดตั้ง Lab ที่ใช้ทิ้งได้ ให้เพิ่มไฟล์ดาวน์โหลดทดสอบมากกว่า 8,192 ไฟล์
พร้อม `.part` เก่า/ล็อกที่หมุนเก็บแล้วในไดเรกทอรีซ้อนกัน
คง ACL ของราก ไฟล์สำคัญ ไฟล์สำรองที่รู้จัก และไดเรกทอรี `logs`/`data`/`data/downloads` ให้ตรงมาตรฐาน
startup และ `-Action Repair` ต้องสำเร็จโดยไม่ไล่รายชื่อหรือเปลี่ยนรายการลูกเหล่านี้
เทียบไบต์/ACL ของตัวอย่างไฟล์เก่าก่อนและหลัง Repair
ไฟล์สำรองสำคัญหรือไดเรกทอรีขอบเขตที่ไม่ปลอดภัยยังต้องถูกปฏิเสธ
รวมถึง reparse ที่ตัวไดเรกทอรี downloads

ทดสอบจุดใช้งานจริงแยกต่างหาก: ทำให้ล็อกปัจจุบันหรือปลายทางดาวน์โหลดทดสอบเป็น
reparse/hard link/ACL ที่ไม่ปลอดภัย การเปิดล็อกหรืองานดาวน์โหลดนั้นต้องปฏิเสธโดยไม่เขียนผ่านวัตถุ
ตำแหน่งล็อกสำรองที่ไม่ปลอดภัยต้องถูกปฏิเสธเมื่อการหมุนล็อกแตะตำแหน่งนั้นจริง
พร้อมข้อความวินิจฉัยใน Event Log โดยไม่เกิดการเขียนล็อกวนซ้ำหรือ deadlock
คืนข้อมูลทดสอบแล้วตรวจการเขียนต่อท้าย/หมุนล็อก และดาวน์โหลด/ตรวจ checksum/เผยแพร่ไฟล์
ตรวจการเขียนสถานะ hash ข้อมูลประจำตัว/การตั้งค่า Desktop JPEG และขั้นตอนติดตั้ง/อัปเดตเดิม
รันชุดทดสอบ I/O ที่ป้องกันไว้ด้วยสิทธิ์ elevated ด้วย เนื่องจากถูกข้ามในเซสชันที่พัฒนา
ห้ามเปิดเผยกุญแจจริงระหว่างทดสอบ ACL ที่ไม่ปลอดภัย
