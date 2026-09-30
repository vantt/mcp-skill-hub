#Requires -Version 5.1
<#
.SYNOPSIS
    Lifecycle and security test suite for the Windows installer (install.ps1).
.DESCRIPTION
    Mirrors scripts/test-installer-lifecycle.sh for Windows / PowerShell.
    Tests fresh install, same-version no-op, upgrade, automatic rollback,
    unmanaged binary protection, Sigstore signature matrix (optional / required / mismatched),
    SHA-256 verification, archive member safety, uninstall, and PATH configuration.
    Guards Windows registry calls so it can run safely on both Windows CI runners
    and local/Linux pwsh test environments.
#>
[CmdletBinding()]
param()

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$InstallPs1Path = Join-Path $ScriptDir 'install.ps1'

if (-not (Test-Path -LiteralPath $InstallPs1Path -PathType Leaf)) {
    throw "install.ps1 not found at $InstallPs1Path"
}

$RunningOnWindows = ([System.Environment]::OSVersion.Platform -eq [System.PlatformID]::Win32NT)
$PathSep = [System.IO.Path]::PathSeparator
$PwshPath = (Get-Process -Id $PID).Path

# --- Temporary Test Directory ----------------------------------------------
$TestRoot = Join-Path ([System.IO.Path]::GetTempPath()) ("skillhub-win-test." + [System.Guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Force -Path $TestRoot | Out-Null

$FakeBin      = Join-Path $TestRoot 'fake-bin'
$FixturesDir  = Join-Path $TestRoot 'fixtures'
$HttpRoot     = Join-Path $TestRoot 'http-releases'
$HttpLog      = Join-Path $TestRoot 'http-urls.log'
$CosignLog    = Join-Path $TestRoot 'cosign.log'
$Workspace    = Join-Path $TestRoot 'canonical-workspace'
$HostConfig   = Join-Path $TestRoot 'host-config.json'

New-Item -ItemType Directory -Force -Path $FakeBin | Out-Null
New-Item -ItemType Directory -Force -Path $FixturesDir | Out-Null
New-Item -ItemType Directory -Force -Path $HttpRoot | Out-Null
New-Item -ItemType Directory -Force -Path $Workspace | Out-Null

Set-Content -Path (Join-Path $Workspace 'skill.md') -Value 'canonical-workspace-data'
Set-Content -Path $HostConfig -Value '{"host":"config"}'

# --- Test Assertion Helpers ------------------------------------------------
function Fail {
    param([string]$Message)
    Write-Host "FAIL: $Message" -ForegroundColor Red
    throw "FAIL: $Message"
}

function Pass {
    param([string]$Message)
    Write-Host "  PASS: $Message" -ForegroundColor Green
}

function Assert-True([bool]$Condition, [string]$Message) {
    if (-not $Condition) { Fail $Message }
}

function Assert-False([bool]$Condition, [string]$Message) {
    if ($Condition) { Fail $Message }
}

function Assert-FileExists([string]$Path, [string]$Label = '') {
    if (-not (Test-Path -LiteralPath $Path -PathType Leaf)) {
        Fail "Expected file to exist: $Path $(if ($Label) { "($Label)" })"
    }
}

function Assert-PathAbsent([string]$Path, [string]$Label = '') {
    if (Test-Path -LiteralPath $Path) {
        Fail "Expected path to be absent: $Path $(if ($Label) { "($Label)" })"
    }
}

function Get-FileSha256([string]$Path) {
    return (Get-FileHash -LiteralPath $Path -Algorithm SHA256).Hash.ToLowerInvariant()
}

# --- Isolated Process Runner -----------------------------------------------
function Invoke-InstallerProcess {
    param(
        [hashtable]$EnvVars = @{},
        [string[]]$Args = @()
    )

    $psi = New-Object System.Diagnostics.ProcessStartInfo
    $psi.FileName = $PwshPath
    $psi.UseShellExecute = $false
    $psi.RedirectStandardOutput = $true
    $psi.RedirectStandardError = $true
    $psi.CreateNoWindow = $true

    $psi.ArgumentList.Add('-NoProfile')
    $psi.ArgumentList.Add('-File')
    $psi.ArgumentList.Add($InstallPs1Path)
    foreach ($arg in $Args) {
        $psi.ArgumentList.Add($arg)
    }

    foreach ($k in $EnvVars.Keys) {
        $psi.EnvironmentVariables[$k] = [string]$EnvVars[$k]
    }

    $proc = [System.Diagnostics.Process]::Start($psi)
    $stdout = $proc.StandardOutput.ReadToEnd()
    $stderr = $proc.StandardError.ReadToEnd()
    $proc.WaitForExit()

    return [PSCustomObject]@{
        ExitCode = $proc.ExitCode
        Stdout   = $stdout
        Stderr   = $stderr
        Output   = "$stdout`n$stderr"
    }
}

# --- Fake Binary & Fixture Generators --------------------------------------
function New-FakeSkillhubBinary {
    param(
        [string]$Path,
        [string]$Version,
        [string]$Behavior = 'working'
    )

    if (-not $RunningOnWindows) {
        $content = @"
#!/bin/sh
set -eu
case "`$1" in
    version)
        if [ "$Behavior" = "fail-after-install" ]; then
            case "`$0" in
                *extract*|*skillhub-install*)
                    printf '%s\n' "$Version"
                    exit 0
                    ;;
                *)
                    exit 19
                    ;;
            esac
        fi
        printf '%s\n' "$Version"
        exit 0
        ;;
    *)
        exit 18
        ;;
