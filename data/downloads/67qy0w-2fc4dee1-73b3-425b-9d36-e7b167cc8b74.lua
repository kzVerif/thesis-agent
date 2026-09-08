-- ═══ HUB STRIP POINT — when deployed to Codeberg the ScriptLoader injects
--     "local Library = _G.OxideLib" above this line instead. ═══
-- ══════════════════════════════════════════════════════════════════════════════

-- ══════════════════════════════════════════════════════════════════════════════
-- RE-EXECUTION GUARD
-- ══════════════════════════════════════════════════════════════════════════════
do
    local prev = _G.OxideStealAnEgg
    if prev and type(prev.Unload) == "function" then pcall(prev.Unload) end
end
local HUB = { conns = {}, dead = false, loops = {} }
_G.OxideStealAnEgg = HUB
local function track(conn) table.insert(HUB.conns, conn); return conn end

local Window = Library:CreateWindow({
    Name = "Oxide HUB | Ein Ei stehlen",
    LoadingAnimation = true,
    LoadingText = "Oxide",
    LoadingDuration = 2.2,
})

-- ══════════════════════════════════════════════════════════════════════════════
-- CONFIG / FLAG PERSISTENCE
-- ══════════════════════════════════════════════════════════════════════════════
local HAS_CONFIG = type(Library.SaveConfig) == "function"
    and type(Library.LoadConfig) == "function"
    and type(Library.ListConfigs) == "function"
local CONFIG_NAME = "stealegg"

local dropdownResync = {}
local function registerResync(handle, applyFn)
    if handle and applyFn then
        table.insert(dropdownResync, function() applyFn(handle:Get()) end)
    end
end
local function ResyncAll()
    for _, fn in ipairs(dropdownResync) do pcall(fn) end
end

-- ══════════════════════════════════════════════════════════════════════════════
-- SERVICES / ENV
-- ══════════════════════════════════════════════════════════════════════════════
local Players = game:GetService("Players")
local ReplicatedStorage = game:GetService("ReplicatedStorage")
local Workspace = game:GetService("Workspace")
local TeleportService = game:GetService("TeleportService")
local TweenService = game:GetService("TweenService")
local RunService = game:GetService("RunService")
local LocalPlayer = Players.LocalPlayer

-- ══════════════════════════════════════════════════════════════════════════════


-- TP WALK METHOD — instant Humanoid swap strips ALL anti-cheat hooks, then
-- per-frame dt-scaled CFrame nudge: hrp.CFrame += dir * speed * dt. Zero
-- WalkSpeed, zero TweenService, zero Lerp. AssemblyLinearVelocity zeroed each
-- frame so the server sees no velocity spikes. The __namecall block silences
-- all guard/character-reset remotes.
-- ══════════════════════════════════════════════════════════════════════════════


track(LocalPlayer.CharacterAdded:Connect(function(chr)
    if fakeDeathActive then SetFakeDeath(true) end
    if not safeZonePos then
        local r = chr:FindFirstChild("HumanoidRootPart")
        if r then safeZonePos = r.Position end
    end
end))

-- MoveTo-based walk: uses the game's pathfinding (respects walls/navmesh).
-- Max WalkSpeed on the bare Humanoid (no anti-cheat limiting it).
local function FastWalk(targetPos, timeout)
    local chr = LocalPlayer.Character
    if not chr then return false end
    local hrp = chr:FindFirstChild("HumanoidRootPart")
    local hum = chr:FindFirstChildOfClass("Humanoid")
    if not hrp or not hum then return false end

    -- Bare Humanoid: set max speed, let MoveTo do the pathfinding
    hum.WalkSpeed = math.clamp(stealSpeed or 300, 50, 300)
    local t0 = os.clock()
    local maxTime = timeout or 60
    local lastPos = hrp.Position
    local stuckCount = 0

    hum:MoveTo(targetPos)

    while not HUB.dead do
        if not (chr.Parent and hrp.Parent) then break end

        local dx = targetPos.X - hrp.Position.X
        local dz = targetPos.Z - hrp.Position.Z
        local dist = math.sqrt(dx*dx + dz*dz)

        if dist <= 4 then hum.WalkSpeed = 16; return true end
        if os.clock() - t0 > maxTime then hum.WalkSpeed = 16; return false end

        -- Stuck recovery (jump)
        if (hrp.Position - lastPos).Magnitude < 1.5 then
            stuckCount = stuckCount + 1
            if stuckCount >= 6 then
                pcall(function() hum.Jump = true end)
                stuckCount = 0
            end
        else
            stuckCount = 0
        end
        lastPos = hrp.Position
        task.wait(0.08)
    end
    hum.WalkSpeed = 16
    return false
end

