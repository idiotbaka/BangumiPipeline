[CmdletBinding()]
param(
  [string]$OutputDir = "apk",
  [string]$KeystorePath = "src-tauri/android-signing/bakavip2-release.jks",
  [string]$KeyAlias = "bakavip2",
  [switch]$SkipBuild
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

$repoRoot = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path
$tauriConfigPath = Join-Path $repoRoot "src-tauri/tauri.conf.json"
$signingEnvPath = Join-Path $repoRoot "src-tauri/android-signing/signing.env"
$resolvedOutputDir = Join-Path $repoRoot $OutputDir

function ConvertFrom-SecureText {
  param([securestring]$Value)
  $ptr = [Runtime.InteropServices.Marshal]::SecureStringToBSTR($Value)
  try {
    return [Runtime.InteropServices.Marshal]::PtrToStringBSTR($ptr)
  } finally {
    if ($ptr -ne [IntPtr]::Zero) {
      [Runtime.InteropServices.Marshal]::ZeroFreeBSTR($ptr)
    }
  }
}

function Import-SigningEnv {
  param([string]$Path)
  if (-not (Test-Path -LiteralPath $Path)) {
    return
  }

  foreach ($line in Get-Content -LiteralPath $Path) {
    if ($line -match "^\s*$" -or $line -match "^\s*#") {
      continue
    }
    $index = $line.IndexOf("=")
    if ($index -le 0) {
      continue
    }
    $name = $line.Substring(0, $index).Trim()
    $value = $line.Substring($index + 1)
    if ($name -and -not [Environment]::GetEnvironmentVariable($name, "Process")) {
      [Environment]::SetEnvironmentVariable($name, $value, "Process")
    }
  }
}

function Read-RequiredSecret {
  param(
    [string]$Name,
    [string]$Prompt
  )

  $value = [Environment]::GetEnvironmentVariable($Name, "Process")
  if ($value) {
    return $value
  }

  $secure = Read-Host -Prompt $Prompt -AsSecureString
  $plain = ConvertFrom-SecureText $secure
  if (-not $plain) {
    throw "$Name is required."
  }
  [Environment]::SetEnvironmentVariable($Name, $plain, "Process")
  return $plain
}

function Save-SigningEnv {
  param(
    [string]$Path,
    [string]$StorePassword,
    [string]$KeyPassword
  )

  $directory = Split-Path -Parent $Path
  New-Item -ItemType Directory -Force -Path $directory | Out-Null
  @(
    "# Local Android signing secrets. Do not commit this file.",
    "ANDROID_KEYSTORE_PATH=$KeystorePath",
    "ANDROID_KEY_ALIAS=$KeyAlias",
    "ANDROID_KEYSTORE_PASSWORD=$StorePassword",
    "ANDROID_KEY_PASSWORD=$KeyPassword"
  ) | Set-Content -LiteralPath $Path -Encoding UTF8
}

function Find-AndroidBuildTool {
  param(
    [string]$ToolName
  )

  $sdkRoot = $env:ANDROID_HOME
  if (-not $sdkRoot) {
    $sdkRoot = $env:ANDROID_SDK_ROOT
  }
  if (-not $sdkRoot) {
    $candidate = Join-Path $env:LOCALAPPDATA "Android/Sdk"
    if (Test-Path -LiteralPath $candidate) {
      $sdkRoot = $candidate
    }
  }
  if (-not $sdkRoot) {
    throw "Android SDK was not found. Set ANDROID_HOME or ANDROID_SDK_ROOT."
  }

  $buildToolsDir = Join-Path $sdkRoot "build-tools"
  if (-not (Test-Path -LiteralPath $buildToolsDir)) {
    throw "Android build-tools directory was not found: $buildToolsDir"
  }

  $versions = Get-ChildItem -LiteralPath $buildToolsDir -Directory |
    Sort-Object { try { [version]$_.Name } catch { [version]"0.0.0" } } -Descending

  foreach ($versionDir in $versions) {
    $toolPath = Join-Path $versionDir.FullName $ToolName
    if (Test-Path -LiteralPath $toolPath) {
      return $toolPath
    }
  }

  throw "$ToolName was not found in Android build-tools."
}

function Invoke-Checked {
  param(
    [string]$FilePath,
    [string[]]$Arguments
  )

  & $FilePath @Arguments
  if ($LASTEXITCODE -ne 0) {
    throw "Command failed with exit code $LASTEXITCODE`: $FilePath $($Arguments -join ' ')"
  }
}

function Get-ApkAbi {
  param([string]$Path)
  $normalized = $Path.ToLowerInvariant()
  if ($normalized -match "arm64|aarch64") { return "arm64-v8a" }
  if ($normalized -match "armeabi|armv7|[\\/_-]arm[\\/_-]") { return "armeabi-v7a" }
  if ($normalized -match "x86_64") { return "x86_64" }
  if ($normalized -match "i686|x86") { return "x86" }
  return "universal"
}

function ConvertTo-FileSafeName {
  param([string]$Value)
  return ($Value -replace "[^A-Za-z0-9._-]", "")
}

Import-SigningEnv $signingEnvPath

$envKeystorePath = [Environment]::GetEnvironmentVariable("ANDROID_KEYSTORE_PATH", "Process")
if ($envKeystorePath) {
  $KeystorePath = $envKeystorePath
}
$envKeyAlias = [Environment]::GetEnvironmentVariable("ANDROID_KEY_ALIAS", "Process")
if ($envKeyAlias) {
  $KeyAlias = $envKeyAlias
}
$resolvedKeystorePath = if ([System.IO.Path]::IsPathRooted($KeystorePath)) {
  $KeystorePath
} else {
  Join-Path $repoRoot $KeystorePath
}

$config = Get-Content -LiteralPath $tauriConfigPath -Raw | ConvertFrom-Json
$appName = ConvertTo-FileSafeName $config.productName
$version = ConvertTo-FileSafeName $config.version
if (-not $appName) { $appName = "BakaVip2" }
if (-not $version) { $version = "1.0.0" }

$zipalign = Find-AndroidBuildTool "zipalign.exe"
$apksigner = Find-AndroidBuildTool "apksigner.bat"
$keytoolCommand = Get-Command "keytool" -ErrorAction SilentlyContinue
if (-not $keytoolCommand) {
  throw "keytool was not found. Install JDK or add keytool to PATH."
}

$keystorePassword = Read-RequiredSecret "ANDROID_KEYSTORE_PASSWORD" "ANDROID_KEYSTORE_PASSWORD"
$keyPassword = Read-RequiredSecret "ANDROID_KEY_PASSWORD" "ANDROID_KEY_PASSWORD"

if (-not (Test-Path -LiteralPath $resolvedKeystorePath)) {
  Write-Host "Creating Android release keystore: $resolvedKeystorePath"
  New-Item -ItemType Directory -Force -Path (Split-Path -Parent $resolvedKeystorePath) | Out-Null
  Invoke-Checked $keytoolCommand.Source @(
    "-genkeypair",
    "-v",
    "-keystore", $resolvedKeystorePath,
    "-storetype", "JKS",
    "-alias", $KeyAlias,
    "-keyalg", "RSA",
    "-keysize", "2048",
    "-validity", "10000",
    "-storepass", $keystorePassword,
    "-keypass", $keyPassword,
    "-dname", "CN=BakaVip2, OU=BakaVip, O=BakaVip, L=Hong Kong, ST=Hong Kong, C=HK"
  )
  Save-SigningEnv $signingEnvPath $keystorePassword $keyPassword
} elseif (-not (Test-Path -LiteralPath $signingEnvPath)) {
  Save-SigningEnv $signingEnvPath $keystorePassword $keyPassword
}

if (-not $SkipBuild) {
  Push-Location $repoRoot
  try {
    Invoke-Checked "cargo" @(
      "tauri",
      "android",
      "build",
      "--apk",
      "--split-per-abi",
      "--target",
      "aarch64",
      "armv7",
      "i686",
      "x86_64",
      "--ci"
    )
  } finally {
    Pop-Location
  }
}

$apkRoot = Join-Path $repoRoot "src-tauri/gen/android/app/build/outputs/apk"
if (-not (Test-Path -LiteralPath $apkRoot)) {
  throw "APK output directory was not found: $apkRoot"
}

$sourceApks = Get-ChildItem -LiteralPath $apkRoot -Recurse -Filter "*.apk" |
  Where-Object {
    $_.FullName -match "[\\/]release[\\/]" -and
    $_.Name -notmatch "(^|-)aligned" -and
    $_.Name -notmatch "(^|-)signed\.apk$"
  } |
  Sort-Object FullName

if (-not $sourceApks) {
  throw "No release APKs were found under: $apkRoot"
}

$timestamp = Get-Date -Format "yyyyMMdd-HHmmss"
$releaseDir = Join-Path $resolvedOutputDir "$appName-$version-$timestamp"
$tempDir = Join-Path ([System.IO.Path]::GetTempPath()) "bakavip2-apk-sign-$timestamp"
New-Item -ItemType Directory -Force -Path $releaseDir | Out-Null
New-Item -ItemType Directory -Force -Path $tempDir | Out-Null

$signedApks = @()
try {
  foreach ($apk in $sourceApks) {
    $abi = Get-ApkAbi $apk.FullName
    $alignedApk = Join-Path $tempDir "$($apk.BaseName)-aligned.apk"
    $signedApk = Join-Path $releaseDir "$appName-$version-$abi.apk"

    Write-Host "Signing $abi APK..."
    Invoke-Checked $zipalign @("-p", "-f", "4", $apk.FullName, $alignedApk)
    Invoke-Checked $apksigner @(
      "sign",
      "--ks", $resolvedKeystorePath,
      "--ks-key-alias", $KeyAlias,
      "--ks-pass", "pass:$keystorePassword",
      "--key-pass", "pass:$keyPassword",
      "--out", $signedApk,
      $alignedApk
    )
    Invoke-Checked $apksigner @("verify", "--verbose", "--print-certs", $signedApk)
    $signedApks += Get-Item -LiteralPath $signedApk
  }
} finally {
  if (Test-Path -LiteralPath $tempDir) {
    Remove-Item -LiteralPath $tempDir -Recurse -Force
  }
}

Write-Host ""
Write-Host "Signed APKs:"
foreach ($apk in $signedApks) {
  Write-Host " - $($apk.FullName)"
}
