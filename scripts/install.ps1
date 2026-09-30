#Requires -Version 5.1
<#
.SYNOPSIS
    Self-contained installer, upgrader, and uninstaller for Skill Hub on Windows.
.DESCRIPTION
    Installs skillhub into %LOCALAPPDATA%\skillhub\bin (or $env:SKILLHUB_INSTALL_DIR),
    configures user PATH in the Windows Registry, and handles atomic upgrades and uninstall.
#>
[CmdletBinding()]
param(
    [string]$Version = '',
    [string]$InstallDir = '',
    [switch]$Uninstall,
    [switch]$RequireSignature,
    [switch]$NoModifyPath
)

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

# --- Configuration & Constants ---------------------------------------------
$DEFAULT_VERSION = '@SKILLHUB_VERSION@'
$ReleaseBase = if (-not [string]::IsNullOrWhiteSpace($env:SKILLHUB_RELEASE_BASE)) {
    $env:SKILLHUB_RELEASE_BASE
} else {
    'https://github.com/vantt/mcp-skill-hub/releases/download'
}

$SigstoreIssuer = 'https://token.actions.githubusercontent.com'
$SigstoreWorkflow = 'https://github.com/vantt/mcp-skill-hub/.github/workflows/release.yml'
$ChecksumMaxBytes   = 1048576       # 1 MiB
$BundleMaxBytes     = 4194304       # 4 MiB
$ArchiveMaxBytes    = 134217728     # 128 MiB
$ConnectTimeoutSec  = 10
$DownloadTimeoutSec = 120

$isUninstall = $Uninstall.IsPresent -or ($env:SKILLHUB_UNINSTALL -eq '1')
$isRequireSignature = $RequireSignature.IsPresent -or ($env:SKILLHUB_REQUIRE_SIGNATURE -eq '1')
$isNoModifyPath = $NoModifyPath.IsPresent -or ($env:SKILLHUB_NO_MODIFY_PATH -eq '1')
$isDryRun = ($env:SKILLHUB_DRY_RUN -eq '1')

# --- Helper Functions ------------------------------------------------------
function Die {
    param([string]$Message)
    [Console]::Error.WriteLine("error: $Message")
    exit 1
}

function Test-IsWindows {
    $isWinVar = Get-Variable -Name 'IsWindows' -ErrorAction SilentlyContinue
    if ($null -ne $isWinVar) {
        return [bool]$isWinVar.Value
    }
    return ([System.Environment]::OSVersion.Platform -eq [System.PlatformID]::Win32NT)
}