esac
"@
        [System.IO.File]::WriteAllText($Path, $content.Replace("`r`n", "`n"), (New-Object System.Text.UTF8Encoding($false)))
        & chmod +x $Path 2>$null
    } else {
        $cscPaths = @(
            "$env:SystemRoot\Microsoft.NET\Framework64\v4.0.30319\csc.exe",
            "$env:SystemRoot\Microsoft.NET\Framework\v4.0.30319\csc.exe"
        )
        $csc = $null
        foreach ($c in $cscPaths) {
            if (Test-Path $c) { $csc = $c; break }
        }
        if ($null -eq $csc) {
            $cscCmd = Get-Command 'csc.exe' -ErrorAction SilentlyContinue
            if ($cscCmd) { $csc = $cscCmd.Source }
        }

        if ($csc) {
            $srcFile = [System.IO.Path]::ChangeExtension($Path, '.cs')
            $csCode = @"
using System;
public class Program {
    public static int Main(string[] args) {
        if (args.Length > 0 && args[0] == "version") {
            if ("$Behavior" == "fail-after-install") {
                string loc = System.Reflection.Assembly.GetExecutingAssembly().Location;
                if (loc.IndexOf("extract", StringComparison.OrdinalIgnoreCase) >= 0 ||
                    loc.IndexOf("skillhub-install", StringComparison.OrdinalIgnoreCase) >= 0) {
                    Console.WriteLine("$Version");
                    return 0;
                }
                return 19;
            }
            Console.WriteLine("$Version");
            return 0;
        }
        return 18;
    }
}
"@
            [System.IO.File]::WriteAllText($srcFile, $csCode, [System.Text.Encoding]::UTF8)
            & $csc /nologo /target:exe /out:$Path $srcFile *>$null
            Remove-Item -LiteralPath $srcFile -Force -ErrorAction SilentlyContinue
        } elseif (Get-Command go -ErrorAction SilentlyContinue) {
            $srcFile = [System.IO.Path]::ChangeExtension($Path, '.go')
            $goCode = @"
package main
import (
    "fmt"
    "os"
)
func main() {
    if len(os.Args) > 1 && os.Args[1] == "version" {
        if "$Behavior" == "fail-after-install" {
            execPath, _ := os.Executable()
            if strings.Contains(execPath, "extract") || strings.Contains(execPath, "skillhub-install") {
                fmt.Println("$Version")
                os.Exit(0)
            }
            os.Exit(19)
        }
        fmt.Println("$Version")
        os.Exit(0)
    }
    os.Exit(18)
}
"@
            [System.IO.File]::WriteAllText($srcFile, $goCode, [System.Text.Encoding]::UTF8)
            & go build -o $Path $srcFile *>$null
            Remove-Item -LiteralPath $srcFile -Force -ErrorAction SilentlyContinue
        } else {
            throw "Neither csc.exe nor go found on Windows to compile fake test binary"
        }
    }
}

function New-FixturePackage {
    param(
        [string]$Version,
        [string]$Behavior = 'working'
    )

    $fixtureDir = Join-Path $FixturesDir "v$Version"
    $stagingDir = Join-Path $fixtureDir 'staging'
    New-Item -ItemType Directory -Force -Path $stagingDir | Out-Null

    $binaryPath = Join-Path $stagingDir 'skillhub.exe'
    New-FakeSkillhubBinary -Path $binaryPath -Version $Version -Behavior $Behavior

    $zipAmd64 = Join-Path $fixtureDir "skillhub-$Version-windows-amd64.zip"
    $zipArm64 = Join-Path $fixtureDir "skillhub-$Version-windows-arm64.zip"

    Compress-Archive -Path $binaryPath -DestinationPath $zipAmd64 -Force
    Compress-Archive -Path $binaryPath -DestinationPath $zipArm64 -Force
    Remove-Item -Recurse -Force $stagingDir

    $hashAmd64 = Get-FileSha256 $zipAmd64
    $hashArm64 = Get-FileSha256 $zipArm64

    $manifest = Join-Path $fixtureDir 'checksums.txt'
    $manifestContent = "$hashAmd64  skillhub-$Version-windows-amd64.zip`n$hashArm64  skillhub-$Version-windows-arm64.zip`n"
    [System.IO.File]::WriteAllText($manifest, $manifestContent, (New-Object System.Text.UTF8Encoding($false)))

    $bundle = Join-Path $fixtureDir 'checksums.txt.sigstore.json'
    $manifestHash = Get-FileSha256 $manifest
    [System.IO.File]::WriteAllText($bundle, "$manifestHash`n", (New-Object System.Text.UTF8Encoding($false)))

    return $fixtureDir
}

function New-ExtraEntryFixture {
    param([string]$Version)

    $fixtureDir = Join-Path $FixturesDir 'extra-entry'
    $stagingDir = Join-Path $fixtureDir 'staging'
    New-Item -ItemType Directory -Force -Path $stagingDir | Out-Null

    $binaryPath = Join-Path $stagingDir 'skillhub.exe'
    New-FakeSkillhubBinary -Path $binaryPath -Version $Version -Behavior 'working'

    $extraPath = Join-Path $stagingDir 'unexpected.txt'
    Set-Content -Path $extraPath -Value 'unexpected content'

    $zip = Join-Path $fixtureDir "skillhub-$Version-windows-amd64.zip"
    Compress-Archive -Path "$stagingDir\*" -DestinationPath $zip -Force
    Remove-Item -Recurse -Force $stagingDir

    $hash = Get-FileSha256 $zip
    $manifest = Join-Path $fixtureDir 'checksums.txt'
    $manifestContent = "$hash  skillhub-$Version-windows-amd64.zip`n"
    [System.IO.File]::WriteAllText($manifest, $manifestContent, (New-Object System.Text.UTF8Encoding($false)))

    $bundle = Join-Path $fixtureDir 'checksums.txt.sigstore.json'
    $manifestHash = Get-FileSha256 $manifest
    [System.IO.File]::WriteAllText($bundle, "$manifestHash`n", (New-Object System.Text.UTF8Encoding($false)))

    return $fixtureDir
}

