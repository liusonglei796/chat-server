<#
.SYNOPSIS
    WSL2 常驻守护（watchdog 版）：保持一个 wsl.exe 会话存活，VM 就不会被 WSL 回收。

.DESCRIPTION
    为什么需要：WSL 在最后一个 wsl.exe 会话退出后会调用 `systemctl poweroff`，
    而 k3s + containerd 的容器无法在 10 秒内干净退出，WSL 于是升级为硬重启
    （WSL ERROR: systemctl poweroff did not terminate the instance in 10000 ms,
    calling reboot(RB_POWER_OFF)），k3s / flannel / 所有 Pod 跟着反复重建。

    为什么用 watchdog：单条 `sleep infinity` 会话如果被误杀（比如有人批量杀 wsl.exe
    进程），VM 就会再次被回收。watchdog 会在会话退出后自动重开。
    watchdog 本身用 WMI (Win32_Process.Create) 启动，脱离调用方的进程树，
    不会被调用方的进程清理连带杀掉。

.EXAMPLE
    pwsh -File scripts/k3s/wsl-keepalive.ps1
    pwsh -File scripts/k3s/wsl-keepalive.ps1 -Stop
#>
param(
    [switch]$Stop,
    [int]$MemoryGB = 24,
    [int]$Processors = 8,
    [string]$Distro = 'Ubuntu-24.04'
)

$ErrorActionPreference = 'Stop'
$watchdog = Join-Path $PSScriptRoot 'wsl-keepalive-watchdog.ps1'

function Get-Watchdog {
    Get-CimInstance Win32_Process -Filter "Name='powershell.exe' OR Name='pwsh.exe'" -ErrorAction SilentlyContinue |
        Where-Object { $_.CommandLine -like '*wsl-keepalive-watchdog.ps1*' }
}

if ($Stop) {
    Get-Watchdog | ForEach-Object {
        Write-Host "停止 watchdog PID=$($_.ProcessId)"
        Stop-Process -Id $_.ProcessId -Force -ErrorAction SilentlyContinue
    }
    return
}

# .wslconfig：内存上限 + 不要因为空闲就回收 VM
$wslConfig = Join-Path $env:USERPROFILE '.wslconfig'
Write-Host "==> 写入 $wslConfig"
@"
[wsl2]
memory=${MemoryGB}GB
processors=${Processors}
vmIdleTimeout=2147483647

[experimental]
autoMemoryReclaim=gradual
"@ | Set-Content -Path $wslConfig -Encoding UTF8

if (Get-Watchdog) {
    Write-Host "==> watchdog 已在运行"
} else {
    Write-Host "==> 通过 WMI 启动 watchdog（脱离进程树）"
    $cmd = "powershell.exe -NoProfile -ExecutionPolicy Bypass -WindowStyle Hidden -File `"$watchdog`" -Distro $Distro"
    $res = Invoke-CimMethod -ClassName Win32_Process -MethodName Create -Arguments @{ CommandLine = $cmd }
    if ($res.ReturnValue -ne 0) {
        throw "启动 watchdog 失败，ReturnValue=$($res.ReturnValue)"
    }
    Write-Host "    watchdog PID=$($res.ProcessId)"
}

Write-Host "==> 校验 VM 在多次调用间保持稳定"
$uptimes = @()
for ($i = 0; $i -lt 3; $i++) {
    Start-Sleep -Seconds 12
    $u = (& wsl.exe -d $Distro -u root -- cat /proc/uptime) -split ' ' | Select-Object -First 1
    $uptimes += [double]$u
    Write-Host "    uptime[$i] = $u"
}
if ($uptimes[2] -le $uptimes[0]) {
    throw "VM uptime 未增长（$($uptimes -join ' -> ')），说明仍被回收"
}
$reboots = (& wsl.exe -d $Distro -u root -- bash -c "journalctl --no-pager -n 200 2>/dev/null | grep -c 'calling reboot(RB_POWER_OFF)' || true")
Write-Host "    最近 200 条日志中的硬重启次数: $reboots"
if ([int]$reboots -gt 0) {
    throw "检测到 WSL 硬重启，keep-alive 未生效"
}
Write-Host "KEEPALIVE_OK"
