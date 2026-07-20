param(
    [string]$OutputDir = "",
    [string]$HttpProxy = "",
    [string]$HttpsProxy = "",
    [string]$UserAgent = "private-user/BangumiPipeline-smiles/0.1"
)

$ErrorActionPreference = "Stop"
$repoRoot = Split-Path -Parent $PSScriptRoot
if ([string]::IsNullOrWhiteSpace($OutputDir)) {
    $OutputDir = Join-Path $repoRoot "data\images\bangumi\smiles"
} elseif (-not [System.IO.Path]::IsPathRooted($OutputDir)) {
    $OutputDir = Join-Path $repoRoot $OutputDir
}
$OutputDir = [System.IO.Path]::GetFullPath($OutputDir)

$arguments = @(
    "run", "./cmd/sync-bangumi-smiles",
    "--output", $OutputDir,
    "--user-agent", $UserAgent
)
if (-not [string]::IsNullOrWhiteSpace($HttpProxy)) {
    $arguments += @("--http-proxy", $HttpProxy)
}
if (-not [string]::IsNullOrWhiteSpace($HttpsProxy)) {
    $arguments += @("--https-proxy", $HttpsProxy)
}

Push-Location (Join-Path $repoRoot "backend")
try {
    & go @arguments
    if ($LASTEXITCODE -ne 0) {
        throw "Bangumi 表情同步命令失败，退出码：$LASTEXITCODE"
    }
} finally {
    Pop-Location
}