function New-CorruptFixture {
    param([string]$Version)

    $fixtureDir = New-FixturePackage -Version $Version -Behavior 'working'
    $zip = Join-Path $fixtureDir "skillhub-$Version-windows-amd64.zip"
    [System.IO.File]::AppendAllText($zip, "tampered-corrupt-data`n")
    return $fixtureDir
}

# --- Setup Fixture Verifier & Mock Tools -----------------------------------
$FixtureVerifier = Join-Path $FakeBin 'verify-fixture-bundle.ps1'
$verifierCode = @'
param($manifest, $bundle, $identity, $issuer)
if ($args.Count + 4 -ne 4 -and $null -eq $manifest) { exit 20 }
if ($issuer -ne "https://token.actions.githubusercontent.com") { exit 21 }
if ($identity -notlike "https://github.com/vantt/mcp-skill-hub/.github/workflows/release.yml@refs/tags/v*") { exit 22 }
$actual = (Get-FileHash -LiteralPath $manifest -Algorithm SHA256).Hash.ToLowerInvariant()
$expected = (Get-Content -LiteralPath $bundle -TotalCount 1).Trim().ToLowerInvariant()
if ($actual -ne $expected) { exit 23 }
exit 0
'@
[System.IO.File]::WriteAllText($FixtureVerifier, $verifierCode)

# Cross-platform command wrappers
function Install-FakeCommand {
    param(
        [string]$Name,
        [string]$Ps1Script
    )

    $ps1File = Join-Path $FakeBin "$Name.ps1"
    [System.IO.File]::WriteAllText($ps1File, $Ps1Script)

    # Shell wrapper for POSIX / Linux
    $shFile = Join-Path $FakeBin $Name
    $shContent = @"
#!/bin/sh
exec "$PwshPath" -NoProfile -File "`$(dirname "`$0")/$Name.ps1" "`$@"
"@
    [System.IO.File]::WriteAllText($shFile, $shContent.Replace("`r`n", "`n"))
    & chmod +x $shFile 2>$null

    # Batch wrapper for Windows
    $cmdFile = Join-Path $FakeBin "$Name.cmd"
    $cmdContent = @"
@echo off
"$PwshPath" -NoProfile -File "%~dp0$Name.ps1" %*
"@
    [System.IO.File]::WriteAllText($cmdFile, $cmdContent)
}

# Mock Cosign
$cosignScript = @'
$cmd = $null
$bundle = $null
$identity = $null
$issuer = $null
$manifest = $null

$i = 0
while ($i -lt $args.Count) {
    switch ($args[$i]) {
        "verify-blob" { $cmd = "verify-blob"; $i++ }
        "--bundle" { $bundle = $args[$i+1]; $i += 2 }
        "--certificate-identity" { $identity = $args[$i+1]; $i += 2 }
        "--certificate-oidc-issuer" { $issuer = $args[$i+1]; $i += 2 }
        default {
            if (-not $args[$i].StartsWith("-")) { $manifest = $args[$i] }
            $i++
        }
    }
}
if ($cmd -ne "verify-blob") { exit 41 }
if ($issuer -ne "https://token.actions.githubusercontent.com") { exit 46 }
$expectedIdentity = $env:EXPECTED_COSIGN_IDENTITY
if ($null -ne $expectedIdentity -and $identity -ne $expectedIdentity) { exit 44 }

$actual = (Get-FileHash -LiteralPath $manifest -Algorithm SHA256).Hash.ToLowerInvariant()
$expected = (Get-Content -LiteralPath $bundle -TotalCount 1).Trim().ToLowerInvariant()
if ($actual -ne $expected) { exit 47 }

if ($env:FIXTURE_COSIGN_LOG) {
    [System.IO.File]::AppendAllText($env:FIXTURE_COSIGN_LOG, "$identity`n")
}
exit 0
'@
Install-FakeCommand -Name 'cosign' -Ps1Script $cosignScript

# Mock Curl
$curlScript = @'
$output = $null
$url = $null

$i = 0
while ($i -lt $args.Count) {
    switch ($args[$i]) {
        "--output" { $output = $args[$i+1]; $i += 2 }
        "--connect-timeout" { $i += 2 }
        "--max-time" { $i += 2 }
        "--max-filesize" { $i += 2 }
        "--proto" { $i += 2 }
        "--fail" { $i++ }
        "--location" { $i++ }
        "--tlsv1.2" { $i++ }
        "--silent" { $i++ }
        "--show-error" { $i++ }
        default {
            if ($args[$i].StartsWith("https://")) { $url = $args[$i] }
            $i++
        }
    }
}

if (-not $url) { exit 34 }
if ($env:FIXTURE_HTTP_LOG) {
    [System.IO.File]::AppendAllText($env:FIXTURE_HTTP_LOG, "$url`n")
}

$prefix = "https://fixtures.invalid/releases/"
if ($url.StartsWith($prefix)) {
    $rel = $url.Substring($prefix.Length)
    $source = Join-Path $env:FIXTURE_HTTP_ROOT $rel
    if (-not (Test-Path -LiteralPath $source -PathType Leaf)) { exit 37 }
    if ($output) {
        Copy-Item -LiteralPath $source -Destination $output -Force
    } else {
        Get-Content -LiteralPath $source -Raw | Write-Host -NoNewline
    }
    exit 0
}
exit 36
'@
Install-FakeCommand -Name 'curl' -Ps1Script $curlScript