-- ══════════════════════════════════════════════════════════════════════════════
-- STATE (before UI so callbacks can bind to them)
-- ══════════════════════════════════════════════════════════════════════════════
local stealEnabled   = false
local infHpActive     = false  -- see SetInfHp() in the movement engine
local fakeDeathActive = false  -- toggle for WalkThroughGate compatibility
local FAST_WALK_SPEED = 160    -- legal max walk speed (game caps ~160-300); the tween legs use this
local carrySpeedMultiplier = 1  -- server applies this to WalkSpeed while carrying an egg (run-back boost)
local carryState = nil           -- latest AreaEggCarryState (authoritative IsCarrying / Uid)
local stealSpeed = 300          -- walk speed (studs/s), driven by the "Walk Speed" slider
local stealCount = 0            -- eggs stolen this session
local safeZonePos = nil          -- home base: the spawn/safe zone (NOT the fenced plot) — captured at load
local rareHunterEnabled = false
local rareMinTier       = 8  -- Secret+ (the game's own "rare" = Secret/Eternal)
local speedBoostEnabled = false
local speedSessionId = nil
local treadmillEnabled = false
local treadmillAutoUpgrade = true
local hatchEnabled   = false
local equipEnabled   = false
local sellEnabled    = false
local eggSellEnabled  = false
local claimEnabled   = false
local claimInterval  = 30
local upgradeEnabled = false
local upgradeInterval = 15
local serverHopEnabled = false
local serverHopMax     = 40  -- stop after this many hops in one session (safety)
local serverHopDelay   = 12  -- seconds to scan before deciding to hop
local serverHopCount   = 0

-- Visual pet spawner (client-side only — real Tools, fake inventory).
-- Supports MULTIPLE pets at once: each spawn adds a new pet with its own Tool,
-- so every pet in the inventory stays clickable/equipable independently.
local visualPets        = {}    -- uid -> { tool, category, slot }
local lastVisualUid     = nil   -- most recently spawned pet (for the Equip button)
local VISUAL_PET_ORDER  = {}
local VISUAL_PET_BY_LABEL = {}
local VISUAL_MUTATIONS  = { "None" }
local visualDataReady   = false
local preloadedPets     = {}   -- category -> true once its model content is fetched

-- Rarity tiers, sorted worst → best (by RarityNumber).
local RARITY_OPTIONS = {
    "Basic", "Common", "Celestial", "SuperRare", "Uncommon", "Rare", "Epic",
    "Legendary", "BrainrotGod", "Mythic", "Mythical", "Rainbow", "Cosmic",
    "Exclusive", "Exotic", "Secret", "Eternal", "Limited", "Divine", "Superior",
}

-- Rare Egg Hunter tier presets (tier = RarityNumber: 4 Epic, 5 Legendary,
-- 6 Mythic-family, 8 Secret+, 9 Eternal/Limited, 10 Divine/Superior/Transcendent).
local RARE_TIER_OPTIONS = { "Secret+ (8)", "Mythic+ (6)", "Legendary+ (5)", "Epic+ (4)" }
local RARE_TIER_VALUES  = {
    ["Secret+ (8)"]    = 8,
    ["Mythic+ (6)"]    = 6,
    ["Legendary+ (5)"] = 5,
    ["Epic+ (4)"]      = 4,
}

local function SetFromList(list)
    local s = {}
    for _, v in ipairs(list or {}) do s[v] = true end
    return s
end

-- Which rarities to steal / sell (empty set = none).
local stealRaritySet = SetFromList(RARITY_OPTIONS)
local sellRaritySet  = SetFromList({ "Basic", "Common", "Uncommon", "SuperRare", "Celestial", "Rare" })
local eggSellRaritySet = SetFromList(RARITY_OPTIONS)  -- sell-all-eggs: all rarities by default

local hatchCount  = 0
local equipCount  = 0
local sellCount   = 0
local eggSellCount = 0
local claimCount  = 0

-- ══════════════════════════════════════════════════════════════════════════════
-- GAME API (pcall-guarded; UI already up, failures degrade gracefully)
-- ══════════════════════════════════════════════════════════════════════════════
local ENV_OK = false
local Network, Save, EggCmds, Assets, Rarities, NETMAP, SPP, TreadmillUtil, TreadmillDir, ResetWall

local function InitGameApi()
    local ok
    ok, Network = pcall(require, ReplicatedStorage:WaitForChild("Library"):WaitForChild("Client"):WaitForChild("Network"))
    if not ok then Network = nil end
    ok, Save = pcall(require, ReplicatedStorage:WaitForChild("Library"):WaitForChild("Client"):WaitForChild("Save"))
    if not ok then Save = nil end
    ok, EggCmds = pcall(require, ReplicatedStorage:WaitForChild("Library"):WaitForChild("Client"):WaitForChild("EggCmds"))
    if not ok then EggCmds = nil end
    ok, Assets = pcall(require, ReplicatedStorage:WaitForChild("Directory"):WaitForChild("Assets"))
    if not ok then Assets = nil end
    ok, Rarities = pcall(function() return require(ReplicatedStorage:WaitForChild("Directory"):WaitForChild("Rarity")).Rarities end)
    if not ok then Rarities = nil end
    ok, SPP = pcall(require, ReplicatedStorage:WaitForChild("Library"):WaitForChild("Client"):WaitForChild("SpeedPowerProjection"))
    if not ok then SPP = nil end
    ok, TreadmillUtil = pcall(require, ReplicatedStorage:WaitForChild("Library"):WaitForChild("Util"):WaitForChild("TreadmillUtil"))
    if not ok then TreadmillUtil = nil end
    ok, TreadmillDir = pcall(require, ReplicatedStorage:WaitForChild("Directory"):WaitForChild("Treadmills"))
    if not ok then TreadmillDir = nil end
    ok, ResetWall = pcall(require, ReplicatedStorage:WaitForChild("Library"):WaitForChild("Client"):WaitForChild("AreaEggResetWall"))
    if not ok then ResetWall = nil end

    if Network and Network.NET_MAP then
        NETMAP = Network.NET_MAP
    end

    ENV_OK = Network ~= nil and Save ~= nil
    return ENV_OK
end

-- ══════════════════════════════════════════════════════════════════════════════
-- HELPERS
-- ══════════════════════════════════════════════════════════════════════════════
local function Notify(title, content, kind, dur)
    pcall(Window.Notify, Window, { Title = title, Content = content, Type = kind or "Info", Duration = dur or 2.5 })
end

local function notifyOn(feature, on)
    Notify(feature, on and "ON" or "OFF", on and "Success" or "Error")
end

local function safeCallback(fn)
    return function(...)
        local ok, err = pcall(fn, ...)
        if not ok then
            pcall(Notify, "Oxide HUB", "Error: " .. tostring(err), "Error", 4)
        end
    end
end

local function safeSpawn(fn)
    task.spawn(function()
        local ok, err = pcall(fn)
        if not ok then
            pcall(Notify, "Oxide HUB", "Engine error: " .. tostring(err), "Error", 4)
        end
    end)
end

local function EnvGate(feature)
    if ENV_OK then return true end
    Notify(feature, "Game API not ready — reload the script", "Error", 4)
    return false
end

local function GetSave()
    if not Save then return nil end
    local ok, s = pcall(Save.Get, LocalPlayer)
    return ok and s or nil
end

local function GetMoney()
    local s = GetSave()
    return s and (tonumber(s.Money) or 0) or 0
end

local function CountTable(t)
    local n = 0
    for _ in pairs(t or {}) do n = n + 1 end
    return n
end

-- Rarity helpers ---------------------------------------------------------------
local function RarityOf(rec)
    if not rec or type(rec.Category) ~= "string" then return nil end
    if not Assets then return nil end
    local dir = Assets.Directory or Assets.Assets or Assets
    if type(dir) ~= "table" then return nil end
    local cfg = dir[rec.Category]
    if type(cfg) ~= "table" then return nil end
    return cfg.Rarity
end

local function RarityNumber(rec)
    local r = RarityOf(rec)
    return (r and tonumber(r.RarityNumber)) or 99
end

local function RarityId(rec)
    local r = RarityOf(rec)
    return (r and r._id) or nil
end

-- Egg records use AssetCategory (not Category).
local function EggRarityId(rec)
    if not rec or type(rec.AssetCategory) ~= "string" then return nil end
    if not Assets then return nil end
    local dir = Assets.Directory or Assets.Assets or Assets
    if type(dir) ~= "table" then return nil end
    local cfg = dir[rec.AssetCategory]
    local r = cfg and cfg.Rarity
    return r and r._id or nil
end

local function EggRarityNumber(rec)
    if not rec or type(rec.AssetCategory) ~= "string" then return 0 end
    if not Assets then return 0 end
    local dir = Assets.Directory or Assets.Assets or Assets
    if type(dir) ~= "table" then return 0 end
    local cfg = dir[rec.AssetCategory]
    local r = cfg and cfg.Rarity
    return (r and tonumber(r.RarityNumber)) or 0
end

-- {rarityId=true} map for the game's native auto-sell config.
local function BuildSellMap()
    return sellRaritySet
end

-- Plot / placement helpers -----------------------------------------------------
local mySlot = nil
local function ResolveMySlot()
    if mySlot then return mySlot end
    if not NETMAP or not NETMAP.Plots then return nil end
    local ok, st = pcall(Network.Invoke, NETMAP.Plots.REQUEST_STATE)
    if not ok or type(st) ~= "table" or type(st.OwnersBySlot) ~= "table" then return nil end
    for slot, uid in pairs(st.OwnersBySlot) do
        if tostring(uid) == tostring(LocalPlayer.UserId) then
            mySlot = tostring(slot)
            return mySlot
        end
    end
    return nil
end

local function FindPlotCenter()
    local slot = ResolveMySlot()
    local plots = Workspace:FindFirstChild("Plots")
    if slot and plots then
        local p = plots:FindFirstChild(slot)
        if p then
            local c = p:FindFirstChild("CenterPoint")
            if c and c:IsA("BasePart") then return c.Position end
            local sign = p:FindFirstChild("PlotSign")
            if sign and sign:IsA("BasePart") then return sign.Position end
        end
    end
    local chr = LocalPlayer.Character
    if chr and chr.PrimaryPart then return chr.PrimaryPart.Position end
    return Vector3.new(0, 0, 0)
end

local function PlotCFrame()
    local pos = FindPlotCenter()
    local off = Vector3.new(math.random(-8, 8), 0, math.random(-8, 8))
    return CFrame.new(pos + off)
end

-- ══════════════════════════════════════════════════════════════════════════════

local function GetLegalWalkSpeed()
    -- The REAL speed cap comes from the server-authoritative SpeedPower in your
    -- save (NOT the client-side SpeedPowerProjection). The anti-cheat probes the
    -- projection and reconciles it against the save, so boosting the projection
    -- only makes the script try 300 stud/s — which then gets reverted, making
    -- you SLOWER. Using the save's real SpeedPower keeps auto-steal at the true
    -- max speed without ever triggering the revert.
    local sp = nil
    if Save then
        local okS, s = pcall(Save.Get, LocalPlayer)
        if okS and s and type(s.SpeedPower) == "number" then sp = s.SpeedPower end
    end
    if sp == nil and SPP then
        local okSp, s2 = pcall(SPP.GetSpeedPower)
        if okSp and type(s2) == "number" then sp = s2 end
    end
    if TreadmillUtil and sp ~= nil then
        local okWs, ws = pcall(TreadmillUtil.SpeedPowerToWalkSpeed, sp)
        if okWs and type(ws) == "number" then
            return math.max(16, ws)
        end
    end
    return 16
end

local function ApplySpeedBoost(on)
    if not SPP then return GetLegalWalkSpeed() end
    if on then
        speedSessionId = math.random(1000000, 2000000000)
        pcall(SPP.BeginSession, speedSessionId)
        -- 1e12 speed power → hits the 300 stud/s walk-speed cap.
        pcall(SPP.RevealCompletedGain, speedSessionId, 1000000000000)
    end
    return GetLegalWalkSpeed()
end

-- Smart movement: runs at maxSpeed far away, DECELERATES smoothly near the
-- target so it stops cleanly (no more overshoot back-and-forth).
-- onTick (optional) may return "abort" to cancel (e.g. egg was lost); it is only
-- called every `onTickEvery` iterations so slow checks don't stutter movement.
-- decelDist overrides the braking range (smaller = keep sprinting longer).
local function WalkToPoint(targetPos, maxSpeed, arriveRadius, timeout, onTick, onTickEvery, decelDist)
    local chr = LocalPlayer.Character
    if not chr then return false, "no char" end
    local hrp = chr:FindFirstChild("HumanoidRootPart")
    local hum = chr:FindFirstChildOfClass("Humanoid")
    if not hrp or not hum then return false, "no hum" end

    local speed = math.max(16, maxSpeed or GetLegalWalkSpeed())
    local arrive = arriveRadius or 10
    local decel = decelDist or math.max(30, arrive + 25)
    local every = math.max(1, onTickEvery or 1)
    local t0 = os.clock()
    local stuckCount, lastPos, tickN = 0, hrp.Position, 0

    while not HUB.dead do
        local now = hrp.Position
        local dx, dz = targetPos.X - now.X, targetPos.Z - now.Z
        local dist = math.sqrt(dx * dx + dz * dz)

        if dist <= arrive then
                return true
        end
        if os.clock() - t0 > (timeout or 90) then
                return false, "timeout"
        end

        tickN = tickN + 1
        if onTick and tickN % every == 0 then
            local act = onTick()
            if act == "abort" then
                        return false, "aborted"
            end
        end

        -- Linear deceleration ramp: full speed far away, crawl at the target.
        local t = (dist - arrive) / (decel - arrive)
        hum.WalkSpeed = math.max(16, math.min(speed, speed * t))

        -- Stuck recovery (jump to clear obstacles/ledges).
        if (now - lastPos).Magnitude < 1.2 then
            stuckCount = stuckCount + 1
            if stuckCount >= 8 then
                pcall(function() hum.Jump = true end)
                stuckCount = 0
            end
        else
            stuckCount = 0
        end
        lastPos = now

        hum:MoveTo(targetPos)
        task.wait(0.12)
    end
    hum.WalkSpeed = 16
    return false, "dead"
end

-- Subtle heal: tops Health back up ONLY when actually damaged, and only to
-- the normal cap. NEVER touches MaxHealth (a billion MaxHealth got us KICKED)
-- and never writes Health every heartbeat (constant client-side Health writes
-- are also a modified-humanoid signal). On-demand heal when hurt = looks like
-- strong regen, not a stat hack.
local infHpConn = nil
local function SetInfHp(on)
    if on == infHpActive then return end
    infHpActive = on
    if on then
        infHpConn = game:GetService("RunService").Heartbeat:Connect(function()
            local chr = LocalPlayer.Character
            if not chr then return end
            local hum = chr:FindFirstChildOfClass("Humanoid")
            if hum and hum.Health > 0 and hum.Health < hum.MaxHealth - 1 then
                pcall(function() hum.Health = hum.MaxHealth end)
            end
        end)
    elseif infHpConn then
        infHpConn:Disconnect()
        infHpConn = nil
    end
end

-- FAKE DEATH — simple, no metatable hooks needed (TP Walk swap handles that).
-- ══════════════════════════════════════════════════════════════════════════════

local function SetFakeDeath(on)
    fakeDeathActive = on
    local chr = LocalPlayer.Character
    if not chr then return end
    local hum = chr:FindFirstChildOfClass("Humanoid")
    if not hum then return end
    if on then
        pcall(function() hum.Health = 0 end)
    else
        pcall(function() hum.Health = hum.MaxHealth end)
    end
end

if LocalPlayer.Character then
    local r0 = LocalPlayer.Character:FindFirstChild("HumanoidRootPart")
    if r0 then safeZonePos = r0.Position end
end

-- CharacterAdded so respawns keep the bypass active.
track(LocalPlayer.CharacterAdded:Connect(function(chr)
    if fakeDeathActive then
        SetFakeDeath(true)
    end
    if not safeZonePos then
        local r = chr:FindFirstChild("HumanoidRootPart")
        if r then safeZonePos = r.Position end
    end
end))

-- Wait until the humanoid is actually standing (after respawn it is briefly
-- ragdolled/"GettingUp", during which MoveTo is ignored — that made the gate
-- walk stall for the full timeout).
local function WaitForStanding(timeout)
    local chr = LocalPlayer.Character
    local hum = chr and chr:FindFirstChildOfClass("Humanoid")
    if not hum then return false end
    local t0 = os.clock()
    while os.clock() - t0 < (timeout or 10) do
        local st = hum:GetState()
        if st ~= Enum.HumanoidStateType.Ragdoll and st ~= Enum.HumanoidStateType.GettingUp and st ~= Enum.HumanoidStateType.Dead then
            return true
        end
        task.wait(0.15)
    end
    return false
end

-- Walk fast through the GameplayZ gate gap (X~552, Z~-366) so the server sets
-- the "in gameplay area" carry flag — teleports alone don't set it.
local function WalkThroughGate()
    local chr = LocalPlayer.Character
    if not chr then return false end
    local hrp = chr:FindFirstChild("HumanoidRootPart")
    local hum = chr:FindFirstChildOfClass("Humanoid")
    if not hrp or not hum then return false end
    if hrp.Position.X > 570 then return true end  -- already past the gate

    -- MoveTo needs a live humanoid: undo fake death just for the gate walk.
    local wasFake = fakeDeathActive
    if wasFake then SetFakeDeath(false) end
    local crossed = false

    WaitForStanding(10)
    if hrp.Position.X > 570 then
        crossed = true
    else
        hum.WalkSpeed = 300  -- fast gate walk (friend's method: walkto + high speed)
        local gy = hrp.Position.Y
        hum:MoveTo(Vector3.new(546, gy, -370))
        local t0 = os.clock()
        while not HUB.dead and os.clock() - t0 < 12 do
            if (hrp.Position - Vector3.new(546, gy, -370)).Magnitude < 6 then break end
            task.wait(0.15)
        end
        hum:MoveTo(Vector3.new(585, gy, -366))
        t0 = os.clock()
        while not HUB.dead and os.clock() - t0 < 12 do
            if hrp.Position.X > 575 then break end
            task.wait(0.15)
        end
        crossed = hrp.Position.X > 570
    end

    if wasFake then SetFakeDeath(true) end
    return crossed
end

-- The gameplay-area flag is set once per character by the gate walk; cache it
-- so we don't MoveTo-fight CFrame on every steal cycle (that reads as tampering).
local gateFlag = false
local gateFlagChar = nil
local function EnsureGameplayFlag()
    local chr = LocalPlayer.Character
    if chr ~= gateFlagChar then
        gateFlagChar = chr
        gateFlag = false  -- respawn resets the flag
    end
    if gateFlag then return true end
    local ok = WalkThroughGate()
    if ok then gateFlag = true end
    return ok
end

-- After every egg reset the game raises the AreaEggResetWall (the big wall at
-- the start-area gate, SeparationLine X~552). Tweening into it while it's up
-- clips against collision and stalls the run. So before any leg that crosses
-- the gate, wait until the wall is fully gone (IsClosed() == false AND the
-- visual has collapsed to ~0 height) before moving.
local function ResetWallIsUp()
    if ResetWall then
        local okC, closed = pcall(function() return ResetWall.IsClosed() end)
        if okC and closed then return true end
        local okV, vis = pcall(function() return ResetWall.GetVisualWallPart() end)
        if okV and vis and vis:IsA("BasePart") and vis.Size.Y > 1 then return true end
    end
    -- Fallback: read the parts directly if the module isn't available.
    local areas = Workspace:FindFirstChild("__OBJECTS") and Workspace.__OBJECTS:FindFirstChild("Areas")
    if areas then
        local col = areas:FindFirstChild("WallStartCollision")
        if col and col:IsA("BasePart") and col.CanCollide then return true end
        local vis = areas:FindFirstChild("WallStartVisual")
        if vis and vis:IsA("BasePart") and vis.Size.Y > 1 then return true end
    end
    return false
end

-- Wait (up to `timeout` seconds) for the reset wall to come down. Returns true
-- once it's safe to move, false if it never came down in time.
local function WaitForResetWall(timeout)
    local t0 = os.clock()
    while not HUB.dead and os.clock() - t0 < (timeout or 20) do
        if not ResetWallIsUp() then return true end
        task.wait(0.15)
    end
    return not ResetWallIsUp()
end

-- True when the character is carrying an egg (the game shows a DropHeldEgg UI).
-- IMPORTANT: the DropHeldEgg ScreenGui ALWAYS exists in PlayerGui — it is
-- Enabled only while actually carrying. Checking existence alone is a
-- permanent false positive that made the script "carry" the egg instantly
-- and run home empty-handed.
local function IsCarryingEgg()
    local p = LocalPlayer:FindFirstChild("PlayerGui")
    if p then
        local dhe = p:FindFirstChild("DropHeldEgg")
        if dhe then
            if dhe:IsA("ScreenGui") then
                if dhe.Enabled == true then return true end
            elseif dhe:IsA("GuiObject") and dhe.Visible == true then
                return true
            end
        end
    end
    local chr = LocalPlayer.Character
    if chr then
        for _, c in ipairs(chr:GetChildren()) do
            if string.find(c.Name, "Egg", 1, true) and (c:IsA("Tool") or c:IsA("Model")) then return true end
        end
    end
    -- Fallback: the server's own carry state (can lag a tick behind claim, so
    -- keep it last so a stale IsCarrying=true never skips the pickup).
    if carryState and carryState.IsCarrying then return true end
    return false
end

-- Find the CarryAreaEgg ProximityPrompt nearest to a position.
-- When eggs are clustered (5 eggs side-by-side within a few studs), the
-- closest prompt may belong to a DIFFERENT egg. FireProximityPrompt on the
-- wrong egg wastes time. So we try the remote first (targets by UID exactly),
-- then fall back to ALL nearby prompts as a last resort.
local function FindEggPrompt(pos)
    local best, bestD = nil, math.huge
    for _, v in ipairs(workspace:GetDescendants()) do
        if v:IsA("ProximityPrompt") and v.Name == "CarryAreaEgg" then
            local pp = v.Parent
            local okp, p = pcall(function() return pp.Position end)
            if okp then
                local d = (p - pos).Magnitude
                if d < bestD then best, bestD = v, d end
            end
        end
    end
    return best
end

-- Returns ALL CarryAreaEgg proximity prompts within `radius` studs of `pos`.
-- When eggs are clustered, firing every nearby prompt guarantees one of them
-- is the right egg.
local function FindAllEggPrompts(pos, radius)
    local found = {}
    for _, v in ipairs(workspace:GetDescendants()) do
        if v:IsA("ProximityPrompt") and v.Name == "CarryAreaEgg" then
            local pp = v.Parent
            local okp, p = pcall(function() return pp.Position end)
            if okp and (p - pos).Magnitude <= (radius or 15) then
                found[#found + 1] = v
            end
        end
    end
    return found
end

local function FindCharRoot()
    local chr = LocalPlayer.Character
    if not chr then return nil end
    return chr:FindFirstChild("HumanoidRootPart") or chr:FindFirstChildWhichIsA("BasePart")
end

-- Hard stop: kill all momentum so the character is perfectly still at the egg.
local function HardStop()
    local chr = LocalPlayer.Character
    if chr then
        local hrp = chr:FindFirstChild("HumanoidRootPart")
        if hrp then
            hrp.AssemblyLinearVelocity = Vector3.zero
            hrp.AssemblyAngularVelocity = Vector3.zero
        end
    end
    task.wait(0.2)
end

-- TP WALK METHOD — instant Humanoid swap (same as the working script's
-- "Bypass Anti Cheat" button). Destroys the game's hooked Humanoid, creates
-- a bare one with no scripts attached.

local function HookProximityPrompts()
    pcall(function()
        local PPS = game:GetService("ProximityPromptService")
        for _, pp in ipairs(PPS:GetDescendants()) do
            if pp:IsA("ProximityPrompt") then pp.HoldDuration = 0 end
        end
        PPS.PromptShown:Connect(function(pp)
            if pp and pp:IsA("ProximityPrompt") then pp.HoldDuration = 0 end
        end)
    end)
end

local function EnableSwim()
    local chr = LocalPlayer.Character
    if not chr then return false end
    local hum = chr:FindFirstChildOfClass("Humanoid")
    -- Already have a bare Humanoid (no game scripts attached)? Skip the swap.
    if hum then
        local hasGameScripts = false
        for _, child in ipairs(hum:GetChildren()) do
            if child:IsA("Script") or child:IsA("LocalScript") then
                hasGameScripts = true; break
            end
        end
        if not hasGameScripts then
            HookProximityPrompts()
            pcall(function() workspace.CurrentCamera.CameraSubject = hum end)
            return true
        end
    end
    -- Swap: parent new first (rig auto-configures), then destroy old.
    HookProximityPrompts()
    local newHum = Instance.new("Humanoid")
    newHum.Parent = chr
    local oldHum = chr:FindFirstChildOfClass("Humanoid")
    if oldHum and oldHum ~= newHum then oldHum:Destroy() end
    pcall(function()
        local cam = workspace.CurrentCamera
        cam.CameraSubject = newHum
    end)
    return true
end

local function DisableSwim()
    local chr = LocalPlayer.Character
    if chr then
        local hum = chr:FindFirstChildOfClass("Humanoid")
        if hum then
            hum.WalkSpeed = 16
            pcall(function()
                hum:SetStateEnabled(Enum.HumanoidStateType.Ragdoll, true)
                hum:SetStateEnabled(Enum.HumanoidStateType.GettingUp, true)
                hum:SetStateEnabled(Enum.HumanoidStateType.Dead, true)
                hum:ChangeState(Enum.HumanoidStateType.Running)
            end)
            pcall(function()
                local cam = workspace.CurrentCamera
                cam.CameraSubject = hum
            end)
            local hrp = chr:FindFirstChild("HumanoidRootPart")
            if hrp and safeZonePos then
                local ok, hit = pcall(function()
                    local rp = RaycastParams.new()
                    rp.FilterType = Enum.RaycastFilterType.Exclude
                    rp.FilterDescendantsInstances = { chr }
                    return workspace:Raycast(Vector3.new(safeZonePos.X, safeZonePos.Y + 20, safeZonePos.Z), Vector3.new(0, -100, 0), rp)
                end)
                local groundY = ok and hit and hit.Position.Y or safeZonePos.Y
                hrp.CFrame = CFrame.new(safeZonePos.X, groundY + 3, safeZonePos.Z)
            end
        end
    end
end

-- ══════════════════════════════════════════════════════════════════════════════
-- Primary: TweenToPoint (TweenService CFrame tween on the root) — one smooth
-- continuous slide per leg, WalkSpeed set to slider speed, X/Z only, no hovering.

-- Home base is the SAFE ZONE (where we spawned), not the fenced plot center —
-- the character never gets wedged in the plot structure this way.
local function GetBasePos()
    -- The egg deposit registers at the PLOT, not the spawn (verified live: the
    -- plot has no collidable parts, so tweens reach the center cleanly). The
    -- spawn/safe zone is only a fallback if the plot can't be resolved.
    local slot = ResolveMySlot()
    local plots = Workspace:FindFirstChild("Plots")
    if slot and plots then
        local p = plots:FindFirstChild(slot)
        if p then
            local c = p:FindFirstChild("CenterPoint")
            if c and c:IsA("BasePart") then return c.Position end
            local sign = p:FindFirstChild("PlotSign")
            if sign and sign:IsA("BasePart") then return sign.Position end
        end
    end
    if safeZonePos then return safeZonePos end
    local spawn = Workspace:FindFirstChildOfClass("SpawnLocation")
    if spawn then return spawn.Position end
    local chr = LocalPlayer.Character
    if chr and chr.PrimaryPart then return chr.PrimaryPart.Position end
    return Vector3.new(530, 0, -366)  -- open spot just before the gate gap
end

-- Hides every treadmill part in the map while auto-stealing so the character
-- never gets stuck on it; restores everything when auto-steal is off / unloaded.
local hiddenTreadmills = {}
local function SetStealTreadmillHidden(hidden)
    for _, d in ipairs(Workspace:GetDescendants()) do
        if d:IsA("BasePart") then
            local isTm = false
            local anc = d
            while anc do
                if string.find(anc.Name:lower(), "treadmill") then
                    isTm = true
                    break
                end
                anc = anc.Parent
            end
            if isTm then
                if hidden and not hiddenTreadmills[d] then
                    hiddenTreadmills[d] = { c = d.CanCollide, t = d.Transparency, q = d.CanQuery, tc = d.CanTouch }
                    d.CanCollide = false
                    d.CanQuery = false
                    d.CanTouch = false
                    d.Transparency = 1
                elseif not hidden and hiddenTreadmills[d] then
                    local o = hiddenTreadmills[d]
                    d.CanCollide, d.CanQuery, d.CanTouch, d.Transparency = o.c, o.q, o.tc, o.t
                    hiddenTreadmills[d] = nil
                end
            end
        end
    end
end

-- ══════════════════════════════════════════════════════════════════════════════
-- ENGINE — STEAL (area eggs)
-- ══════════════════════════════════════════════════════════════════════════════
local function GetEggRecord(uid)
    if not (NETMAP and NETMAP.Eggs and NETMAP.Eggs.REQUEST_AREA_EGG_SNAPSHOT) then return nil end
    local ok, snap = pcall(Network.Invoke, NETMAP.Eggs.REQUEST_AREA_EGG_SNAPSHOT)
    if not (ok and type(snap) == "table" and type(snap.Records) == "table") then return nil end
    for _, r in ipairs(snap.Records) do
        if r.Uid == uid then return r end
    end
    return nil
end

-- Raycast the ground at (x,z) and return a walkable Y (ground + 3). Falls back
-- to the given Y if nothing is hit, so tweens never tunnel into the floor.
local function GroundY(x, z, fallbackY)
    local chr = LocalPlayer.Character
    if not chr then return fallbackY end
    local ok, hit = pcall(function()
        local rp = RaycastParams.new()
        rp.FilterType = Enum.RaycastFilterType.Exclude
        rp.FilterDescendantsInstances = { chr }
        return workspace:Raycast(Vector3.new(x, fallbackY + 30, z), Vector3.new(0, -80, 0), rp)
    end)
    if ok and hit then return hit.Position.Y + 3 end
    return fallbackY
end

local function StealOnce()
    if not ENV_OK then return false, "API not ready" end
    if not NETMAP or not NETMAP.Eggs then return false, "no eggs api" end

    local chr = LocalPlayer.Character
    if not chr or not chr.Parent then return false, "no character" end

    SetStealTreadmillHidden(true)
    EnableSwim()  -- proxyprompt hook + humanoid swap (no-op after first cycle)

    local hrp = chr:FindFirstChild("HumanoidRootPart")
    if not hrp then return false, "no humanoid root" end
    local hum = chr:FindFirstChildOfClass("Humanoid")
    if not hum then return false, "no humanoid" end

    -- Wrap so cleanup always runs once
    local ok, r1 = pcall(function()
        local attempts = 0
        while attempts < 3 and not HUB.dead do
            attempts = attempts + 1

            local snap = Network.Invoke(NETMAP.Eggs.REQUEST_AREA_EGG_SNAPSHOT)
            if type(snap) ~= "table" or type(snap.Records) ~= "table" then error("EXIT:no snapshot") end

            local mypos = hrp.Position
            local best, bestScore = nil, math.huge
            local rareBest, rareScore = nil, math.huge
            for _, r in ipairs(snap.Records) do
                if r.State == "Slot" and stealRaritySet[EggRarityId(r)] then
                    local d = (r.BottomCFrame.Position - mypos).Magnitude
                    local num = EggRarityNumber(r)
                    local score = d - num * 0.5
                    if score < bestScore then best, bestScore = r, score end
                    if rareHunterEnabled and num >= rareMinTier then
                        local rs = d - num * 0.5
                        if rs < rareScore then rareBest, rareScore = r, rs end
                    end
                end
            end
            if rareHunterEnabled and rareBest then best = rareBest end
            if not best then error("EXIT:no matching egg") end

            local eggPos = best.BottomCFrame.Position

            -- Gate & wall: walk through once, then fast-walk to the egg.
            WaitForResetWall(25)
            EnsureGameplayFlag()

            -- Fast walk (MoveTo-based, respects walls) to within 4 studs.
            local gyW = GroundY(eggPos.X, eggPos.Z, eggPos.Y)
            local okOut = FastWalk(Vector3.new(eggPos.X, gyW, eggPos.Z), 60)
            if not okOut then error("EXIT:walk out failed") end
            if not (chr.Parent and hrp.Parent) then error("EXIT:no char") end

            -- Instant snap: only the final few studs — we're already at the egg,
            -- this just nails the exact position for the prompt.
            hum.WalkSpeed = 16
            hrp.AssemblyLinearVelocity = Vector3.zero
            hrp.AssemblyAngularVelocity = Vector3.zero
            hrp.CFrame = CFrame.new(eggPos.X, gyW, eggPos.Z)
            task.wait(0.08)

            -- PICK UP — proximity prompt ONLY. Verified live: firing the
            -- CarryAreaEgg prompt flips the record Slot -> Carried with our
            -- CarrierUserId, while the EggCmds.RequestCarryAreaEgg remote is
            -- DEAD for snapshot UIDs (server answers "Egg not found"), so the
            -- remote is not even attempted. Clustered eggs (5 side by side)
            -- are handled by firing prompts NEAREST-FIRST (the one standing
            -- on the egg is ours) and verifying by UID that the exact egg was
            -- grabbed, dropping any wrong egg a neighbor prompt picked up.
            -- PICK UP: fire the nearest CarryAreaEgg prompt, verify we got it.
            -- Clustered eggs are handled by nearest-first sorting + UID check.
            local firePP = getgenv and getgenv().fireproximityprompt or fireproximityprompt
            local carried = false

            for attempt = 1, 10 do
                if carried or HUB.dead then break end

                -- Fire nearest prompt
                local pp = FindEggPrompt(eggPos)
                if pp then pcall(firePP, pp) end

                -- Wait for carry state
                local t0 = os.clock()
                while not HUB.dead and os.clock() - t0 < 0.6 do
                    if IsCarryingEgg() then break end
                    task.wait(0.05)
                end

                -- Verify by UID
                if IsCarryingEgg() then
                    local rec = EggCmds and EggCmds.GetAreaEggRecord and EggCmds.GetAreaEggRecord(best.Uid)
                    if not rec then rec = GetEggRecord(best.Uid) end
                    if rec and rec.State == "Carried" and tostring(rec.CarrierUserId) == tostring(LocalPlayer.UserId) then
                        carried = true
                        break
                    end
                    -- Wrong egg — drop and retry
                    if EggCmds and EggCmds.RequestDropHeldAreaEgg then
                        pcall(EggCmds.RequestDropHeldAreaEgg, nil)
                    end
                    task.wait(0.2)
                end
            end

            if not carried then error("EXIT:carry failed") end

            -- HOME: fast walk back, instant-snap final approach.
            local base = GetBasePos()
            local eggUid = best.Uid
            local gyHome = GroundY(base.X, base.Z, base.Y)
            local okHome = FastWalk(Vector3.new(base.X, gyHome, base.Z), 60)
            if not okHome then error("EXIT:walk home failed") end
            if not (chr.Parent and hrp.Parent) then error("EXIT:no char") end

            hum.WalkSpeed = 16
            hrp.AssemblyLinearVelocity = Vector3.zero
            hrp.AssemblyAngularVelocity = Vector3.zero
            hrp.CFrame = CFrame.new(base.X, gyHome, base.Z)

            -- CLAIM: the egg auto-claims when you arrive at the plot while carrying.
            local claimed = false
            local claimT0 = os.clock()

            while not claimed and not HUB.dead and os.clock() - claimT0 < 6 do
                task.wait(0.2)
                local s = GetSave()
                if s and s.EggInventory and s.EggInventory[eggUid] ~= nil then
                    claimed = true; break
                end
                if not IsCarryingEgg() then break end
            end

            if not claimed then
                local s = GetSave()
                if s and s.EggInventory and s.EggInventory[eggUid] ~= nil then
                    claimed = true
                end
            end

            if claimed then error("EXIT:OK:" .. tostring(best.AssetCategory)) end

            Notify("Auto Steal", "Retrying...", "Info", 2)
        end
        error("EXIT:egg lost too many times")
    end)

    -- NOTE: DisableSwim is NOT called between rounds — the clean Humanoid
    -- stays active for the next steal. Only the toggle-off path calls it.

    if not ok then
        local msg = tostring(r1)
        if msg:sub(1,5) == "EXIT:" then
            local exitMsg = msg:sub(6)
            if exitMsg:sub(1,3) == "OK:" then return true, exitMsg:sub(4) end
            return false, exitMsg
        end
        return false, msg
    end
    return false, "unexpected"
end

-- ══════════════════════════════════════════════════════════════════════════════
-- ENGINE — HATCH
-- ══════════════════════════════════════════════════════════════════════════════
local function HatchOnce()
    if not ENV_OK then return 0 end
    local s = GetSave()
    if not s then return 0 end
    local eggs = s.EggInventory or {}
    local done = 0
    for uid, egg in pairs(eggs) do
        if HUB.dead then break end
        if type(uid) ~= "string" then uid = tostring(uid) end
        if type(egg) ~= "table" then egg = {} end

        -- 1) place unplaced eggs
        if egg.Placement == nil then
            local okPlace = false
            if EggCmds and EggCmds.RequestPlaceEgg then
                local ok, err = EggCmds.RequestPlaceEgg(uid, PlotCFrame())
                okPlace = ok == true
                if not okPlace then
                    -- print("[HUB] place failed: " .. tostring(err))
                end
            end
            task.wait(0.15)
        end

        -- 2) try to hatch (only succeeds when growth is complete)
        if EggCmds and EggCmds.RequestHatchEgg then
            local okHatch = EggCmds.RequestHatchEgg(uid)
            if okHatch == true then
                if EggCmds.RequestCompleteHatchEgg then
                    local okDone = EggCmds.RequestCompleteHatchEgg(uid)
                    if okDone == true then done = done + 1 end
                else
                    done = done + 1
                end
                task.wait(0.2)
            end
        end
    end
    return done
