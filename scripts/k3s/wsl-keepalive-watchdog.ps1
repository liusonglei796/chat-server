<#
.SYNOPSIS
    WSL2 常驻会话的 watchdog：只要本进程活着，就保证发行版里至少有一个 wsl.exe 会话。

.DESCRIPTION
    循环执行 `wsl -d <distro> -u root -- sleep infinity`。
    该会话退出（被误杀 / 发行版重启）后自动重开，保证 WSL 不会因为
    「没有活跃会话」而发起 poweroff。
#>
param(
    [string]$Distro = 'Ubuntu-24.04'
)

$ErrorActionPreference = 'Continue'
Write-Host "[watchdog] 启动，发行版=$Distro"

while ($true) {
    try {
        & wsl.exe -d $Distro -u root -- sh -c 'exec sleep infinity'
    } catch {
        Write-Host "[watchdog] 会话异常退出: $_"
    }
    Write-Host "[watchdog] 会话已退出，2 秒后重开"
    Start-Sleep -Seconds 2
}
