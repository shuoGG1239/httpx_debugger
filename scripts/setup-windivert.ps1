# 下载并放置 WinDivert.dll / WinDivert.sys（全流量抓取所需）。
# 必须以「管理员身份」运行：pwsh 右键 → 以管理员身份运行，然后执行本脚本。
# 用法: pwsh scripts\setup-windivert.ps1
$ErrorActionPreference = 'Stop'
$root = Split-Path -Parent $PSScriptRoot
$dest = Join-Path $root 'core'
$ver = '2.2.2'
$url = "https://github.com/basil00/WinDivert/releases/download/v$ver/WinDivert-$ver-A.zip"
$tmp = Join-Path $env:TEMP 'windivert-setup'
$zip = Join-Path $tmp 'windivert.zip'

Write-Host "[1/3] 下载 WinDivert $ver ..." -ForegroundColor Cyan
if (-not (Test-Path $zip)) {
    try {
        Invoke-WebRequest -Uri $url -OutFile $zip -UseBasicParsing
    } catch {
        Write-Host "自动下载失败（可能需要代理/镜像），请手动下载:" -ForegroundColor Yellow
        Write-Host "  $url"
        Write-Host "解压后把 x64\WinDivert.dll 与 x64\WinDivert.sys 放到:" -ForegroundColor Yellow
        Write-Host "  $dest"
        exit 1
    }
}

Write-Host "[2/3] 解压并复制 DLL / 驱动 ..." -ForegroundColor Cyan
Expand-Archive -Path $zip -DestinationPath $tmp -Force
Copy-Item (Join-Path $tmp "x64\WinDivert.dll") $dest -Force
Copy-Item (Join-Path $tmp "x64\WinDivert.sys") $dest -Force

Write-Host "[3/3] 检查驱动可加载性 ..." -ForegroundColor Cyan
$isAdmin = ([Security.Principal.WindowsPrincipal][Security.Principal.WindowsIdentity]::GetCurrent()).IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
if (-not $isAdmin) {
    Write-Host "当前不是管理员：文件已就位，但驱动需管理员加载。" -ForegroundColor Yellow
    Write-Host "请用「以管理员身份运行」重新启动 HTTPX Debugger，再点「全流量」。" -ForegroundColor Yellow
    exit 0
}
# 预装驱动服务（WinDivert.dll 在 Open 时也会尝试自动加载；这里显式注册更稳）
sc.exe stop WinDivert 2>$null | Out-Null
sc.exe delete WinDivert 2>$null | Out-Null
sc.exe create WinDivert type= kernel binPath= "$dest\WinDivert.sys" | Out-Null
sc.exe start WinDivert | Out-Null
Start-Sleep -Milliseconds 500
$svc = Get-Service WinDivert -ErrorAction SilentlyContinue
if ($svc -and $svc.Status -eq 'Running') {
    Write-Host "驱动已加载：WinDivert 服务运行中。" -ForegroundColor Green
} else {
    Write-Host "驱动注册完成但未启动（状态: $($svc.Status)）。重启应用后再点「全流量」。" -ForegroundColor Yellow
}