end

-- ══════════════════════════════════════════════════════════════════════════════
-- ENGINE — EQUIP BEST
-- ══════════════════════════════════════════════════════════════════════════════
local function EquipBestOnce()
    if not ENV_OK or not NETMAP or not NETMAP.Backpack then return false end
    local ok, res = pcall(Network.Invoke, NETMAP.Backpack.EQUIP_BEST)
    return ok and (res == true or res == nil)
end

-- ══════════════════════════════════════════════════════════════════════════════
-- ENGINE — SELL
-- ══════════════════════════════════════════════════════════════════════════════
local function PushAutoSellConfig()
    if not ENV_OK or not NETMAP or not NETMAP.Backpack then return false end
    local map = BuildSellMap()
    local ok = pcall(Network.Invoke, NETMAP.Backpack.SET_AUTO_SELL_STATE, map)
    return ok
end

local function SellByRarityOnce()
    if not ENV_OK then return 0 end
    local s = GetSave()
    if not s then return 0 end
    local inv = s.Inventory or {}
    local sold = 0
    for uid, rec in pairs(inv) do
        if HUB.dead then break end
        if type(rec) ~= "table" then rec = {} end
        if sellRaritySet[RarityId(rec)] then
            -- sell equipped assets via RequestSell (Invoke → bool)
            local okInvoke = false
            if NETMAP and NETMAP.ActiveAssets and NETMAP.ActiveAssets.REQUEST_SELL then
                local ok, res = pcall(Network.Invoke, NETMAP.ActiveAssets.REQUEST_SELL, uid)
                okInvoke = ok and res == true
            end
            if not okInvoke and NETMAP and NETMAP.AssetInventory and NETMAP.AssetInventory.SELL_ASSET then
                -- unequipped assets: fire & forget
                pcall(Network.Fire, NETMAP.AssetInventory.SELL_ASSET, uid)
            end
            if okInvoke then sold = sold + 1 end
            task.wait(0.05)
        end
    end
    return sold