function Normalize-PathString {
    param([string]$PathValue)
    if ([string]::IsNullOrWhiteSpace($PathValue)) { return '' }
    $trimmed = $PathValue.Trim().Trim('"')
    $expanded = [System.Environment]::ExpandEnvironmentVariables($trimmed)
    try {
        return [System.IO.Path]::GetFullPath($expanded).TrimEnd('\', '/')
    } catch {
        return $expanded.TrimEnd('\', '/')
    }
}

function Test-SkillhubManaged {
    param([string]$MarkerPath)
    if (-not (Test-Path -LiteralPath $MarkerPath -PathType Leaf)) {
        return $false
    }
    try {
        $firstLine = (Get-Content -LiteralPath $MarkerPath -TotalCount 1).Trim()
        return ($firstLine -eq 'skillhub-managed-v1')
    } catch {
        return $false
    }
}

function Get-InstalledVersion {
    param([string]$MarkerPath)
    if (-not (Test-Path -LiteralPath $MarkerPath -PathType Leaf)) {
        return $null
    }
    try {
        foreach ($line in (Get-Content -LiteralPath $MarkerPath)) {
            $trimmed = $line.Trim()
            if ($trimmed.StartsWith('version=')) {
                return $trimmed.Substring('version='.Length)
            }
        }
    } catch { }
    return $null
}

function Test-SkillhubExecutable {
    param([string]$ExePath)
    if (-not (Test-Path -LiteralPath $ExePath -PathType Leaf)) {
        return $false
    }
    if (-not (Test-IsWindows)) {
        & chmod +x $ExePath 2>$null
    }
    try {
        $null = & $ExePath version 2>&1
        return ($LASTEXITCODE -eq 0)
    } catch {
        return $false
    }
}

function Broadcast-EnvironmentChange {
    if (-not (Test-IsWindows)) { return }
    try {
        if (-not ('SkillhubInstaller.NativeMethods' -as [type])) {
            Add-Type -Namespace SkillhubInstaller -Name NativeMethods -MemberDefinition @'
[System.Runtime.InteropServices.DllImport("user32.dll", SetLastError = true, CharSet = System.Runtime.InteropServices.CharSet.Unicode)]
public static extern System.IntPtr SendMessageTimeout(
    System.IntPtr hWnd,
    uint message,
    System.UIntPtr wParam,
    string lParam,
    uint flags,
    uint timeout,
    out System.UIntPtr result);
'@
        }
        $result = [System.UIntPtr]::Zero
        [SkillhubInstaller.NativeMethods]::SendMessageTimeout(
            [System.IntPtr]0xffff,
            0x001a,
            [System.UIntPtr]::Zero,
            'Environment',
            0x0002,
            5000,
            [ref]$result
        ) | Out-Null
    } catch {
        # Best effort broadcast; do not fail installation if user32 broadcast fails
    }
}

function Set-SkillhubPath {
    param([string]$BinDir)

    $sessionPaths = if ($env:Path) { $env:Path -split ';' } else { @() }
    $inSession = $false
    $normalizedBin = Normalize-PathString $BinDir
    foreach ($p in $sessionPaths) {
        if ((Normalize-PathString $p) -eq $normalizedBin) {
            $inSession = $true
            break
        }
    }

    if ($isNoModifyPath) {
        if (-not $inSession) {
            Write-Host "Notice: $BinDir is not in your PATH."
            Write-Host "Add it to your PATH by running:"
            Write-Host "  [Environment]::SetEnvironmentVariable('Path', [Environment]::GetEnvironmentVariable('Path', 'User') + ';$BinDir', 'User')"
        }
        return
    }

    if (Test-IsWindows) {
        try {
            $hkcu = [Microsoft.Win32.Registry]::CurrentUser
            $envKey = $hkcu.OpenSubKey('Environment', $true)
            if ($null -eq $envKey) {
                $envKey = $hkcu.CreateSubKey('Environment')
            }
            if ($null -ne $envKey) {
                try {
                    $options = [Microsoft.Win32.RegistryValueOptions]::DoNotExpandEnvironmentNames
                    $rawValue = [string]$envKey.GetValue('Path', '', $options)
                    $kind = try { $envKey.GetValueKind('Path') } catch { [Microsoft.Win32.RegistryValueKind]::ExpandString }
                    if ($kind -ne [Microsoft.Win32.RegistryValueKind]::String -and $kind -ne [Microsoft.Win32.RegistryValueKind]::ExpandString) {
                        $kind = [Microsoft.Win32.RegistryValueKind]::ExpandString
                    }

                    $rawEntries = if (-not [string]::IsNullOrWhiteSpace($rawValue)) {
                        $rawValue.Split(';', [System.StringSplitOptions]::RemoveEmptyEntries)
                    } else {
                        @()
                    }

                    $alreadyInRegistry = $false
                    foreach ($entry in $rawEntries) {
                        if ((Normalize-PathString $entry) -eq $normalizedBin) {
                            $alreadyInRegistry = $true
                            break
                        }
                    }

                    if (-not $alreadyInRegistry) {
                        $newRawValue = if ($rawEntries.Count -gt 0) {
                            ($rawEntries + $BinDir) -join ';'
                        } else {
                            $BinDir
                        }
                        $envKey.SetValue('Path', $newRawValue, $kind)
                        Broadcast-EnvironmentChange
                    }
                } finally {
                    $envKey.Dispose()
                }
            }
        } catch {
            Write-Warning "Failed to update registry PATH: $_"
        }
    }

    if (-not $inSession) {
        $env:Path = if ($env:Path) { "$env:Path;$BinDir" } else { $BinDir }
    }
}

function Validate-SkillhubVersion {
    param([string]$RawVersion)

    if ([string]::IsNullOrWhiteSpace($RawVersion) -or $RawVersion -eq '@SKILLHUB_VERSION@') {
        Die "A release version is required. Set SKILLHUB_VERSION."
    }

    $invalidMsg = "Invalid release version '$RawVersion'; expected SemVer such as 1.2.3, v1.2.3, or v1.2.3-rc.1+build.5."

    $v = $RawVersion.Trim()
    if ($v.StartsWith('v', [System.StringComparison]::Ordinal)) {
        $v = $v.Substring(1)
    }

    $semverPattern = '^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(?:-([0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*))?(?:\+([0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*))?$'
    $match = [System.Text.RegularExpressions.Regex]::Match($v, $semverPattern)
    if (-not $match.Success) {
        Die $invalidMsg
    }

    if ($match.Groups[4].Success -and -not [string]::IsNullOrEmpty($match.Groups[4].Value)) {
        $prerelease = $match.Groups[4].Value
        $identifiers = $prerelease.Split('.')
        foreach ($id in $identifiers) {
            if ($id -match '^0[0-9]+$') {
                Die $invalidMsg
            }
        }
    }

    return [PSCustomObject]@{
        ArtifactVersion = $v
        ReleaseTag      = "v$v"
    }
}

function Get-TargetArchitecture {
    $rawArch = if (-not [string]::IsNullOrWhiteSpace($env:SKILLHUB_ARCH)) {
        $env:SKILLHUB_ARCH
    } elseif (-not [string]::IsNullOrWhiteSpace($env:PROCESSOR_ARCHITEW6432)) {
        $env:PROCESSOR_ARCHITEW6432
    } elseif (-not [string]::IsNullOrWhiteSpace($env:PROCESSOR_ARCHITECTURE)) {
        $env:PROCESSOR_ARCHITECTURE
    } else {
        ''
    }

    switch ($rawArch.Trim().ToLowerInvariant()) {
        'amd64'   { return 'amd64' }
        'x86_64'  { return 'amd64' }
        'x64'     { return 'amd64' }
        'arm64'   { return 'arm64' }
        'aarch64' { return 'arm64' }
        default {
            Die "Unsupported architecture '$rawArch'; supported architectures are AMD64 and ARM64."
        }
    }
}

function Validate-InstallSource {
    if (-not [string]::IsNullOrWhiteSpace($env:SKILLHUB_LOCAL_FIXTURE)) {
        if (-not (Test-Path -LiteralPath $env:SKILLHUB_LOCAL_FIXTURE -PathType Container)) {
            Die "Local fixture directory not found: $env:SKILLHUB_LOCAL_FIXTURE"
        }
        if ([string]::IsNullOrWhiteSpace($env:SKILLHUB_FIXTURE_VERIFIER)) {
            Die "Local fixture mode requires SKILLHUB_FIXTURE_VERIFIER."
        }
        if (-not (Test-Path -LiteralPath $env:SKILLHUB_FIXTURE_VERIFIER -PathType Leaf)) {
            Die "Fixture verifier is not an executable file: $env:SKILLHUB_FIXTURE_VERIFIER"
        }
        return
    }

    if (-not $ReleaseBase.StartsWith('https://', [System.StringComparison]::OrdinalIgnoreCase)) {
        Die "Release base must use HTTPS. Use SKILLHUB_LOCAL_FIXTURE for explicit local test data."
    }
}

function Invoke-NetStreamingDownload {
    param(
        [string]$Uri,
        [string]$Destination,
        [long]$MaxBytes,
        [int]$TimeoutSeconds = 120
    )

    try {
        [Net.ServicePointManager]::SecurityProtocol = [Net.ServicePointManager]::SecurityProtocol -bor [Net.SecurityProtocolType]::Tls12
    } catch { }

    try {
        $request = [System.Net.HttpWebRequest]::Create($Uri)
        $request.Timeout = $TimeoutSeconds * 1000
        $request.ReadWriteTimeout = $TimeoutSeconds * 1000
        $request.AllowAutoRedirect = $true
        $request.UserAgent = 'skillhub-installer'

        $response = $request.GetResponse()
        try {
            $contentLength = $response.ContentLength
            if ($contentLength -gt $MaxBytes) {
                Die "Download exceeds the maximum allowed size of $MaxBytes bytes: $Uri"
            }
            $stream = $response.GetResponseStream()
            try {
                $fileStream = [System.IO.File]::Create($Destination)
                try {
                    $buffer = New-Object byte[] 65536
                    $totalBytes = [long]0
                    while (($bytesRead = $stream.Read($buffer, 0, $buffer.Length)) -gt 0) {
                        $totalBytes += $bytesRead
                        if ($totalBytes -gt $MaxBytes) {
                            Die "Download exceeds the maximum allowed size of $MaxBytes bytes: $Uri"
                        }
                        $fileStream.Write($buffer, 0, $bytesRead)
                    }
                } finally {
                    $fileStream.Dispose()
                }
            } finally {
                $stream.Dispose()
            }
        } finally {
            $response.Dispose()
        }
    } catch {
        Die "Download failed: $Uri"
    }
}

function Download-ReleaseFile {
    param(
        [string]$SourceName,
        [string]$Destination,
        [long]$MaxBytes,
        [string]$ReleaseTag
    )

    if (-not [string]::IsNullOrWhiteSpace($env:SKILLHUB_LOCAL_FIXTURE)) {
        $sourcePath = Join-Path $env:SKILLHUB_LOCAL_FIXTURE $SourceName
        if (-not (Test-Path -LiteralPath $sourcePath -PathType Leaf)) {
            Die "Fixture file not found: $sourcePath"
        }
        $fileSize = (Get-Item -LiteralPath $sourcePath).Length
        if ($fileSize -gt $MaxBytes) {
            Die "$SourceName exceeds the maximum allowed size of $MaxBytes bytes."
        }
        Copy-Item -LiteralPath $sourcePath -Destination $Destination -Force
        return
    }

    $url = $ReleaseBase.TrimEnd('/') + "/$ReleaseTag/$SourceName"
    if (-not $url.StartsWith('https://', [System.StringComparison]::OrdinalIgnoreCase)) {
        Die "Refusing non-HTTPS download: $url"
    }

    $curlCmd = Get-Command 'curl' -ErrorAction SilentlyContinue
    if ($null -eq $curlCmd) {
        $curlCmd = Get-Command 'curl.exe' -ErrorAction SilentlyContinue
    }

    if ($null -ne $curlCmd) {
        $curlArgs = @(
            '--fail',
            '--location',
            '--proto', '=https',
            '--tlsv1.2',
            '--silent',
            '--show-error',
            '--connect-timeout', "$ConnectTimeoutSec",
            '--max-time', "$DownloadTimeoutSec",
            '--max-filesize', "$MaxBytes",
            '--output', $Destination,
            $url
        )
        $output = & $curlCmd.Source @curlArgs 2>&1
        if ($LASTEXITCODE -ne 0) {
            Die "Download failed: $url"
        }
        if (-not (Test-Path -LiteralPath $Destination -PathType Leaf)) {
            Die "Download failed: $url"
        }
        $downloadedSize = (Get-Item -LiteralPath $Destination).Length
        if ($downloadedSize -gt $MaxBytes) {
            Die "$SourceName exceeds the maximum allowed size of $MaxBytes bytes."
        }
        return
    }

    Invoke-NetStreamingDownload -Uri $url -Destination $Destination -MaxBytes $MaxBytes -TimeoutSeconds $DownloadTimeoutSec
}

function Verify-ReleaseManifest {
    param(
        [string]$ManifestPath,
        [string]$BundlePath,
        [string]$ReleaseTag
    )

    $identity = "$SigstoreWorkflow@refs/tags/$ReleaseTag"

    if (-not [string]::IsNullOrWhiteSpace($env:SKILLHUB_LOCAL_FIXTURE)) {
        & $env:SKILLHUB_FIXTURE_VERIFIER $ManifestPath $BundlePath $identity $SigstoreIssuer
        if ($LASTEXITCODE -ne 0) {
            Die "Fixture checksum manifest authentication failed."
        }
        Write-Host "Authenticated checksums.txt with Sigstore bundle"
        return
    }

    $cosignCmd = Get-Command 'cosign' -ErrorAction SilentlyContinue
    if ($null -eq $cosignCmd) {
        $cosignCmd = Get-Command 'cosign.exe' -ErrorAction SilentlyContinue
    }

    if ($null -ne $cosignCmd) {
        $cosignArgs = @(
            'verify-blob',
            '--bundle', $BundlePath,
            '--certificate-identity', $identity,
            '--certificate-oidc-issuer', $SigstoreIssuer,
            $ManifestPath
        )
        $output = & $cosignCmd.Source @cosignArgs 2>&1
        if ($LASTEXITCODE -ne 0) {
            Die "Sigstore verification failed for checksums.txt."
        }
        Write-Host "Authenticated checksums.txt with Sigstore bundle"
    } else {
        Write-Host ("Signature not checked (cosign not installed). To verify: " +
            "cosign verify-blob --bundle checksums.txt.sigstore.json --certificate-identity $identity --certificate-oidc-issuer $SigstoreIssuer checksums.txt")
    }
}

function Verify-ArchiveChecksum {
    param(
        [string]$ArchivePath,
        [string]$ManifestPath,
        [string]$ArchiveName
    )

    $lines = Get-Content -LiteralPath $ManifestPath
    $expectedHash = $null
    $matches = 0
    foreach ($line in $lines) {
        $trimmed = $line.Trim()
        if ([string]::IsNullOrWhiteSpace($trimmed)) { continue }
        $parts = $trimmed -split '\s+'
        if ($parts.Count -lt 2) { continue }
        $hash = $parts[0]
        $file = $parts[1].TrimStart('*')
        if ($file -eq $ArchiveName) {
            if ($parts.Count -gt 2) {
                Die "Malformed checksum entry for $ArchiveName."
            }
            $expectedHash = $hash.ToLowerInvariant()
            $matches++
        }
    }

    if ($matches -ne 1) {
        Die "Checksum manifest must contain exactly one entry for $ArchiveName."
    }

    if ($expectedHash -notmatch '^[0-9a-f]{64}$') {
        Die "Invalid SHA-256 checksum for $ArchiveName."
    }

    $actualHash = (Get-FileHash -LiteralPath $ArchivePath -Algorithm SHA256).Hash.ToLowerInvariant()
    if ($actualHash -ne $expectedHash) {
        Die "SHA-256 verification failed for $ArchiveName."
    }

    Write-Host "Verified SHA-256 for $ArchiveName"
}

function Invoke-SkillhubUninstall {
    param(
        [string]$BinDir,
        [string]$TargetExe,
        [string]$MarkerFile
    )

    $isManaged = Test-SkillhubManaged $MarkerFile
    $targetExists = Test-Path -LiteralPath $TargetExe -PathType Leaf
    $markerExists = Test-Path -LiteralPath $MarkerFile -PathType Leaf

    if ($targetExists) {
        if (-not $isManaged) {
            Die "Refusing to remove unmanaged path: $TargetExe"
        }
        if ($isDryRun) {
            Write-Host "Would remove $TargetExe"
        } else {
            Write-Host "Removing $TargetExe"
            Remove-Item -LiteralPath $TargetExe -Force
        }
    } elseif ($markerExists) {
        if (-not $isManaged) {
            Die "Invalid installation marker: $MarkerFile"
        }
        Write-Host "Managed binary is already absent: $TargetExe"
    } else {
        Write-Host "No managed skillhub installation found at $TargetExe"
    }

    if ($markerExists -and $isManaged) {
        if ($isDryRun) {
            Write-Host "Would remove $MarkerFile"
        } else {
            Remove-Item -LiteralPath $MarkerFile -Force
        }
    }

    if ($isDryRun) {
        exit 0
    }

    if (Test-IsWindows) {
        try {
            $hkcu = [Microsoft.Win32.Registry]::CurrentUser
            $envKey = $hkcu.OpenSubKey('Environment', $true)
            if ($null -ne $envKey) {
                try {
                    $options = [Microsoft.Win32.RegistryValueOptions]::DoNotExpandEnvironmentNames
                    $rawValue = [string]$envKey.GetValue('Path', '', $options)
                    $kind = try { $envKey.GetValueKind('Path') } catch { [Microsoft.Win32.RegistryValueKind]::ExpandString }
                    if (-not [string]::IsNullOrWhiteSpace($rawValue)) {
                        $normalizedBin = Normalize-PathString $BinDir
                        $rawEntries = $rawValue.Split(';', [System.StringSplitOptions]::RemoveEmptyEntries)
                        $retained = @()
                        $modified = $false
                        foreach ($entry in $rawEntries) {
                            if ((Normalize-PathString $entry) -eq $normalizedBin) {
                                $modified = $true
                            } else {
                                $retained += $entry
                            }
                        }
                        if ($modified) {
                            $newRawValue = $retained -join ';'
                            $envKey.SetValue('Path', $newRawValue, $kind)
                            Broadcast-EnvironmentChange
                        }
                    }
                } finally {
                    $envKey.Dispose()
                }
            }
        } catch {
            Write-Warning "Failed to update registry PATH during uninstall: $_"
        }
    }

    if ($env:Path) {
        $normalizedBin = Normalize-PathString $BinDir
        $sessionPaths = $env:Path -split ';'
        $retainedSession = @()
        foreach ($p in $sessionPaths) {
            if ((Normalize-PathString $p) -ne $normalizedBin) {
                $retainedSession += $p
            }
        }
        $env:Path = $retainedSession -join ';'
    }

    Write-Host "Uninstall complete. Canonical workspaces and host configuration were preserved."
    exit 0
}

# --- Resolve Install Location ----------------------------------------------
$installDirInput = if (-not [string]::IsNullOrWhiteSpace($InstallDir)) {
    $InstallDir
} elseif (-not [string]::IsNullOrWhiteSpace($env:SKILLHUB_INSTALL_DIR)) {
    $env:SKILLHUB_INSTALL_DIR
} else {
    $null
}

$defaultBinDir = if ($installDirInput) {
    $installDirInput
} elseif (-not [string]::IsNullOrWhiteSpace($env:LOCALAPPDATA)) {
    Join-Path $env:LOCALAPPDATA 'skillhub\bin'
} elseif (-not [string]::IsNullOrWhiteSpace($env:USERPROFILE)) {
    Join-Path $env:USERPROFILE 'AppData\Local\skillhub\bin'
} elseif (-not [string]::IsNullOrWhiteSpace($env:HOME)) {
    Join-Path $env:HOME '.local\share\skillhub\bin'
} else {
    Join-Path (Get-Location).Path 'skillhub\bin'
}

$binDir = [System.IO.Path]::GetFullPath($defaultBinDir).TrimEnd('\', '/')
$targetExe = Join-Path $binDir 'skillhub.exe'
$markerFile = Join-Path $binDir '.skillhub-managed'

# --- Handle Uninstall Path -------------------------------------------------
if ($isUninstall) {
    Invoke-SkillhubUninstall -BinDir $binDir -TargetExe $targetExe -MarkerFile $markerFile
}

# --- Version & Architecture Validation ------------------------------------
$versionInput = if (-not [string]::IsNullOrWhiteSpace($Version)) {
    $Version
} elseif (-not [string]::IsNullOrWhiteSpace($env:SKILLHUB_VERSION)) {
    $env:SKILLHUB_VERSION
} else {
    $DEFAULT_VERSION
}

$versionInfo = Validate-SkillhubVersion -RawVersion $versionInput
$artifactVersion = $versionInfo.ArtifactVersion
$releaseTag = $versionInfo.ReleaseTag

$arch = Get-TargetArchitecture
Validate-InstallSource

# --- Check Current Installation State -------------------------------------
$isUpgrade = $false
$isSameVersion = $false

$targetExists = Test-Path -LiteralPath $targetExe -PathType Leaf
$markerExists = Test-Path -LiteralPath $markerFile -PathType Leaf

if ($targetExists) {
    if (-not (Test-SkillhubManaged $markerFile)) {
        Die "Refusing to overwrite existing unmanaged binary at $targetExe. Only installations managed by skillhub installer (with a valid .skillhub-managed marker) can be upgraded."
    }
    $isUpgrade = $true
    $installedVersion = Get-InstalledVersion $markerFile
    if (-not [string]::IsNullOrWhiteSpace($installedVersion) -and $installedVersion -eq $artifactVersion) {
        $isSameVersion = $true
    }
} elseif ($markerExists) {
    Die "Refusing installation because a marker already exists without a binary: $markerFile"
}

if ($isDryRun) {
    if ($isSameVersion) {
        Write-Host "Dry run: skillhub $artifactVersion is already installed at $targetExe"
    } elseif ($isUpgrade) {
        Write-Host "Dry run: would upgrade skillhub to $artifactVersion at $targetExe"
    } else {
        Write-Host "Dry run: would install skillhub $artifactVersion to $targetExe"
    }
    exit 0
}

if ($isSameVersion) {
    Write-Host "skillhub $artifactVersion is already installed at $targetExe"
    Set-SkillhubPath -BinDir $binDir
    Write-Host "Next step:`n  skillhub init ~/skillhub --yes"
    exit 0
}

# Check signature requirement before network operations
$cosignCmd = Get-Command 'cosign' -ErrorAction SilentlyContinue
if ($null -eq $cosignCmd) {
    $cosignCmd = Get-Command 'cosign.exe' -ErrorAction SilentlyContinue
}
if ($isRequireSignature -and [string]::IsNullOrWhiteSpace($env:SKILLHUB_LOCAL_FIXTURE) -and ($null -eq $cosignCmd)) {
    Die "Signature verification is required (SKILLHUB_REQUIRE_SIGNATURE=1) but cosign is not installed. Install cosign from https://github.com/sigstore/cosign to continue."
}

# --- Download & Extract Candidate -----------------------------------------
$tempDir = Join-Path ([System.IO.Path]::GetTempPath()) ("skillhub-install." + [System.Guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Force -Path $tempDir | Out-Null

try {
    $archiveName = "skillhub-$artifactVersion-windows-$arch.zip"
    $archivePath = Join-Path $tempDir $archiveName
    $manifestPath = Join-Path $tempDir 'checksums.txt'
    $bundlePath = Join-Path $tempDir 'checksums.txt.sigstore.json'
    $extractDir = Join-Path $tempDir 'extract'

    Download-ReleaseFile -SourceName 'checksums.txt' -Destination $manifestPath -MaxBytes $ChecksumMaxBytes -ReleaseTag $releaseTag

    $shouldVerifySig = (-not [string]::IsNullOrWhiteSpace($env:SKILLHUB_LOCAL_FIXTURE)) -or ($null -ne $cosignCmd)
    if ($shouldVerifySig) {
        Download-ReleaseFile -SourceName 'checksums.txt.sigstore.json' -Destination $bundlePath -MaxBytes $BundleMaxBytes -ReleaseTag $releaseTag
        Verify-ReleaseManifest -ManifestPath $manifestPath -BundlePath $bundlePath -ReleaseTag $releaseTag
    } else {
        $identity = "$SigstoreWorkflow@refs/tags/$releaseTag"
        Write-Host ("Signature not checked (cosign not installed). To verify: " +
            "cosign verify-blob --bundle checksums.txt.sigstore.json --certificate-identity $identity --certificate-oidc-issuer $SigstoreIssuer checksums.txt")
    }

    Download-ReleaseFile -SourceName $archiveName -Destination $archivePath -MaxBytes $ArchiveMaxBytes -ReleaseTag $releaseTag
    Verify-ArchiveChecksum -ArchivePath $archivePath -ManifestPath $manifestPath -ArchiveName $archiveName

    Add-Type -AssemblyName System.IO.Compression.FileSystem -ErrorAction SilentlyContinue
    try {
        $zip = [System.IO.Compression.ZipFile]::OpenRead($archivePath)
    } catch {
        Die "Could not inspect release archive: $archiveName"
    }

    try {
        if ($zip.Entries.Count -ne 1) {
            Die "Release archive must contain exactly one non-traversing skillhub.exe binary."
        }
        $entry = $zip.Entries[0]
        $entryName = $entry.FullName.Replace('\', '/')
        if ($entryName -ne 'skillhub.exe' -and $entryName -ne './skillhub.exe') {
            Die "Release archive must contain exactly one non-traversing skillhub.exe binary."
        }
        if ($entryName.Contains('..') -or $entryName.StartsWith('/') -or $entryName.Contains(':')) {
            Die "Release archive must contain exactly one non-traversing skillhub.exe binary."
        }
    } finally {
        $zip.Dispose()
    }

    New-Item -ItemType Directory -Force -Path $extractDir | Out-Null
    try {
        Expand-Archive -LiteralPath $archivePath -DestinationPath $extractDir -Force
    } catch {
        Die "Could not extract release archive: $archiveName"
    }

    $candidateExe = Join-Path $extractDir 'skillhub.exe'
    if (-not (Test-Path -LiteralPath $candidateExe -PathType Leaf)) {
        Die "Release archive does not contain a regular skillhub.exe binary."
    }

    if (-not (Test-SkillhubExecutable $candidateExe)) {
        Die "Downloaded skillhub binary failed its version check."
    }

    # --- Install / Upgrade Target -----------------------------------------
    if (-not (Test-Path -LiteralPath $binDir)) {
        New-Item -ItemType Directory -Force -Path $binDir | Out-Null
    }

    if ($isUpgrade) {
        $oldExe = Join-Path $binDir ("skillhub.exe.old." + [System.Guid]::NewGuid().ToString('N'))
        $markerBackup = Join-Path $tempDir "managed-marker.backup"

        Copy-Item -LiteralPath $markerFile -Destination $markerBackup -Force
        Move-Item -LiteralPath $targetExe -Destination $oldExe -Force
        Move-Item -LiteralPath $candidateExe -Destination $targetExe -Force
        if (-not (Test-IsWindows)) {
            & chmod +x $targetExe 2>$null
        }

        $markerContent = "skillhub-managed-v1`nversion=$artifactVersion`n"
        [System.IO.File]::WriteAllText($markerFile, $markerContent, (New-Object System.Text.UTF8Encoding($false)))

        if (-not (Test-SkillhubExecutable $targetExe)) {
            $restored = $false
            try {
                Remove-Item -LiteralPath $targetExe -Force -ErrorAction SilentlyContinue
                Move-Item -LiteralPath $oldExe -Destination $targetExe -Force
                Copy-Item -LiteralPath $markerBackup -Destination $markerFile -Force
                if (-not (Test-IsWindows)) {
                    & chmod +x $targetExe 2>$null
                }
                $restored = (Test-Path -LiteralPath $targetExe -PathType Leaf)
            } catch {
                $restored = $false
            }
            if ($restored) {
                Die "Upgraded binary failed its version check; restored the previous installation."
            } else {
                Die "Upgraded binary failed its version check and automatic rollback failed. Recovery copy: $oldExe"
            }
        }

        try {
            Remove-Item -LiteralPath $oldExe -Force -ErrorAction SilentlyContinue
        } catch { }

        Write-Host "Upgraded skillhub to $artifactVersion at $targetExe"
    } else {
        Move-Item -LiteralPath $candidateExe -Destination $targetExe -Force
        if (-not (Test-IsWindows)) {
            & chmod +x $targetExe 2>$null
        }

        $markerContent = "skillhub-managed-v1`nversion=$artifactVersion`n"
        [System.IO.File]::WriteAllText($markerFile, $markerContent, (New-Object System.Text.UTF8Encoding($false)))

        if (-not (Test-SkillhubExecutable $targetExe)) {
            Remove-Item -LiteralPath $targetExe -Force -ErrorAction SilentlyContinue
            Remove-Item -LiteralPath $markerFile -Force -ErrorAction SilentlyContinue
            Die "Installed skillhub binary failed its version check."
        }

        Write-Host "Installed skillhub $artifactVersion to $targetExe"
    }

    Set-SkillhubPath -BinDir $binDir
    Write-Host "Next step:`n  skillhub init ~/skillhub --yes"
} finally {
    if (Test-Path -LiteralPath $tempDir) {
        Remove-Item -LiteralPath $tempDir -Recurse -Force -ErrorAction SilentlyContinue
    }
}
