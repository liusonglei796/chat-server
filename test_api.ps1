#!/usr/bin/env pwsh
# KamaChat API Test Script (PowerShell)
# 用于测试KamaChat所有核心API功能
# 使用方法: .\test_api.ps1

param(
    [string]$BaseUrl = "http://localhost:8000"
)

# 颜色输出函数
function Write-Success {
    param([string]$Message)
    Write-Host "✅ $Message" -ForegroundColor Green
}

function Write-Error {
    param([string]$Message)
    Write-Host "❌ $Message" -ForegroundColor Red
}

function Write-Info {
    param([string]$Message)
    Write-Host "ℹ️ $Message" -ForegroundColor Cyan
}

# 测试变量
$global:AccessToken = ""
$global:UserId = ""
$global:TestPhone = "13800138000"
$global:TestPassword = "Test123456"
$global:TestNickname = "TestUser"
$global:VerificationCode = ""

# 1. 发送短信验证码
function Test-SendSmsCode {
    Write-Info "测试1: 发送短信验证码"
    
    $body = @{
        telephone = $global:TestPhone
    } | ConvertTo-Json
    
    try {
        $response = Invoke-WebRequest -Uri "$BaseUrl/auth/sms-code" `
            -Method POST `
            -ContentType "application/json" `
            -Body $body
        
        $data = $response.Content | ConvertFrom-Json
        if ($data.code -eq 1000) {
            Write-Success "SMS验证码已发送到 $($global:TestPhone)"
            # 从Redis获取验证码 (仅用于测试)
            $redisCode = docker exec kamachat-redis redis-cli GET "auth:code:$($global:TestPhone)"
            $global:VerificationCode = $redisCode
            Write-Info "验证码: $($global:VerificationCode)"
            return $true
        } else {
            Write-Error "发送验证码失败: $($data.msg)"
            return $false
        }
    } catch {
        Write-Error "请求异常: $_"
        return $false
    }
}

# 2. 用户注册
function Test-Register {
    Write-Info "测试2: 用户注册"
    
    if ([string]::IsNullOrEmpty($global:VerificationCode)) {
        Write-Error "验证码不存在，请先执行SMS验证码测试"
        return $false
    }
    
    $body = @{
        telephone = $global:TestPhone
        password = $global:TestPassword
        nickname = $global:TestNickname
        sms_code = $global:VerificationCode
    } | ConvertTo-Json
    
    try {
        $response = Invoke-WebRequest -Uri "$BaseUrl/auth/register" `
            -Method POST `
            -ContentType "application/json" `
            -Body $body
        
        $data = $response.Content | ConvertFrom-Json
        if ($data.code -eq 1000) {
            $global:UserId = $data.data.uuid
            Write-Success "用户注册成功: $($data.data.nickname) (ID: $($global:UserId))"
            return $true
        } else {
            Write-Error "注册失败: $($data.msg)"
            return $false
        }
    } catch {
        Write-Error "请求异常: $_"
        return $false
    }
}