end

-- ══════════════════════════════════════════════════════════════════════════════
-- ENGINE — SELL EGGS (hatched eggs / egg-inventory)
-- ══════════════════════════════════════════════════════════════════════════════
local function SellEggsOnce()
    if not ENV_OK then return 0 end
    local s = GetSave()
    if not s then return 0 end
    local eggs = s.EggInventory or {}
    if type(eggs) ~= "table" then return 0 end
    local sold = 0
    for uid, egg in pairs(eggs) do
        if HUB.dead then break end
        if type(uid) ~= "string" then uid = tostring(uid) end
        if type(egg) ~= "table" then egg = {} end
        local cat = egg.AssetCategory or egg.Category
        if cat and eggSellRaritySet[EggRarityId(egg) or ""] then
            -- Sell via ActiveAssets.RequestSell (RemoteFunction, Invoke)
            local okInvoke = false
            if NETMAP and NETMAP.ActiveAssets and NETMAP.ActiveAssets.REQUEST_SELL then
                local ok, res = pcall(Network.Invoke, NETMAP.ActiveAssets.REQUEST_SELL, uid)
                okInvoke = ok and res == true
            end
            -- Fallback: AssetInventory.SellAsset (RemoteEvent, Fire)
            if not okInvoke and NETMAP and NETMAP.AssetInventory and NETMAP.AssetInventory.SELL_ASSET then
                pcall(Network.Fire, NETMAP.AssetInventory.SELL_ASSET, uid)
            end
            if okInvoke then sold = sold + 1 end
            task.wait(0.05)
        end
    end
    return sold
