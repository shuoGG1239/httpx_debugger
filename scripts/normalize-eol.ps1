# 项目文件规范化：UTF-8（无 BOM）+ LF 行尾，禁止 CRLF。
# 用法: pwsh scripts\normalize-eol.ps1
# 说明：跳过生成目录（node_modules/out/dist/data/.git）与二进制扩展名；
#       非 UTF-8 文件会告警并跳过，绝不盲目改写。
$root = Split-Path -Parent $PSScriptRoot
$excludeDir = @('node_modules', 'out', 'dist', 'data', '.git')
$binaryExt = @('.exe', '.dll', '.png', '.jpg', '.jpeg', '.gif', '.ico', '.woff', '.woff2', '.ttf', '.eot')

$files = Get-ChildItem $root -Recurse -File | Where-Object {
    $rel = $_.FullName.Substring($root.Length)
    $excluded = $false
    foreach ($d in $excludeDir) {
        if ($rel -match ('\\' + [regex]::Escape($d) + '\\')) { $excluded = $true; break }
    }
    -not $excluded -and $_.Extension.ToLower() -notin $binaryExt
}

$strictUtf8 = [System.Text.UTF8Encoding]::new($false, $true)
$changed = 0
$skipped = 0

foreach ($f in $files) {
    $rel = $f.FullName.Substring($root.Length)
    $bytes = [System.IO.File]::ReadAllBytes($f.FullName)

    # 去 UTF-8 BOM
    $hadBom = $false
    if ($bytes.Length -ge 3 -and $bytes[0] -eq 0xEF -and $bytes[1] -eq 0xBB -and $bytes[2] -eq 0xBF) {
        $hadBom = $true
        $bytes = [byte[]]($bytes[3..($bytes.Length - 1)])
    }

    # CRLF -> LF（孤立 CR 也转 LF）
    $out = New-Object 'System.Collections.Generic.List[byte]'
    for ($i = 0; $i -lt $bytes.Length; $i++) {
        $b = $bytes[$i]
        if ($b -eq 0x0D) {
            if ($i + 1 -lt $bytes.Length -and $bytes[$i + 1] -eq 0x0A) { continue }
            $out.Add([byte]0x0A)
            continue
        }
        $out.Add($b)
    }
    $newBytes = $out.ToArray()

    # 严格 UTF-8 校验，非法则跳过
    try {
        [void]$strictUtf8.GetString($newBytes)
    } catch {
        Write-Warning "非 UTF-8，跳过: $rel"
        $skipped++
        continue
    }

    $same = ($newBytes.Length -eq $bytes.Length)
    if ($same) {
        for ($i = 0; $i -lt $newBytes.Length; $i++) {
            if ($newBytes[$i] -ne $bytes[$i]) { $same = $false; break }
        }
    }

    if (-not $same) {
        [System.IO.File]::WriteAllBytes($f.FullName, $newBytes)
        $changed++
        Write-Output "fixed $rel"
    }
}

Write-Output "done: files=$($files.Count) changed=$changed skipped=$skipped"
