<#
.SYNOPSIS
    构建本地 k3s 验证环境所需的全部镜像。

.DESCRIPTION
    两段式构建（绕开本机没有 Docker 的限制）：
      1) 用宿主机的 Windows 版 Go 交叉编译 linux/amd64 静态二进制到 build/k3s/bin/
      2) 调用 WSL2 里的 nerdctl + buildkitd（复用 k3s 自带的 containerd / k8s.io 命名空间）
         把二进制打包成镜像，kubelet 可直接拉起。

.EXAMPLE
    pwsh -File scripts/k3s/build-images.ps1
    pwsh -File scripts/k3s/build-images.ps1 -Services friendship-service,chat-server
#>
[CmdletBinding()]
param(
    [string[]]$Services = @(),
    [switch]$SkipE2E
)

$ErrorActionPreference = 'Stop'

$root = Split-Path -Parent (Split-Path -Parent $PSScriptRoot)
Set-Location $root
$ctx = Join-Path $root 'build/k3s'

$all = [ordered]@{
    'chat-server'        = 'gateway'
    'user-service'       = 'user_service'
    'apply-service'      = 'apply_service'
    'friendship-service' = 'friendship_service'
    'group-service'      = 'group_service'
    'message-service'    = 'message_service'
    'cdc-service'        = 'cdc_service'
}

if ($Services.Count -gt 0) {
    foreach ($s in $Services) {
        if (-not $all.Contains($s)) { throw "未知服务: $s（可选: $($all.Keys -join ', ')）" }
    }
    $targets = [ordered]@{}
    foreach ($s in $Services) { $targets[$s] = $all[$s] }
} else {
    $targets = $all
}

Write-Host "==> 准备构建上下文 $ctx"
New-Item -ItemType Directory -Force -Path (Join-Path $ctx 'bin') | Out-Null
if (Test-Path (Join-Path $ctx 'web')) { Remove-Item (Join-Path $ctx 'web') -Recurse -Force }
Copy-Item (Join-Path $root 'web') (Join-Path $ctx 'web') -Recurse -Force

# ---- 1. 交叉编译 ----
$env:GOOS = 'linux'
$env:GOARCH = 'amd64'
$env:CGO_ENABLED = '0'
foreach ($name in $targets.Keys) {
    $pkg = "./cmd/$($targets[$name])"
    Write-Host "==> go build $name  ($pkg)"
    & go build -trimpath -o (Join-Path $ctx "bin/$name") $pkg
    if ($LASTEXITCODE -ne 0) { throw "交叉编译失败: $name" }
}

# ---- 2. 打包镜像 ----
# 构建上下文放在 WSL 本地盘（/root/k3s-build）而不是 /mnt/d：
# /mnt/d 是 9p 挂载，把 8 个二进制（约 370MB）整个作为构建上下文传给 buildkit
# 每次要几分钟；只传当前这一个二进制则快一个数量级。
foreach ($name in $targets.Keys) {
    Write-Host "==> nerdctl build kama-chat/${name}:latest"
    & wsl -d Ubuntu-24.04 -u root -- bash "/mnt/d/repo/chat-server/scripts/k3s/build-image.sh" $name "kama-chat/${name}:latest" Dockerfile.service
    if ($LASTEXITCODE -ne 0) { throw "镜像构建失败: $name" }
}

# ---- 3. e2e 测试镜像 ----
if (-not $SkipE2E) {
    Write-Host "==> go test -c ./tests/e2e"
    & go test -c -trimpath -o (Join-Path $ctx 'bin/e2e.test') ./tests/e2e
    if ($LASTEXITCODE -ne 0) { throw "e2e 测试二进制编译失败" }
    Write-Host "==> nerdctl build kama-chat/e2e:latest"
    & wsl -d Ubuntu-24.04 -u root -- bash "/mnt/d/repo/chat-server/scripts/k3s/build-image.sh" e2e.test "kama-chat/e2e:latest" Dockerfile.e2e
    if ($LASTEXITCODE -ne 0) { throw "e2e 镜像构建失败" }
}

Write-Host "==> 镜像列表"
& wsl -d Ubuntu-24.04 -u root -- k3s-nerdctl images
Write-Host "BUILD_IMAGES_OK"