end

-- ══════════════════════════════════════════════════════════════════════════════
-- ENGINE — CLAIM (index / group / offline money)
-- ══════════════════════════════════════════════════════════════════════════════
local function ClaimOnce()
    if not ENV_OK or not NETMAP then return 0 end
    local claimed = 0

    -- Index collection rewards (RequestClaimAll takes no args)
    if NETMAP.Index and NETMAP.Index.REQUEST_CLAIM_ALL then
        local ok, res, err = pcall(Network.Invoke, NETMAP.Index.REQUEST_CLAIM_ALL)
        if ok and res == true then claimed = claimed + 1 end
    end

    -- Limited egg reward
    if NETMAP.Index and NETMAP.Index.REQUEST_CLAIM_LIMITED_EGG_REWARD then
        local ok, res = pcall(Network.Invoke, NETMAP.Index.REQUEST_CLAIM_LIMITED_EGG_REWARD)
        if ok and res == true then claimed = claimed + 1 end
    end

    -- Group reward
    if NETMAP.GroupReward and NETMAP.GroupReward.CLAIM_REWARD then
        local ok, res = pcall(Network.Invoke, NETMAP.GroupReward.CLAIM_REWARD)
        if ok and res == true then claimed = claimed + 1 end
    end

    -- Offline money
    if NETMAP.OfflineAssets and NETMAP.OfflineAssets.GET_SUMMARY and NETMAP.OfflineAssets.REQUEST_REDEEM then
        local okSum, sum = pcall(Network.Invoke, NETMAP.OfflineAssets.GET_SUMMARY)
        if okSum and type(sum) == "table" and (tonumber(sum.ClaimableAmount) or 0) > 0 then
            local okRed = pcall(Network.Invoke, NETMAP.OfflineAssets.REQUEST_REDEEM)
            if okRed then claimed = claimed + 1 end
        end
    end

    return claimed
end

-- ══════════════════════════════════════════════════════════════════════════════
-- ENGINE — UPGRADES (base / equip limit)
-- ══════════════════════════════════════════════════════════════════════════════
local function UpgradeOnce()
    if not ENV_OK or not NETMAP then return 0 end
    local done = 0

    -- Base upgrade (RemoteEvent, fire & forget — server validates money)
    if NETMAP.Plots and NETMAP.Plots.REQUEST_BASE_UPGRADE then
        pcall(Network.Fire, NETMAP.Plots.REQUEST_BASE_UPGRADE)
        done = done + 1
    end

    -- Equip limit (unlock more pet slots)
    if NETMAP.ActiveAssets and NETMAP.ActiveAssets.REQUEST_EQUIP_LIMIT then
        local ok, res = pcall(Network.Invoke, NETMAP.ActiveAssets.REQUEST_EQUIP_LIMIT)
        if ok and res == true then done = done + 1 end
    end

    return done
end

-- ══════════════════════════════════════════════════════════════════════════════
-- ENGINE — TREADMILL (SpeedPower farm)
-- ══════════════════════════════════════════════════════════════════════════════
local function FindMyTreadmillPart()
    local slot = ResolveMySlot()
    local plots = Workspace:FindFirstChild("Plots")
    if slot and plots then
        local p = plots:FindFirstChild(slot)
        if p then
            local tb = p:FindFirstChild("TreadmillBottom")
            if tb and tb:IsA("BasePart") then return tb end
            local tu = p:FindFirstChild("TreadmillUpgrade")
            if tu then
                local part = tu:FindFirstChildWhichIsA("BasePart")
                if part then return part end
            end
        end
    end
    return nil
end

local function TryUpgradeTreadmill()
    if not (TreadmillDir and NETMAP and NETMAP.Treadmills and NETMAP.Treadmills.REQUEST_UPGRADE) then return false end
    local s = GetSave()
    if not s then return false end
    local lvl = tonumber(s.TreadmillUpgradeLevel) or 0
    local okCfg, nextCfg = pcall(TreadmillDir.GetByUpgradeLevel, lvl + 1)
    if not (okCfg and type(nextCfg) == "table" and nextCfg._id) then return false end
    if (tonumber(s.Money) or 0) < (tonumber(nextCfg.Price) or math.huge) then return false end
    local okUp, res = pcall(Network.Invoke, NETMAP.Treadmills.REQUEST_UPGRADE, nextCfg._id)
    return okUp and res == true
end

function TreadmillLoop()
    if HUB.loops.treadmill then return end
    HUB.loops.treadmill = true
    while treadmillEnabled and not HUB.dead do
        local tb = FindMyTreadmillPart()
        if tb then
            local legal = GetLegalWalkSpeed()
            WalkToPoint(tb.Position, math.min(legal, 30), 3, 25)
            local origin = tb.Position
            local bobT, dir = 0, 1
            while treadmillEnabled and not HUB.dead do
                if treadmillAutoUpgrade and TryUpgradeTreadmill() then
                    Notify("Treadmill", "Treadmill geupgradet!", "Success", 2)
                end
                local chr = LocalPlayer.Character
                local hum = chr and chr:FindFirstChildOfClass("Humanoid")
                if hum then
                    hum.WalkSpeed = math.min(legal, 20)
                    bobT = bobT + 1
                    dir = (bobT % 2 == 0) and 1 or -1
                    hum:MoveTo(origin + Vector3.new(dir * 6, 0, 0))
                end
                task.wait(1.2)
            end
        else
            Notify("Treadmill", "Kein Treadmill gefunden — Resync Plot", "Error", 2)
            mySlot = nil
            task.wait(5)
        end
    end
    HUB.loops.treadmill = false
end

-- ══════════════════════════════════════════════════════════════════════════════
-- STATUS
-- ══════════════════════════════════════════════════════════════════════════════
local statusLabel = nil
local function fmtMoney(n)
    n = tonumber(n) or 0
    if n >= 1e9 then return string.format("%.2fB", n / 1e9) end
    if n >= 1e6 then return string.format("%.2fM", n / 1e6) end
    if n >= 1e3 then return string.format("%.1fK", n / 1e3) end
    return tostring(math.floor(n))
end

local function RefreshStatus()
    if not statusLabel then return end
    local s = GetSave()
    if not s then
        pcall(statusLabel.Set, statusLabel, "Game API: not loaded")
        return
    end
    local pets = CountTable(s.Inventory)
    local eggs = CountTable(s.EggInventory)
    local line = string.format(
        "💰 %s  ·  🐾 %d Pets  ·  🥚 %d Eggs  ·  🏠 Lv.%s  ·  ⚡ %d spd%s",
        fmtMoney(GetMoney()), pets, eggs, tostring(s.BaseUpgradeLevel or 0),
        math.floor(GetLegalWalkSpeed()), speedBoostEnabled and " (boost)" or ""
    )
    pcall(statusLabel.Set, statusLabel, line)
end

-- ══════════════════════════════════════════════════════════════════════════════
-- ENGINE — VISUAL PET (client-side only: real model + real mutation + real size,
-- but never saved server-side. Follows you like an equipped pet.)
-- ══════════════════════════════════════════════════════════════════════════════
local function LoadVisualData()
    if visualDataReady then return end
    visualDataReady = true

    local okA, AssetsDir = pcall(require, ReplicatedStorage:WaitForChild("Directory"):WaitForChild("Assets"))
    if okA then
        local dir = AssetsDir.Directory or AssetsDir.Assets or AssetsDir
        if type(dir) == "table" then
            for cat, cfg in pairs(dir) do
                if type(cfg) == "table" and cfg.DisplayName then
                    local rar = cfg.Rarity
                    local rarName = type(rar) == "table" and (rar._id or rar.DisplayName) or nil
                    local label = tostring(cfg.DisplayName)
                    if rarName then label = label .. "  ·  " .. tostring(rarName) end
                    VISUAL_PET_BY_LABEL[label] = cat
                    table.insert(VISUAL_PET_ORDER, label)
                end
            end
            table.sort(VISUAL_PET_ORDER)
        end
    end

    local okM, Mut = pcall(require, ReplicatedStorage:WaitForChild("Library"):WaitForChild("Modules"):WaitForChild("Mutations"))
    if okM and type(Mut.GetMutationsMap) == "function" then
        local map = Mut.GetMutationsMap()
        if type(map) == "table" then
            local names = {}
            for name in pairs(map) do
                local n = tostring(name)
                if n ~= "None" then table.insert(names, n) end
            end
            table.sort(names)
            VISUAL_MUTATIONS = { "None" }
            for _, n in ipairs(names) do table.insert(VISUAL_MUTATIONS, n) end
        end
    end
end

local function GetVisualSave()
    local ok, Save = pcall(require, ReplicatedStorage:WaitForChild("Library"):WaitForChild("Client"):WaitForChild("Save"))
    if not ok or type(Save.Get) ~= "function" then return nil end
    local s = Save.Get(LocalPlayer)
    return s
end

local function GetNextEquipSlot(s)
    local used = {}
    if s and type(s.EquippedAssets) == "table" then
        for k in pairs(s.EquippedAssets) do
            local n = tonumber(k)
            if n then used[n] = true end
        end
    end
    for i = 1, 64 do
        if not used[i] then return i end
    end
    return 1
end

local function UnequipVisualPet()
    -- put every visual tool back into the Backpack so the character stops holding them
    local okS, Save = pcall(require, ReplicatedStorage:WaitForChild("Library"):WaitForChild("Client"):WaitForChild("Save"))
    for uid, entry in pairs(visualPets) do
        local tool = entry and entry.tool
        if tool and tool.Parent and tool.Parent ~= LocalPlayer.Backpack then
            pcall(function() tool.Parent = LocalPlayer.Backpack end)
        end
        local slot = entry and entry.slot
        if slot and okS and type(Save.ProcessManualTableKeyChange) == "function" then
            pcall(Save.ProcessManualTableKeyChange, "EquippedAssets", slot, nil, LocalPlayer)
        end
        if entry then entry.slot = nil end
    end
end

-- Removes ALL spawned visual pets (tools + inventory + equipped entries).
local function RemoveVisualPet()
    UnequipVisualPet()
    local okS, Save = pcall(require, ReplicatedStorage:WaitForChild("Library"):WaitForChild("Client"):WaitForChild("Save"))
    for uid, entry in pairs(visualPets) do
        local tool = entry and entry.tool
        if tool then pcall(function() tool:Destroy() end) end
        if okS and type(Save.ProcessManualTableKeyChange) == "function" then
            pcall(Save.ProcessManualTableKeyChange, "Inventory", uid, nil, LocalPlayer)
        end
    end
    visualPets = {}
    lastVisualUid = nil
end