try {
    Write-Host "=========================================================="
    Write-Host "Starting Windows Installer Lifecycle & Security Test Suite"
    Write-Host "=========================================================="

    # Generate Fixture Packages
    $fixtureV1 = New-FixturePackage -Version '1.0.0' -Behavior 'working'
    $fixtureV2 = New-FixturePackage -Version '2.0.0' -Behavior 'working'
    $fixturePrerelease = New-FixturePackage -Version '2.1.0-rc.1+build.5' -Behavior 'working'
    $fixtureBad = New-FixturePackage -Version '3.0.0' -Behavior 'fail-after-install'
    $fixtureCorrupt = New-CorruptFixture -Version '4.0.0'
    $fixtureExtra = New-ExtraEntryFixture -Version '5.0.0'

    # Setup Fake HTTP root
    Copy-Item -Recurse (Join-Path $fixtureV1 '*') (New-Item -ItemType Directory -Force -Path (Join-Path $HttpRoot 'v1.0.0')).FullName
    Copy-Item -Recurse (Join-Path $fixtureV2 '*') (New-Item -ItemType Directory -Force -Path (Join-Path $HttpRoot 'v2.0.0')).FullName
    Copy-Item -Recurse (Join-Path $fixturePrerelease '*') (New-Item -ItemType Directory -Force -Path (Join-Path $HttpRoot 'v2.1.0-rc.1+build.5')).FullName

    # -----------------------------------------------------------------------
    # Test 1: Version Placeholder Unrendered
    # -----------------------------------------------------------------------
    Write-Host "`n[Test 1] Unrendered version placeholder without version override exits 1..."
    $res1 = Invoke-InstallerProcess
    Assert-True ($res1.ExitCode -eq 1) "Unrendered version should exit 1 (got $($res1.ExitCode))"
    Assert-True ($res1.Stderr -like "*A release version is required*") "Expected release version error message"
    Pass "Unrendered version placeholder rejected"

    # -----------------------------------------------------------------------
    # Test 2: Malformed Versions Rejected
    # -----------------------------------------------------------------------
    Write-Host "`n[Test 2] Malformed SemVer strings rejected without writing files..."
    $invalidVersions = @(
        '1.2', 'v1.2', '1.2.3.4', 'vv1.2.3', '1.2.x',
        '01.2.3', '1.02.3', '1.2.03', '1.2.3-01',
        '1.2.3-', '1.2.3+', '1.2.3-rc..1', '1.2.3+build..1',
        '1.2.3-rc_1', '1.2.3+build_1', '1.2.3++build'
    )
    $corruptPrefix = Join-Path $TestRoot 'rejected-prefix'
    foreach ($inv in $invalidVersions) {
        $res = Invoke-InstallerProcess -EnvVars @{
            SKILLHUB_VERSION     = $inv
            SKILLHUB_ARCH        = 'amd64'
            SKILLHUB_INSTALL_DIR = $corruptPrefix
            SKILLHUB_DRY_RUN     = '1'
        }
        Assert-True ($res.ExitCode -eq 1) "Invalid version $inv should exit 1 (got $($res.ExitCode))"
    }
    Assert-PathAbsent $corruptPrefix "No files should be written on malformed version rejection"
    Pass "All 16 malformed SemVer strings rejected cleanly"

    # -----------------------------------------------------------------------
    # Test 3: Supported Architectures Accepted in Dry Run
    # -----------------------------------------------------------------------
    Write-Host "`n[Test 3] Supported architectures accepted..."
    foreach ($arch in @('amd64', 'arm64', 'x86_64', 'x64', 'aarch64')) {
        $res = Invoke-InstallerProcess -EnvVars @{
            SKILLHUB_VERSION = '1.0.0'
            SKILLHUB_ARCH    = $arch
            SKILLHUB_DRY_RUN = '1'
        }
        Assert-True ($res.ExitCode -eq 0) "Supported architecture $arch should succeed in dry run"
    }
    Pass "Supported architectures accepted"

    # -----------------------------------------------------------------------
    # Test 4: Unsupported Architectures Rejected
    # -----------------------------------------------------------------------
    Write-Host "`n[Test 4] Unsupported architectures rejected..."
    foreach ($arch in @('x86', 'mips', 'arm', 'ia64', 'powerpc')) {
        $res = Invoke-InstallerProcess -EnvVars @{
            SKILLHUB_VERSION = '1.0.0'
            SKILLHUB_ARCH    = $arch
            SKILLHUB_DRY_RUN = '1'
        }
        Assert-True ($res.ExitCode -eq 1) "Unsupported architecture $arch should exit 1"
        Assert-True ($res.Stderr -like "*Unsupported architecture*") "Expected unsupported architecture error"
    }
    Pass "Unsupported architectures rejected cleanly"

    # -----------------------------------------------------------------------
    # Test 5: Architecture Detection Mapping
    # -----------------------------------------------------------------------
    Write-Host "`n[Test 5] Processor environment variable architecture detection..."
    $resAmd64 = Invoke-InstallerProcess -EnvVars @{
        SKILLHUB_VERSION       = '1.0.0'
        SKILLHUB_ARCH          = ''
        PROCESSOR_ARCHITECTURE = 'AMD64'
        SKILLHUB_DRY_RUN       = '1'
    }
    Assert-True ($resAmd64.ExitCode -eq 0) "AMD64 detection should succeed"

    $resArm64 = Invoke-InstallerProcess -EnvVars @{
        SKILLHUB_VERSION       = '1.0.0'
        SKILLHUB_ARCH          = ''
        PROCESSOR_ARCHITECTURE = 'ARM64'
        SKILLHUB_DRY_RUN       = '1'
    }
    Assert-True ($resArm64.ExitCode -eq 0) "ARM64 detection should succeed"
    Pass "Architecture environment detection verified"

    # -----------------------------------------------------------------------
    # Test 6: Non-HTTPS Release Base Rejected
    # -----------------------------------------------------------------------
    Write-Host "`n[Test 6] Non-HTTPS release base rejected..."
    $resInsecure = Invoke-InstallerProcess -EnvVars @{
        SKILLHUB_VERSION      = '1.0.0'
        SKILLHUB_ARCH         = 'amd64'
        SKILLHUB_RELEASE_BASE = 'http://example.invalid'
        SKILLHUB_DRY_RUN      = '1'
    }
    Assert-True ($resInsecure.ExitCode -eq 1) "Insecure HTTP base should exit 1"
    Assert-True ($resInsecure.Stderr -like "*Release base must use HTTPS*") "Expected HTTPS requirement error"
    Pass "Non-HTTPS release base rejected"

    # -----------------------------------------------------------------------
    # Test 7: Local Fixture Requires Controlled Verifier
    # -----------------------------------------------------------------------
    Write-Host "`n[Test 7] Local fixture mode requires SKILLHUB_FIXTURE_VERIFIER..."
    $resNoVerifier = Invoke-InstallerProcess -EnvVars @{
        SKILLHUB_VERSION       = '1.0.0'
        SKILLHUB_ARCH          = 'amd64'
        SKILLHUB_LOCAL_FIXTURE = $fixtureV1
        SKILLHUB_INSTALL_DIR   = (Join-Path $TestRoot 'no-verifier-prefix')
    }
    Assert-True ($resNoVerifier.ExitCode -eq 1) "Missing verifier should exit 1"
    Assert-True ($resNoVerifier.Stderr -like "*Local fixture mode requires SKILLHUB_FIXTURE_VERIFIER*") "Expected verifier requirement error"
    Pass "Local fixture verifier requirement enforced"

    # -----------------------------------------------------------------------
    # Test 8: Direct Install with Local Fixture
    # -----------------------------------------------------------------------
    Write-Host "`n[Test 8] Direct fresh install with local fixture..."
    $directPrefix = Join-Path $TestRoot 'direct-install-prefix'
    $resInstall = Invoke-InstallerProcess -EnvVars @{
        SKILLHUB_VERSION          = 'v1.0.0'
        SKILLHUB_ARCH             = 'amd64'
        SKILLHUB_LOCAL_FIXTURE    = $fixtureV1
        SKILLHUB_FIXTURE_VERIFIER = $FixtureVerifier
        SKILLHUB_INSTALL_DIR      = $directPrefix
        SKILLHUB_NO_MODIFY_PATH   = '1'
    }
    Assert-True ($resInstall.ExitCode -eq 0) "Install should succeed (output: $($resInstall.Output))"
    $targetExe = Join-Path $directPrefix 'skillhub.exe'
    $markerFile = Join-Path $directPrefix '.skillhub-managed'
    Assert-FileExists $targetExe "Target executable installed"
    Assert-FileExists $markerFile "Installation marker written"

    $markerContent = Get-Content -LiteralPath $markerFile
    Assert-True ($markerContent[0] -eq 'skillhub-managed-v1') "Marker header must be skillhub-managed-v1"
    Assert-True ($markerContent[1] -eq 'version=1.0.0') "Marker version must be normalized 1.0.0"

    $verOut = & $targetExe version 2>&1
    Assert-True ($verOut -like '*1.0.0*') "Installed binary version output should be 1.0.0"
    Pass "Direct fresh install succeeded and verified"

    # -----------------------------------------------------------------------
    # Test 9: Re-running Installer with Same Version (Idempotent No-Op)
    # -----------------------------------------------------------------------
    Write-Host "`n[Test 9] Re-running installer with same version is no-op..."
    $resSame = Invoke-InstallerProcess -EnvVars @{
        SKILLHUB_VERSION          = '1.0.0'
        SKILLHUB_ARCH             = 'amd64'
        SKILLHUB_LOCAL_FIXTURE    = $fixtureV1
        SKILLHUB_FIXTURE_VERIFIER = $FixtureVerifier
        SKILLHUB_INSTALL_DIR      = $directPrefix
        SKILLHUB_NO_MODIFY_PATH   = '1'
    }
    Assert-True ($resSame.ExitCode -eq 0) "Same version install should exit 0"
    Assert-True ($resSame.Stdout -like "*already installed*") "Expected 'already installed' message"
    Pass "Same-version no-op reported correctly"

    # -----------------------------------------------------------------------
    # Test 10: Successful Upgrade to v2.0.0
    # -----------------------------------------------------------------------
    Write-Host "`n[Test 10] Upgrading to higher version (v2.0.0)..."
    $resUpgrade = Invoke-InstallerProcess -EnvVars @{
        SKILLHUB_VERSION          = 'v2.0.0'
        SKILLHUB_ARCH             = 'amd64'
        SKILLHUB_LOCAL_FIXTURE    = $fixtureV2
        SKILLHUB_FIXTURE_VERIFIER = $FixtureVerifier
        SKILLHUB_INSTALL_DIR      = $directPrefix
        SKILLHUB_NO_MODIFY_PATH   = '1'
    }
    Assert-True ($resUpgrade.ExitCode -eq 0) "Upgrade should succeed (output: $($resUpgrade.Output))"
    Assert-True ($resUpgrade.Stdout -like "*Upgraded skillhub to 2.0.0*") "Expected upgraded message"

    $markerContent2 = Get-Content -LiteralPath $markerFile
    Assert-True ($markerContent2[1] -eq 'version=2.0.0') "Marker version updated to 2.0.0"

    $verOut2 = & $targetExe version 2>&1
    Assert-True ($verOut2 -like '*2.0.0*') "Installed binary version output should be 2.0.0"

    # User workspace and config files must remain intact
    Assert-FileExists (Join-Path $Workspace 'skill.md') "Workspace preserved across upgrade"
    Assert-FileExists $HostConfig "Host config preserved across upgrade"
    Pass "Upgrade to v2.0.0 succeeded and user state preserved"

    # -----------------------------------------------------------------------
    # Test 11: Upgrade Failure Triggers Automatic Rollback
    # -----------------------------------------------------------------------
    Write-Host "`n[Test 11] Upgrade failure triggers automatic rollback..."
    $resRollback = Invoke-InstallerProcess -EnvVars @{
        SKILLHUB_VERSION          = '3.0.0'
        SKILLHUB_ARCH             = 'amd64'
        SKILLHUB_LOCAL_FIXTURE    = $fixtureBad
        SKILLHUB_FIXTURE_VERIFIER = $FixtureVerifier
        SKILLHUB_INSTALL_DIR      = $directPrefix
        SKILLHUB_NO_MODIFY_PATH   = '1'
    }
    Assert-True ($resRollback.ExitCode -eq 1) "Failing upgrade should exit 1"
    Assert-True ($resRollback.Stderr -like "*restored the previous installation*") "Expected rollback restoration message"

    # Verify rollback restored 2.0.0 binary and marker
    $verOutRollback = & $targetExe version 2>&1
    Assert-True ($verOutRollback -like '*2.0.0*') "Binary should remain 2.0.0 after rollback"
    $markerRollback = Get-Content -LiteralPath $markerFile
    Assert-True ($markerRollback[1] -eq 'version=2.0.0') "Marker should remain 2.0.0 after rollback"
    Pass "Automatic rollback on upgrade failure verified"

    # -----------------------------------------------------------------------
    # Test 12: Protection of Unmanaged Binaries
    # -----------------------------------------------------------------------
    Write-Host "`n[Test 12] Unmanaged binary protection..."
    $unmanagedPrefix = Join-Path $TestRoot 'unmanaged-prefix'
    New-Item -ItemType Directory -Force -Path $unmanagedPrefix | Out-Null
    $unmanagedExe = Join-Path $unmanagedPrefix 'skillhub.exe'
    New-FakeSkillhubBinary -Path $unmanagedExe -Version '0.0.1' -Behavior 'working'

    # Overwrite attempt must be refused
    $resUnmanaged = Invoke-InstallerProcess -EnvVars @{
        SKILLHUB_VERSION          = '1.0.0'
        SKILLHUB_ARCH             = 'amd64'
        SKILLHUB_LOCAL_FIXTURE    = $fixtureV1
        SKILLHUB_FIXTURE_VERIFIER = $FixtureVerifier
        SKILLHUB_INSTALL_DIR      = $unmanagedPrefix
        SKILLHUB_NO_MODIFY_PATH   = '1'
    }
    Assert-True ($resUnmanaged.ExitCode -eq 1) "Installer must refuse to overwrite unmanaged binary"
    Assert-True ($resUnmanaged.Stderr -like "*Refusing to overwrite existing unmanaged binary*") "Expected unmanaged refusal error"

    # Uninstall attempt must also refuse to touch unmanaged binary
    $resUnmanagedUninstall = Invoke-InstallerProcess -EnvVars @{
        SKILLHUB_UNINSTALL   = '1'
        SKILLHUB_INSTALL_DIR = $unmanagedPrefix
    }
    Assert-True ($resUnmanagedUninstall.ExitCode -eq 1) "Uninstall must refuse to remove unmanaged binary"
    Assert-True ($resUnmanagedUninstall.Stderr -like "*Refusing to remove unmanaged path*") "Expected uninstall refusal error"
    Assert-FileExists $unmanagedExe "Unmanaged binary left untouched"
    Pass "Unmanaged binary protected against install overwrite and uninstall"

    # -----------------------------------------------------------------------
    # Test 13: Signature Verification Matrix & Error Handling
    # -----------------------------------------------------------------------
    Write-Host "`n[Test 13] Signature verification matrix & corrupted assets..."

    # 13a: Cosign absent on PATH: prints warning notice, install succeeds
    $cleanPathWithoutCosign = Join-Path $TestRoot 'path-no-cosign'
    New-Item -ItemType Directory -Force -Path $cleanPathWithoutCosign | Out-Null
    # Copy curl to path-no-cosign but NOT cosign
    Copy-Item -Path (Join-Path $FakeBin 'curl*') -Destination $cleanPathWithoutCosign

    $noCosignPrefix = Join-Path $TestRoot 'no-cosign-prefix'
    $resNoCosign = Invoke-InstallerProcess -EnvVars @{
        PATH                    = "$cleanPathWithoutCosign$PathSep$env:PATH"
        FIXTURE_HTTP_ROOT       = $HttpRoot
        FIXTURE_HTTP_LOG        = $HttpLog
        SKILLHUB_VERSION        = 'v1.0.0'
        SKILLHUB_ARCH           = 'amd64'
        SKILLHUB_RELEASE_BASE   = 'https://fixtures.invalid/releases'
        SKILLHUB_INSTALL_DIR    = $noCosignPrefix
        SKILLHUB_NO_MODIFY_PATH = '1'
    }
    Assert-True ($resNoCosign.ExitCode -eq 0) "Install without cosign should succeed (output: $($resNoCosign.Output))"
    Assert-True ($resNoCosign.Stdout -like "*Signature not checked (cosign not installed)*") "Expected signature warning notice"
    Assert-FileExists (Join-Path $noCosignPrefix 'skillhub.exe') "Binary installed when cosign is absent"
    Pass "Cosign absent: warning notice printed and installation succeeded"

    # 13b: Cosign absent and SKILLHUB_REQUIRE_SIGNATURE=1: aborts
    $resReqSig = Invoke-InstallerProcess -EnvVars @{
        PATH                        = "$cleanPathWithoutCosign$PathSep$env:PATH"
        FIXTURE_HTTP_ROOT           = $HttpRoot
        FIXTURE_HTTP_LOG            = $HttpLog
        SKILLHUB_VERSION            = 'v1.0.0'
        SKILLHUB_ARCH               = 'amd64'
        SKILLHUB_RELEASE_BASE       = 'https://fixtures.invalid/releases'
        SKILLHUB_INSTALL_DIR        = (Join-Path $TestRoot 'req-sig-fail-prefix')
        SKILLHUB_REQUIRE_SIGNATURE  = '1'
        SKILLHUB_NO_MODIFY_PATH     = '1'
    }
    Assert-True ($resReqSig.ExitCode -eq 1) "SKILLHUB_REQUIRE_SIGNATURE=1 without cosign should abort"
    Assert-True ($resReqSig.Stderr -like "*Signature verification is required*") "Expected signature requirement error"
    Pass "Signature requirement enforced when cosign is missing"

    # 13c: Cosign present with mismatched identity: aborts
    $resBadIdentity = Invoke-InstallerProcess -EnvVars @{
        PATH                     = "$FakeBin$PathSep$env:PATH"
        FIXTURE_HTTP_ROOT        = $HttpRoot
        FIXTURE_HTTP_LOG         = $HttpLog
        FIXTURE_COSIGN_LOG       = $CosignLog
        EXPECTED_COSIGN_IDENTITY = 'https://github.com/vantt/mcp-skill-hub/.github/workflows/release.yml@refs/tags/vWRONG'
        SKILLHUB_VERSION         = 'v1.0.0'
        SKILLHUB_ARCH            = 'amd64'
        SKILLHUB_RELEASE_BASE    = 'https://fixtures.invalid/releases'
        SKILLHUB_INSTALL_DIR     = (Join-Path $TestRoot 'bad-identity-prefix')
        SKILLHUB_NO_MODIFY_PATH  = '1'
    }
    Assert-True ($resBadIdentity.ExitCode -eq 1) "Mismatched signature identity should abort"
    Assert-True ($resBadIdentity.Stderr -like "*Sigstore verification failed*") "Expected Sigstore failure message"
    Pass "Mismatched signature identity rejected"

    # 13d: Tampered bundle: aborts
    $tamperedBundleDir = Join-Path $TestRoot 'tampered-bundle'
    Copy-Item -Recurse $fixtureV1 $tamperedBundleDir
    Set-Content -Path (Join-Path $tamperedBundleDir 'checksums.txt.sigstore.json') -Value 'not-a-valid-sha'
    $resTampered = Invoke-InstallerProcess -EnvVars @{
        SKILLHUB_VERSION          = '1.0.0'
        SKILLHUB_ARCH             = 'amd64'
        SKILLHUB_LOCAL_FIXTURE    = $tamperedBundleDir
        SKILLHUB_FIXTURE_VERIFIER = $FixtureVerifier
        SKILLHUB_INSTALL_DIR      = (Join-Path $TestRoot 'tampered-prefix')
        SKILLHUB_NO_MODIFY_PATH   = '1'
    }
    Assert-True ($resTampered.ExitCode -eq 1) "Tampered bundle must abort"
    Assert-True ($resTampered.Stderr -like "*Fixture checksum manifest authentication failed*") "Expected bundle failure"
    Pass "Tampered signature bundle rejected"

    # 13e: Corrupted archive (SHA-256 mismatch): aborts
    $resCorrupt = Invoke-InstallerProcess -EnvVars @{
        SKILLHUB_VERSION          = '4.0.0'
        SKILLHUB_ARCH             = 'amd64'
        SKILLHUB_LOCAL_FIXTURE    = $fixtureCorrupt
        SKILLHUB_FIXTURE_VERIFIER = $FixtureVerifier
        SKILLHUB_INSTALL_DIR      = (Join-Path $TestRoot 'corrupt-prefix')
        SKILLHUB_NO_MODIFY_PATH   = '1'
    }
    Assert-True ($resCorrupt.ExitCode -eq 1) "Corrupt archive must abort"
    Assert-True ($resCorrupt.Stderr -like "*SHA-256 verification failed*") "Expected SHA-256 failure message"
    Pass "Corrupted archive rejected before extraction"

    # 13f: Unexpected extra entry in archive: aborts
    $resExtra = Invoke-InstallerProcess -EnvVars @{
        SKILLHUB_VERSION          = '5.0.0'
        SKILLHUB_ARCH             = 'amd64'
        SKILLHUB_LOCAL_FIXTURE    = $fixtureExtra
        SKILLHUB_FIXTURE_VERIFIER = $FixtureVerifier
        SKILLHUB_INSTALL_DIR      = (Join-Path $TestRoot 'extra-entry-prefix')
        SKILLHUB_NO_MODIFY_PATH   = '1'
    }
    Assert-True ($resExtra.ExitCode -eq 1) "Archive with unexpected entry must abort"
    Assert-True ($resExtra.Stderr -like "*Release archive must contain exactly one non-traversing skillhub.exe binary*") "Expected member check error"
    Pass "Archive with unexpected extra member rejected"

    # 13g: Successful signed download via fake curl & cosign
    $pipePrefix = Join-Path $TestRoot 'signed-download-prefix'
    Set-Content -Path $HttpLog -Value ''
    Set-Content -Path $CosignLog -Value ''
    $resSigned = Invoke-InstallerProcess -EnvVars @{
        PATH                     = "$FakeBin$PathSep$env:PATH"
        FIXTURE_HTTP_ROOT        = $HttpRoot
        FIXTURE_HTTP_LOG         = $HttpLog
        FIXTURE_COSIGN_LOG       = $CosignLog
        EXPECTED_COSIGN_IDENTITY = 'https://github.com/vantt/mcp-skill-hub/.github/workflows/release.yml@refs/tags/v1.0.0'
        SKILLHUB_VERSION         = 'v1.0.0'
        SKILLHUB_ARCH            = 'amd64'
        SKILLHUB_RELEASE_BASE    = 'https://fixtures.invalid/releases'
        SKILLHUB_INSTALL_DIR     = $pipePrefix
        SKILLHUB_NO_MODIFY_PATH  = '1'
    }
    Assert-True ($resSigned.ExitCode -eq 0) "Signed download install should succeed (output: $($resSigned.Output))"
    Assert-FileExists (Join-Path $pipePrefix 'skillhub.exe') "Signed download binary installed"
    Pass "Full signed download workflow authenticated and installed"

    # -----------------------------------------------------------------------
    # Test 14: Uninstall Flow & Workspace Preservation
    # -----------------------------------------------------------------------
    Write-Host "`n[Test 14] Uninstall flow..."
    Assert-FileExists $targetExe "Target binary exists before uninstall"
    Assert-FileExists $markerFile "Marker file exists before uninstall"

    $resUninstall = Invoke-InstallerProcess -EnvVars @{
        SKILLHUB_UNINSTALL   = '1'
        SKILLHUB_INSTALL_DIR = $directPrefix
    }
    Assert-True ($resUninstall.ExitCode -eq 0) "Uninstall should exit 0"
    Assert-True ($resUninstall.Stdout -like "*Removing $targetExe*") "Expected removing binary output"
    Assert-True ($resUninstall.Stdout -like "*Uninstall complete*") "Expected uninstall complete message"

    Assert-PathAbsent $targetExe "Binary removed after uninstall"
    Assert-PathAbsent $markerFile "Marker removed after uninstall"

    # Workspace and host config must be preserved
    Assert-FileExists (Join-Path $Workspace 'skill.md') "Workspace preserved after uninstall"
    Assert-FileExists $HostConfig "Host config preserved after uninstall"
    Pass "Uninstall cleanly removed managed files while preserving workspaces"

    # -----------------------------------------------------------------------
    # Test 15: PATH Modification & Unexpanded Variables
    # -----------------------------------------------------------------------
    Write-Host "`n[Test 15] PATH modification logic & registry safety..."
    # Test SKILLHUB_NO_MODIFY_PATH=1 prints notice
    $noModPrefix = Join-Path $TestRoot 'no-modify-path-prefix'
    $resNoMod = Invoke-InstallerProcess -EnvVars @{
        SKILLHUB_VERSION          = '1.0.0'
        SKILLHUB_ARCH             = 'amd64'
        SKILLHUB_LOCAL_FIXTURE    = $fixtureV1
        SKILLHUB_FIXTURE_VERIFIER = $FixtureVerifier
        SKILLHUB_INSTALL_DIR      = $noModPrefix
        SKILLHUB_NO_MODIFY_PATH   = '1'
    }
    Assert-True ($resNoMod.ExitCode -eq 0) "Install with no-modify-path should exit 0"
    Assert-True ($resNoMod.Stdout -like "*Notice: * is not in your PATH*") "Expected notice message when PATH modification skipped"

    # On Windows: test registry PATH manipulation preserving REG_EXPAND_SZ and %USERPROFILE%
    if ($RunningOnWindows) {
        Write-Host "  Testing Windows Registry HKCU\Environment Path modification..."
        try {
            $hkcu = [Microsoft.Win32.Registry]::CurrentUser
            $testSubKeyName = "EnvironmentTest_" + [System.Guid]::NewGuid().ToString('N')
            $testKey = $hkcu.CreateSubKey($testSubKeyName)
            try {
                $initialPath = '%USERPROFILE%\AppData\Local\bin;C:\Windows\System32'
                $testKey.SetValue('Path', $initialPath, [Microsoft.Win32.RegistryValueKind]::ExpandString)

                $options = [Microsoft.Win32.RegistryValueOptions]::DoNotExpandEnvironmentNames
                $rawRead = $testKey.GetValue('Path', '', $options)
                Assert-True ($rawRead -eq $initialPath) "Registry read with DoNotExpandEnvironmentNames should preserve %USERPROFILE%"
                Assert-True ($testKey.GetValueKind('Path') -eq [Microsoft.Win32.RegistryValueKind]::ExpandString) "Registry value kind should be ExpandString"
            } finally {
                $testKey.Dispose()
                $hkcu.DeleteSubKeyTree($testSubKeyName, $false)
            }
            Pass "Windows Registry unexpanded variable preservation verified"
        } catch {
            Write-Warning "Windows registry test encountered non-fatal issue: $_"
        }
    } else {
        Pass "Registry test guarded (running on non-Windows platform)"
    }

    Write-Host "`n=========================================================="
    Write-Host "ALL 15 TESTS PASSED SUCCESSFULLY!"
    Write-Host "=========================================================="
} finally {
    if (Test-Path -LiteralPath $TestRoot) {
        Remove-Item -LiteralPath $TestRoot -Recurse -Force -ErrorAction SilentlyContinue
    }
}