# 3. 用户登录
function Test-Login {
    Write-Info "测试3: 用户登录"
    
    $body = @{
        telephone = $global:TestPhone
        password = $global:TestPassword
    } | ConvertTo-Json
    
    try {
        $response = Invoke-WebRequest -Uri "$BaseUrl/auth/login" `
            -Method POST `
            -ContentType "application/json" `
            -Body $body
        
        $data = $response.Content | ConvertFrom-Json
        if ($data.code -eq 1000) {
            $global:AccessToken = $data.data.access_token
            Write-Success "登录成功，获得Access Token"
            Write-Info "Token (前32字符): $($global:AccessToken.Substring(0, 32))..."
            return $true
        } else {
            Write-Error "登录失败: $($data.msg)"
            return $false
        }
    } catch {
        Write-Error "请求异常: $_"
        return $false
    }
}

# 4. 获取用户信息
function Test-GetUserInfo {
    Write-Info "测试4: 获取用户信息"
    
    if ([string]::IsNullOrEmpty($global:AccessToken)) {
        Write-Error "Access Token不存在，请先执行登录测试"
        return $false
    }
    
    $headers = @{
        "Authorization" = "Bearer $($global:AccessToken)"
    }
    
    try {
        $response = Invoke-WebRequest -Uri "$BaseUrl/user/info" `
            -Method GET `
            -Headers $headers
        
        $data = $response.Content | ConvertFrom-Json
        if ($data.code -eq 1000) {
            Write-Success "获取用户信息成功"
            Write-Info "昵称: $($data.data.nickname), 电话: $($data.data.telephone), 状态: $($data.data.status)"
            return $true
        } else {
            Write-Error "获取用户信息失败: $($data.msg)"
            return $false
        }
    } catch {
        Write-Error "请求异常: $_"
        return $false
    }
}

# 5. AI智能回复建议
function Test-AiReplySuggestions {
    Write-Info "测试5: AI智能回复建议"
    
    if ([string]::IsNullOrEmpty($global:AccessToken)) {
        Write-Error "Access Token不存在，请先执行登录测试"
        return $false
    }
    
    $headers = @{
        "Authorization" = "Bearer $($global:AccessToken)"
    }
    
    $body = @{
        target_id = $global:UserId
        draft = "你好呀"
        style = "brief"
        context_limit = 5
    } | ConvertTo-Json
    
    try {
        $response = Invoke-WebRequest -Uri "$BaseUrl/ai/reply-suggestions" `
            -Method POST `
            -Headers $headers `
            -ContentType "application/json" `
            -Body $body
        
        $data = $response.Content | ConvertFrom-Json
        if ($data.code -eq 1000) {
            Write-Success "AI回复建议获取成功"
            Write-Info "建议: $($data.data.suggestions -join ', ')"
            return $true
        } else {
            Write-Error "AI回复建议获取失败: $($data.msg)"
            return $false
        }
    } catch {
        Write-Error "请求异常: $_"
        return $false
    }
}

# 6. 获取用户公开信息
function Test-GetPublicUserInfo {
    Write-Info "测试6: 获取用户公开信息"
    
    if ([string]::IsNullOrEmpty($global:UserId)) {
        Write-Error "用户ID不存在，请先执行用户注册测试"
        return $false
    }
    
    try {
        $response = Invoke-WebRequest -Uri "$BaseUrl/user/public-info?uuid=$($global:UserId)" `
            -Method GET
        
        $data = $response.Content | ConvertFrom-Json
        if ($data.code -eq 1000) {
            Write-Success "获取用户公开信息成功"
            Write-Info "昵称: $($data.data.nickname), 头像: $($data.data.avatar)"
            return $true
        } else {
            Write-Error "获取用户公开信息失败: $($data.msg)"
            return $false
        }
    } catch {
        Write-Error "请求异常: $_"
        return $false
    }
}

# 主测试流程
function Main {
    Write-Host "`n╔════════════════════════════════════════╗" -ForegroundColor Magenta
    Write-Host "║   KamaChat API 完整功能测试脚本        ║" -ForegroundColor Magenta
    Write-Host "╚════════════════════════════════════════╝`n" -ForegroundColor Magenta
    
    Write-Info "API服务地址: $BaseUrl"
    
    # 测试序列
    $tests = @(
        { Test-SendSmsCode },
        { Test-Register },
        { Test-Login },
        { Test-GetUserInfo },
        { Test-GetPublicUserInfo },
        { Test-AiReplySuggestions }
    )
    
    $passCount = 0
    $failCount = 0
    
    foreach ($test in $tests) {
        if (& $test) {
            $passCount++
        } else {
            $failCount++
        }
        Write-Host ""
    }
    
    # 测试总结
    Write-Host "`n╔════════════════════════════════════════╗" -ForegroundColor Magenta
    Write-Host "║              测试总结                 ║" -ForegroundColor Magenta
    Write-Host "╚════════════════════════════════════════╝`n" -ForegroundColor Magenta
    
    Write-Success "通过: $passCount"
    if ($failCount -gt 0) {
        Write-Error "失败: $failCount"
    }
    
    $total = $passCount + $failCount
    $passRate = [math]::Round(($passCount / $total) * 100, 2)
    Write-Info "通过率: $passRate%"
    
    if ($failCount -eq 0) {
        Write-Host "`n✅ 所有测试通过！项目可以投入使用。" -ForegroundColor Green
    } else {
        Write-Host "`n⚠️ 部分测试失败，请检查日志。" -ForegroundColor Yellow
    }
}

# 执行主函数
Main