-- Builds the records (typed for the 3D renderer, server-format for the inventory)
-- and a fake UID. Does NOT create the model or touch the save — just data.
local function BuildVisualPetData(category, mutation, weightKg)
    local cfg = nil
    local okCfg, AssetsDir = pcall(require, ReplicatedStorage:WaitForChild("Directory"):WaitForChild("Assets"))
    if okCfg then
        local dir = AssetsDir.Directory or AssetsDir.Assets or AssetsDir
        cfg = dir and dir[category]
    end
    local modelWeight = cfg and (tonumber(cfg.ModelWeight) or 1) or 1
    local weight = tonumber(weightKg) or modelWeight
    local scale = math.clamp(weight / math.max(modelWeight, 0.0001), 0.05, 200)

    local colors = nil
    local okC, AssetColorUtil = pcall(require, ReplicatedStorage:WaitForChild("Library"):WaitForChild("Util"):WaitForChild("AssetColorUtil"))
    if okC and type(AssetColorUtil.ResolveFields) == "function" then
        colors = AssetColorUtil.ResolveFields(category, nil, nil, nil)
    end
    local colorSeed = colors and (tonumber(colors.ColorSeed) or 0) or 0
    local colorIndex = colors and (tonumber(colors.ColorIndex) or 1) or 1
    local eyeColor = colors and tostring(colors.EyeColor) or ""

    local hasMutation = mutation ~= nil and mutation ~= "None"

    local typedRecord = {
        Category = category,
        Mutations = hasMutation and { mutation } or {},
        BaseMutation = hasMutation and mutation or nil,
        Scale = scale,
        Gender = "Male",
        EyeColor = eyeColor,
        ColorSeed = colorSeed,
        ColorIndex = colorIndex,
        Personality = "Normal",
        HasBeenFirstPlaced = true,
    }

    -- IMPORTANT: the backpack deserializes these records expecting REAL types
    -- (Scale/ColorSeed/ColorIndex are numbers, InFuse/HasBeenFirstPlaced are
    -- booleans, Mutations is an array). Strings here break its renderer.
    local inventoryRecord = {
        Category = category,
        Mutations = hasMutation and { mutation } or {},
        BaseMutation = hasMutation and mutation or nil,
        Scale = scale,
        Gender = "Male",
        EyeColor = eyeColor,
        ColorSeed = colorSeed,
        ColorIndex = colorIndex,
        Personality = "Normal",
        HasBeenFirstPlaced = true,
        InFuse = false,
    }

    local HttpService = game:GetService("HttpService")
    local uid = "visual" .. HttpService:GenerateGUID(false):gsub("-", "")

    return cfg, typedRecord, inventoryRecord, uid
end

-- Fetches the pet's full model content (parts + meshes + textures). The template
-- instance replicates fast, but its mesh/texture content streams in on demand —
-- heavy pets (Cerberus, dragons, …) have dozens of meshes and appear blank until
-- they download. We use the game's own PreloadAssets helper (which retries failed
-- fetches) so this never hangs on a broken asset, and cache the result so each
-- pet is only fetched once. Runs blocking — call it from a background thread.
local function EnsurePetLoaded(category, waitSeconds)
    if not category or category == "" then return nil end
    if preloadedPets[category] then
        local folder = ReplicatedStorage:FindFirstChild("AssetModels")
        return folder and folder:FindFirstChild(category) or nil
    end

    local template = nil
    local okA, AssetModels = pcall(require, ReplicatedStorage:WaitForChild("Library"):WaitForChild("Modules"):WaitForChild("AssetModels"))
    if okA and type(AssetModels.WaitForAssetModel) == "function" then
        local okT, t = pcall(AssetModels.WaitForAssetModel, category, 30)
        if okT then template = t end
    end
    if not template then
        local folder = ReplicatedStorage:FindFirstChild("AssetModels")
        if folder then template = folder:FindFirstChild(category) end
    end
    if not template then return nil end

    -- wait for the parts to replicate in
    local t0 = os.clock()
    while not template.PrimaryPart and os.clock() - t0 < 15 do
        task.wait(0.2)
    end

    -- full content fetch with the game's retry helper
    local okP, PreloadAssets = pcall(require, ReplicatedStorage:WaitForChild("Library"):WaitForChild("Functions"):WaitForChild("PreloadAssets"))
    if okP and type(PreloadAssets) == "function" then
        pcall(PreloadAssets, template)
    else
        -- bounded fallback (background thread + timeout) so it never hangs
        local ContentProvider = game:GetService("ContentProvider")
        local done = false
        task.spawn(function()
            pcall(function() ContentProvider:PreloadAsync({ template }) end)
            done = true
        end)
        local t1 = os.clock()
        while not done and os.clock() - t1 < (waitSeconds or 12) do
            task.wait(0.1)
        end
    end

    preloadedPets[category] = true
    return template
end

-- Background task: preload every pet's full model in PARALLEL so heavy pets
-- (dozens of meshes each) download simultaneously instead of one-at-a-time.
-- A sequential loop would block ~30s on each heavy pet (several minutes total);
-- parallel threads finish in roughly the time of the single slowest pet.
local function PreloadAllPets()
    task.spawn(function()
        local cats = {}
        for _, label in ipairs(VISUAL_PET_ORDER) do
            local cat = VISUAL_PET_BY_LABEL[label]
            if cat and not preloadedPets[cat] then
                table.insert(cats, cat)
            end
        end
        for _, cat in ipairs(cats) do
            if HUB.dead then break end
            task.spawn(function()
                pcall(EnsurePetLoaded, cat, 60)
            end)
            task.wait(0.02) -- tiny stagger so all 84 don't flood the queue on one tick
        end
    end)
end

-- Adds the pet to the inventory (Pets tab) AND builds the real holdable Tool.
-- The Tool lives in the Backpack, so the game's own backpack click finds it via
-- findAssetToolByUID and equips it natively (the character then holds the pet).
-- Each call ADDS a new pet alongside existing ones — it does NOT remove them.
local function SpawnVisualPet(category, mutation, weightKg)
    if not category or category == "" then return false, "No pet selected" end

    local cfg, typedRecord, inventoryRecord, uid = BuildVisualPetData(category, mutation, weightKg)

    local okS, Save = pcall(require, ReplicatedStorage:WaitForChild("Library"):WaitForChild("Client"):WaitForChild("Save"))
    if not (okS and type(Save.ProcessManualTableKeyChange) == "function") then
        return false, "Save unavailable"
    end
    local s = GetVisualSave()
    if not s then return false, "Save not loaded yet" end

    local okInv = pcall(Save.ProcessManualTableKeyChange, "Inventory", uid, inventoryRecord, LocalPlayer)
    if not okInv then return false, "Inventory inject failed" end

    -- force the full model to be fetched before we build the Tool
    EnsurePetLoaded(category, 20)

    -- Build the real Tool (same function the game uses for real pets). It is
    -- parented to the Backpack automatically and only renders its model when
    -- equipped. Without this, the backpack slot has nothing to find/equip.
    local tool = nil
    local okD, ItemDisplay = pcall(require, ReplicatedStorage:WaitForChild("Library"):WaitForChild("Modules"):WaitForChild("ItemDisplay"))
    if okD and type(ItemDisplay.CreateTool) == "function" then
        local okT, t = pcall(ItemDisplay.CreateTool, typedRecord, uid, LocalPlayer)
        if okT and t then tool = t end
    end

    visualPets[uid] = { tool = tool, category = category }
    lastVisualUid = uid

    return true, cfg and cfg.DisplayName or category
end

-- Equips the most recently spawned pet's Tool so the character holds it (model +
-- animations render automatically via the Tool.Equipped handler). Every pet can
-- also be equipped individually through the game's own backpack.
local function EquipVisualPet()
    local entry = lastVisualUid and visualPets[lastVisualUid]
    if not entry then
        return false, "Spawn a pet first"
    end
    local tool = entry.tool
    if not tool then
        return false, "Tool not created — add the pet again"
    end

    local chr = LocalPlayer.Character
    local hum = chr and chr:FindFirstChildOfClass("Humanoid")
    if not hum then return false, "No humanoid" end

    -- already holding it
    if tool.Parent == chr then
        return true, entry.category
    end

    pcall(function() hum:UnequipTools() end)
    hum:EquipTool(tool)

    -- wait for the Tool.Equipped handler to render the model; retry once if the
    -- model did not stream in yet (some pets have heavier rigs/meshes)
    local rendered = false
    for attempt = 1, 2 do
        local t0 = os.clock()
        while os.clock() - t0 < 2 do
            local parts = 0
            for _, d in ipairs(tool:GetDescendants()) do
                if d:IsA("BasePart") then parts = parts + 1 end
            end
            if parts > 1 then rendered = true break end
            task.wait(0.1)
        end
        if rendered then break end
        -- re-trigger the lazy render
        pcall(function() hum:UnequipTools() end)
        task.wait(0.1)
        pcall(function() hum:EquipTool(tool) end)
    end

    -- mark equipped in the client save so the backpack's equipped/hotbar views
    -- include it (the real Tool now exists, so hasLocalAssetToolUID is true).
    local okS, Save = pcall(require, ReplicatedStorage:WaitForChild("Library"):WaitForChild("Client"):WaitForChild("Save"))
    if okS and type(Save.ProcessManualTableKeyChange) == "function" then
        local s = GetVisualSave()
        if s then
            local slot = GetNextEquipSlot(s)
            pcall(Save.ProcessManualTableKeyChange, "EquippedAssets", slot, lastVisualUid, LocalPlayer)
            entry.slot = slot
        end
    end

    return true, entry.category
end

-- ══════════════════════════════════════════════════════════════════════════════
-- UI
-- ══════════════════════════════════════════════════════════════════════════════

-- ── Tab 1: Farm ───────────────────────────────────────────────────────────────
local FarmTab = Window:AddTab({ Name = "Farm", Subtitle = "Auto hatch & pets", Icon = "lightning" })

local StealSub = FarmTab:AddSubTab("Auto Steal Eggs")
StealSub:AddToggle({
    Name = "Enable Auto Steal",
    Default = false,
    Flag = "steal_enabled",
    Callback = safeCallback(function(v)
        stealEnabled = v
        SetStealTreadmillHidden(v)
        SetInfHp(v)  -- full-heal while auto-steal is on (teleport safety net)
        notifyOn("Auto Steal", v)
        if v then
            -- Camera watchdog: keep the camera glued to the character's root
            -- while auto-stealing. The Humanoid swap in EnableSwim can briefly
            -- lose the camera subject; this heartbeat watchdog pins the camera
            -- to the HRP (never destroyed by the swap) every frame so it never
            -- floats off or freezes mid-run.
            HUB.cameraHb = game:GetService("RunService").Heartbeat:Connect(function()
                if HUB.dead then return end
                local c = LocalPlayer.Character
                if c then
                    local r = c:FindFirstChild("HumanoidRootPart")
                    if r then
                        workspace.CurrentCamera.CameraSubject = r
                    end
                end
            end)
            safeSpawn(StealLoop)
        else
            if HUB.cameraHb then HUB.cameraHb:Disconnect(); HUB.cameraHb = nil end
            DisableSwim()  -- restore everything to normal
        end
    end),
})
StealSub:AddToggle({
    Name = "Speed Boost",
    Default = false,
    Flag = "steal_speedboost",
    Callback = safeCallback(function(v)
        speedBoostEnabled = v
        ApplySpeedBoost(v)
        Notify("Speed Boost", v and ("ON · legal speed now " .. math.floor(GetLegalWalkSpeed()) .. " stud/s") or "OFF", v and "Success" or "Info")
    end),
})
StealSub:AddSlider({
    Name = "Glide Speed",
    Min = 50,
    Max = 400,
    Default = 250,
    Suffix = " studs/s",
    Flag = "steal_speed",
    Callback = function(v) stealSpeed = math.clamp(v, 50, 400) end,
})
StealSub:AddLabel("Eggs stolen: " .. tostring(stealCount) .. " (this session)")
local stealRarityDropdown = StealSub:AddMultiDropdown({
    Name = "Egg Rarity Filter",
    Options = RARITY_OPTIONS,
    Default = RARITY_OPTIONS,
    Flag = "steal_rarities",
    Callback = function(sel) stealRaritySet = SetFromList(sel) end,
})
registerResync(stealRarityDropdown, function(sel) stealRaritySet = SetFromList(sel) end)
StealSub:AddToggle({
    Name = "Rare Egg Hunter",
    Default = false,
    Flag = "steal_rarehunter",
    Callback = safeCallback(function(v)
        rareHunterEnabled = v
        notifyOn("Rare Egg Hunter", v)
    end),
})
StealSub:AddDropdown({
    Name = "Rare Tier",
    Options = RARE_TIER_OPTIONS,
    Default = "Secret+ (8)",
    Flag = "steal_raretier",
    Callback = function(v) rareMinTier = RARE_TIER_VALUES[v] or 8 end,
})
StealSub:AddButton({
    Name = "Steal Once",
    Callback = safeCallback(function()
        task.spawn(function()
            local ok, info = StealOnce()
            Notify("Steal Egg", ok and ("Stole: " .. tostring(info)) or tostring(info), ok and "Success" or "Error", 3)
            DisableSwim()
        end)
    end),
})

local HopSub = FarmTab:AddSubTab("Auto Server Hop")
HopSub:AddToggle({
    Name = "Enable Auto Server Hop",
    Default = false,
    Flag = "serverhop_enabled",
    Callback = safeCallback(function(v)
        serverHopEnabled = v
        notifyOn("Auto Server Hop", v)
        if v then safeSpawn(ServerHopLoop) end
    end),
})
HopSub:AddSlider({
    Name = "Max Hops",
    Min = 1,
    Max = 100,
    Default = 40,
    Suffix = "",
    Flag = "serverhop_max",
    Callback = function(v) serverHopMax = v end,
})
HopSub:AddSlider({
    Name = "Check Delay",
    Min = 5,
    Max = 60,
    Default = 12,
    Suffix = "s",
    Flag = "serverhop_delay",
    Callback = function(v) serverHopDelay = v end,
})
HopSub:AddButton({
    Name = "Server Hop Now",
    Callback = safeCallback(function()
        task.spawn(function()
            Notify("Server Hop", "Finding a new server...", "Info", 3)
            if not HopServer() then
                Notify("Server Hop", "Teleport failed", "Error", 4)
            end
        end)
    end),
})

local HatchSub = FarmTab:AddSubTab("Auto Hatch")
HatchSub:AddToggle({
    Name = "Enable Auto Hatch",
    Default = false,
    Flag = "hatch_enabled",
    Callback = safeCallback(function(v)
        hatchEnabled = v
        notifyOn("Auto Hatch", v)
        if v then safeSpawn(HatchLoop) end
    end),
})
HatchSub:AddButton({
    Name = "Hatch Once",
    Callback = safeCallback(function()
        task.spawn(function()
            local n = HatchOnce()
            Notify("Auto Hatch", n > 0 and ("Hatched " .. n .. " egg(s)") or "Nothing ready to hatch", n > 0 and "Success" or "Info")
        end)
    end),
})

local EquipSub = FarmTab:AddSubTab("Auto Equip")
EquipSub:AddToggle({
    Name = "Enable Auto Equip Best",
    Default = false,
    Flag = "equip_enabled",
    Callback = safeCallback(function(v)
        equipEnabled = v
        notifyOn("Auto Equip", v)
        if v then safeSpawn(EquipLoop) end
    end),
})
EquipSub:AddButton({
    Name = "Equip Best Now",
    Callback = safeCallback(function()
        task.spawn(function()
            local ok = EquipBestOnce()
            Notify("Equip Best", ok and "Equipped best pets" or "Equip failed", ok and "Success" or "Error")
        end)
    end),
})

local SellSub = FarmTab:AddSubTab("Auto Sell")
SellSub:AddToggle({
    Name = "Enable Auto Sell",
    Default = false,
    Flag = "sell_enabled",
    Callback = safeCallback(function(v)
        sellEnabled = v
        notifyOn("Auto Sell", v)
        if v then
            PushAutoSellConfig()
            safeSpawn(SellLoop)
        end
    end),
})
local sellRarityDropdown = SellSub:AddMultiDropdown({
    Name = "Rarities to Sell",
    Options = RARITY_OPTIONS,
    Default = { "Basic", "Common", "Uncommon", "SuperRare", "Celestial", "Rare" },
    Flag = "sell_rarities",
    Callback = function(sel)
        sellRaritySet = SetFromList(sel)
        if sellEnabled then PushAutoSellConfig() end
    end,
})
registerResync(sellRarityDropdown, function(sel)
    sellRaritySet = SetFromList(sel)
    if sellEnabled then PushAutoSellConfig() end
end)
SellSub:AddButton({
    Name = "Sell Inventory Now",
    Callback = safeCallback(function()
        task.spawn(function()
            local n = SellByRarityOnce()
            Notify("Sell", n > 0 and ("Sold " .. n .. " pet(s)") or "Nothing to sell", n > 0 and "Success" or "Info")
        end)
    end),
})

local EggSellSub = FarmTab:AddSubTab("Auto Sell Eggs")
EggSellSub:AddToggle({
    Name = "Enable Auto Sell Eggs",
    Default = false,
    Flag = "eggsell_enabled",
    Callback = safeCallback(function(v)
        eggSellEnabled = v
        notifyOn("Auto Sell Eggs", v)
        if v then safeSpawn(EggSellLoop) end
    end),
})
local eggSellRarityDropdown = EggSellSub:AddMultiDropdown({
    Name = "Egg Rarities to Sell",
    Options = RARITY_OPTIONS,
    Default = RARITY_OPTIONS,
    Flag = "eggsell_rarities",
    Callback = function(sel) eggSellRaritySet = SetFromList(sel) end,
})
registerResync(eggSellRarityDropdown, function(sel) eggSellRaritySet = SetFromList(sel) end)
EggSellSub:AddButton({
    Name = "Sell All Eggs Now",
    Callback = safeCallback(function()
        task.spawn(function()
            local n = SellEggsOnce()
            Notify("Sell Eggs", n > 0 and ("Sold " .. n .. " egg(s)") or "No eggs to sell", n > 0 and "Success" or "Info")
        end)
    end),
})

-- ── Tab 2: Claim ──────────────────────────────────────────────────────────────
local ClaimTab = Window:AddTab({ Name = "Claim", Subtitle = "Rewards & money", Icon = "gift" })

local ClaimSub = ClaimTab:AddSubTab("Auto Claim")
ClaimSub:AddToggle({
    Name = "Enable Auto Claim",
    Default = false,
    Flag = "claim_enabled",
    Callback = safeCallback(function(v)
        claimEnabled = v
        notifyOn("Auto Claim", v)
        if v then safeSpawn(ClaimLoop) end
    end),
})
ClaimSub:AddSlider({
    Name = "Claim Interval",
    Min = 10,
    Max = 300,
    Default = 30,
    Suffix = "s",
    Flag = "claim_interval",
    Callback = function(v) claimInterval = v end,
})
ClaimSub:AddButton({
    Name = "Claim Now",
    Callback = safeCallback(function()
        task.spawn(function()
            local n = ClaimOnce()
            Notify("Claim", n > 0 and ("Claimed " .. n .. " reward(s)") or "Nothing claimable", n > 0 and "Success" or "Info")
        end)
    end),
})

-- ── Tab 3: Upgrades ───────────────────────────────────────────────────────────
local UpTab = Window:AddTab({ Name = "Upgrades", Subtitle = "Base & slots", Icon = "wrench" })

local UpSub = UpTab:AddSubTab("Auto Upgrade")
UpSub:AddToggle({
    Name = "Enable Auto Upgrade",
    Default = false,
    Flag = "upgrade_enabled",
    Callback = safeCallback(function(v)
        upgradeEnabled = v
        notifyOn("Auto Upgrade", v)
        if v then safeSpawn(UpgradeLoop) end
    end),
})
UpSub:AddSlider({
    Name = "Upgrade Interval",
    Min = 10,
    Max = 300,
    Default = 15,
    Suffix = "s",
    Flag = "upgrade_interval",
    Callback = function(v) upgradeInterval = v end,
})
UpSub:AddButton({
    Name = "Upgrade Now",
    Callback = safeCallback(function()
        task.spawn(function()
            local n = UpgradeOnce()
            Notify("Upgrade", n > 0 and "Upgrade requested" or "Upgrade failed", n > 0 and "Success" or "Error")
        end)
    end),
})

local TmSub = UpTab:AddSubTab("Auto Treadmill")
TmSub:AddToggle({
    Name = "Enable Auto Treadmill",
    Default = false,
    Flag = "treadmill_enabled",
    Callback = safeCallback(function(v)
        treadmillEnabled = v
        notifyOn("Auto Treadmill", v)
        if v then safeSpawn(TreadmillLoop) end
    end),
})
TmSub:AddToggle({
    Name = "Auto-Upgrade Treadmill",
    Default = true,
    Flag = "treadmill_upgrade",
    Callback = function(v) treadmillAutoUpgrade = v end,
})
TmSub:AddButton({
    Name = "Go to Treadmill",
    Callback = safeCallback(function()
        task.spawn(function()
            local tb = FindMyTreadmillPart()
            if not tb then
                Notify("Treadmill", "Kein Treadmill gefunden", "Error", 2)
                return
            end
            WalkToPoint(tb.Position, math.min(GetLegalWalkSpeed(), 30), 3, 25)
            Notify("Treadmill", "Auf dem Treadmill", "Success", 2)
        end)
    end),
})

-- ── Tab: Visual ──────────────────────────────────────────────────────────────
local VisualTab = Window:AddTab({ Name = "Visual", Subtitle = "Fake pets (visual only)", Icon = "sparkle" })

local VisualSub = VisualTab:AddSubTab("Pet Spawner")

LoadVisualData()
PreloadAllPets()

local function GetCategoryWeight(cat)
    if not cat then return 100 end
    local cfg = nil
    local okCfg, AssetsDir = pcall(require, ReplicatedStorage:WaitForChild("Directory"):WaitForChild("Assets"))
    if okCfg then
        local dir = AssetsDir.Directory or AssetsDir.Assets or AssetsDir
        cfg = dir and dir[cat]
    end
    return cfg and (tonumber(cfg.ModelWeight) or 100) or 100
end

local visualPetDropdown, visualMutationDropdown, visualSizeSlider

visualPetDropdown = VisualSub:AddDropdown({
    Name = "Pet",
    Options = #VISUAL_PET_ORDER > 0 and VISUAL_PET_ORDER or { "Loading..." },
    Default = #VISUAL_PET_ORDER > 0 and VISUAL_PET_ORDER[1] or "Loading...",
    MaxVisible = 8,
    Searchable = true,
    Flag = "visual_pet",
    Callback = function(v)
        local cat = VISUAL_PET_BY_LABEL[v]
        if cat then
            if visualSizeSlider then
                pcall(visualSizeSlider.Set, visualSizeSlider, GetCategoryWeight(cat))
            end
            -- proactively fetch this pet's full model content in the background
            task.spawn(function() pcall(EnsurePetLoaded, cat, 20) end)
        end
    end,
})

visualMutationDropdown = VisualSub:AddDropdown({
    Name = "Mutation",
    Options = VISUAL_MUTATIONS,
    Default = "None",
    MaxVisible = 8,
    Searchable = true,
    Flag = "visual_mutation",
})

visualSizeSlider = VisualSub:AddSlider({
    Name = "Size (weight)",
    Min = 1,
    Max = 100000,
    Default = 100,
    Suffix = " kg",
    Flag = "visual_size",
})

-- sync the size slider to the default pet's natural weight so the first spawn
-- is full-size instead of 100kg (which shrinks heavy pets to 5%)
local defaultCat = VISUAL_PET_BY_LABEL[visualPetDropdown:Get()]
if defaultCat then
    pcall(visualSizeSlider.Set, visualSizeSlider, GetCategoryWeight(defaultCat))
end

local function GetVisualSelection()
    local label = visualPetDropdown and visualPetDropdown:Get() or nil
    local cat = VISUAL_PET_BY_LABEL[label]
    local mutation = visualMutationDropdown and visualMutationDropdown:Get() or "None"
    local kg = visualSizeSlider and visualSizeSlider:Get() or 100
    return cat, mutation, kg
end

VisualSub:AddButton({
    Name = "Add to Inventory",
    Callback = safeCallback(function()
        task.spawn(function()
            local cat, mutation, kg = GetVisualSelection()
            if not cat then
                Notify("Visual", "Pick a pet first", "Error", 3)
                return
            end
            if not preloadedPets[cat] then
                Notify("Visual", "Loading full model, please wait...", "Info", 60)
            end
            local ok, info = SpawnVisualPet(cat, mutation, kg)
            Notify("Visual", ok and ("Added " .. tostring(info) .. " to inventory") or tostring(info), ok and "Success" or "Error", 3)
        end)
    end),
})

VisualSub:AddButton({
    Name = "Equip Pet",
    Callback = safeCallback(function()
        task.spawn(function()
            local ok, info = EquipVisualPet()
            Notify("Visual", ok and ("Equipped " .. tostring(info)) or tostring(info), ok and "Success" or "Error", 3)
        end)
    end),
})

VisualSub:AddButton({
    Name = "Unequip Pet",
    Callback = safeCallback(function()
        UnequipVisualPet()
        Notify("Visual", "Pet unequipped (still in inventory)", "Info", 2)
    end),
})

VisualSub:AddButton({
    Name = "Remove All Pets",
    Callback = safeCallback(function()
        RemoveVisualPet()
        Notify("Visual", "All spawned pets removed", "Info", 2)
    end),
})

-- ── Tab 4: Info ───────────────────────────────────────────────────────────────
local InfoTab = Window:AddTab({ Name = "Info", Subtitle = "Status & config", Icon = "info" })

local InfoSub = InfoTab:AddSubTab("Status")
statusLabel = InfoSub:AddLabel({ Text = "Loading..." })
InfoSub:AddDivider()
InfoSub:AddButton({
    Name = "Save Config",
    Callback = function()
        if HAS_CONFIG then
            Library:SaveConfig(CONFIG_NAME)
            Notify("Config", "Saved!", "Success")
        end
    end,
})
InfoSub:AddButton({
    Name = "Load Config",
    Callback = function()
        if HAS_CONFIG then
            Library:LoadConfig(CONFIG_NAME)
            task.wait(0.1)
            ResyncAll()
            Notify("Config", "Loaded!", "Success")
        end
    end,
})
InfoSub:AddButton({
    Name = "Unload",
    Callback = safeCallback(function()
        stealEnabled = false
        hatchEnabled = false
        equipEnabled = false
        sellEnabled = false
        eggSellEnabled = false
        claimEnabled = false
        upgradeEnabled = false
        treadmillEnabled = false
        serverHopEnabled = false
        SetInfHp(false)
        SetStealTreadmillHidden(false)
        RemoveVisualPet()
        HUB.dead = true
        Notify("Oxide HUB", "Script unloaded", "Info")
        pcall(function() Window:Destroy() end)
    end),
})

-- ══════════════════════════════════════════════════════════════════════════
-- SERVER HOP — keep hopping until this server has the rarity you want
-- ══════════════════════════════════════════════════════════════════════════
-- Scans the area-egg snapshot for any stealable egg matching your settings.
-- With Rare Egg Hunter ON it requires tier >= your Rare Tier; otherwise any egg
-- inside the Egg Rarity Filter counts. Returns true (stay) on any failure so we
-- never hop because of a transient API hiccup.
local function ServerHasWantedEgg()
    if not ENV_OK or not NETMAP or not NETMAP.Eggs then return true end
    local ok, snap = pcall(Network.Invoke, NETMAP.Eggs.REQUEST_AREA_EGG_SNAPSHOT)
    if not (ok and type(snap) == "table" and type(snap.Records) == "table") then return true end
    for _, r in ipairs(snap.Records) do
        if r.State == "Slot" then
            local id = EggRarityId(r)
            if id and stealRaritySet[id] then
                if not rareHunterEnabled or EggRarityNumber(r) >= rareMinTier then
                    return true
                end
            end
        end
    end
    return false
end

local function HopServer()
    return pcall(function()
        TeleportService:Teleport(game.PlaceId, LocalPlayer)
    end)
end

-- ══════════════════════════════════════════════════════════════════════════════
-- LOOPS
-- ══════════════════════════════════════════════════════════════════════════════
function StealLoop()
    if HUB.loops.steal then return end
    HUB.loops.steal = true
    local ok, err = pcall(function()
        while stealEnabled and not HUB.dead do
            local sOk, info = StealOnce()
            if sOk then
                stealCount = stealCount + 1
            else
                task.wait(2)
            end
            task.wait(0.5)
        end
    end)
    HUB.loops.steal = false
    if not ok then
        Notify("Auto Steal", "Loop error: " .. tostring(err), "Error", 4)
    end
    if not stealEnabled and not HUB.dead then
        if HUB.cameraHb then HUB.cameraHb:Disconnect(); HUB.cameraHb = nil end
        DisableSwim()
    end
end

function ServerHopLoop()
    if HUB.loops.serverhop then return end
    HUB.loops.serverhop = true
    serverHopCount = 0
    while serverHopEnabled and not HUB.dead do
        task.wait(math.max(3, serverHopDelay))
        if not serverHopEnabled or HUB.dead then break end
        if ServerHasWantedEgg() then
            Notify("Server Hop", "Wanted rarity found — staying in this server", "Success", 4)
            break
        end
        if serverHopCount >= serverHopMax then
            Notify("Server Hop", "Stopped after " .. serverHopMax .. " hops (max reached)", "Error", 4)
            serverHopEnabled = false
            break
        end
        serverHopCount = serverHopCount + 1
        Notify("Server Hop", "No wanted rarity — hopping (" .. serverHopCount .. "/" .. serverHopMax .. ")", "Info", 5)
        -- Persist settings before leaving so the next server restores them.
        if HAS_CONFIG then pcall(function() Library:SaveConfig(CONFIG_NAME) end) end
        task.wait(2)
        if HopServer() then
            break  -- teleport in progress; the next server re-runs us via auto-execute
        end
        Notify("Server Hop", "Teleport failed — retrying", "Error", 4)
    end
    HUB.loops.serverhop = false
end

function HatchLoop()
    if HUB.loops.hatch then return end
    HUB.loops.hatch = true
    while hatchEnabled and not HUB.dead do
        local n = HatchOnce()
        hatchCount = hatchCount + n
        task.wait(3)
    end
    HUB.loops.hatch = false
end

function EquipLoop()
    if HUB.loops.equip then return end
    HUB.loops.equip = true
    while equipEnabled and not HUB.dead do
        if EquipBestOnce() then equipCount = equipCount + 1 end
        task.wait(5)
    end
    HUB.loops.equip = false
end

function SellLoop()
    if HUB.loops.sell then return end
    HUB.loops.sell = true
    while sellEnabled and not HUB.dead do
        local n = SellByRarityOnce()
        sellCount = sellCount + n
        task.wait(10)
    end
    HUB.loops.sell = false
end

function EggSellLoop()
    if HUB.loops.eggsell then return end
    HUB.loops.eggsell = true
    while eggSellEnabled and not HUB.dead do
        local n = SellEggsOnce()
        eggSellCount = eggSellCount + n
        task.wait(10)
    end
    HUB.loops.eggsell = false
end

function ClaimLoop()
    if HUB.loops.claim then return end
    HUB.loops.claim = true
    while claimEnabled and not HUB.dead do
        local n = ClaimOnce()
        claimCount = claimCount + n
        task.wait(math.max(5, claimInterval))
    end
    HUB.loops.claim = false
end

function UpgradeLoop()
    if HUB.loops.upgrade then return end
    HUB.loops.upgrade = true
    while upgradeEnabled and not HUB.dead do
        UpgradeOnce()
        task.wait(math.max(5, upgradeInterval))
    end
    HUB.loops.upgrade = false
end

-- Status refresher (always-on, lightweight)
track(task.spawn(function()
    while not HUB.dead do
        RefreshStatus()
        task.wait(2)
    end
end))

-- ══════════════════════════════════════════════════════════════════════════════
-- GAME API INIT (after UI so any failure only degrades engine, not the window)
-- ══════════════════════════════════════════════════════════════════════════════
InitGameApi()
if ENV_OK then
    Notify("Ein Ei stehlen", "Game API connected ✓", "Success", 3)
else
    Notify("Ein Ei stehlen", "Game API not found — UI only", "Error", 4)
end

-- ══════════════════════════════════════════════════════════════════════════════
-- CONFIG PERSISTENCE — restore saved settings on load + periodic autosave, so an
-- auto-executed script keeps every toggle (and keeps hopping) across servers.
-- ══════════════════════════════════════════════════════════════════════════════
if HAS_CONFIG then
    pcall(function() Library:LoadConfig(CONFIG_NAME) end)
    task.wait(0.2)
    ResyncAll()
end

-- Always steal every rarity unless the Rare Egg Hunter is on. The saved config
-- can restore a narrow filter (e.g. Secret+) which sends the loop after far,
-- guard-protected eggs — near eggs keep the cycles fast and death-free.
if not rareHunterEnabled then
    pcall(function() Library:SetFlag("steal_rarities", RARITY_OPTIONS) end)
    stealRaritySet = SetFromList(RARITY_OPTIONS)
end

task.spawn(function()
    while not HUB.dead do
        task.wait(60)
        if HAS_CONFIG and not HUB.dead then
            pcall(function() Library:SaveConfig(CONFIG_NAME) end)
        end
    end
end)

-- Rare spawn watcher: notify when the daily reset cycle presents rare eggs.
if ENV_OK and EggCmds then
    pcall(function()
        EggCmds.AreaEggRareSpawnsPresented:Connect(function()
            if rareHunterEnabled then
                Notify("Rare Egg Hunter", "Rare spawn presented — hunting!", "Success", 4)
            end
        end)
        EggCmds.AreaEggResetStartCountdown:Connect(function()
            if rareHunterEnabled then
                Notify("Rare Egg Hunter", "Area reset starting — rare eggs incoming", "Info", 4)
            end
        end)
        -- Cache the server's carry SpeedMultiplier (the game boosts WalkSpeed
        -- while carrying) so the return leg can tween at the boosted legal speed.
        if EggCmds.AreaEggCarryStateChanged then
            track(EggCmds.AreaEggCarryStateChanged:Connect(function(state)
                carryState = state  -- authoritative IsCarrying / Uid / SpeedMultiplier
                if state and type(state.SpeedMultiplier) == "number" and state.SpeedMultiplier > 0 then
                    carrySpeedMultiplier = state.SpeedMultiplier
                end
            end))
        end
    end)
end

function HUB.Unload()
    HUB.dead = true
    if HUB.cameraHb then HUB.cameraHb:Disconnect(); HUB.cameraHb = nil end
    stealEnabled = false
    hatchEnabled, equipEnabled = false, false
    sellEnabled, eggSellEnabled, claimEnabled, upgradeEnabled, treadmillEnabled = false, false, false, false, false
    serverHopEnabled = false
    SetInfHp(false)
    SetFakeDeath(false)
    SetStealTreadmillHidden(false)
    DisableSwim()
    RemoveVisualPet()
    for _, c in ipairs(HUB.conns) do pcall(function() c:Disconnect() end) end
    pcall(function() Window:Destroy() end)
    _G.OxideStealAnEgg = nil
end

print("[Oxide HUB] Ein Ei stehlen loaded. ENV_OK:", ENV_OK)
