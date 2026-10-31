#Requires -Version 7.2
<#
.SYNOPSIS
Builds the server SPKs of all three architecture families (x86_64, armv8, armv7) with Docker.

.DESCRIPTION
PowerShell twin of build.sh (ADR 0026 section 2, task SPK-07): same steps, options and output.
Nothing but Docker and PowerShell 7.2 or newer is needed on the host.

Step 1 (docker build of packaging/synology/Dockerfile, unprivileged): web app, server binaries,
FFmpeg and the staged projects -> packaging/synology/dist/stage/<family>/.
Step 2 (docker run of the toolkit image with CAP_SYS_ADMIN, one family after the other because
the toolkit volume serialises runs): packs each project -> dist/<id>-<version>-<family>.spk
(package revision N >= 1: <id>-<version>-r<N>-<family>.spk), then dist/SHA256SUMS. The first
error stops the build.

A test build (the default outside CI) packs an interim FFmpeg with a loud warning and writes
dist/NOT-FOR-RELEASE.txt. A release build (-Release; the default on CI hosts) refuses interim
or non-free FFmpeg builds, spkstage warnings, the default version 0.0.0-dev and, when git is
present, a dirty working tree.

Empty values: a parameter given with an empty value is refused; an empty environment variable
counts as unset. A host counts as CI when CI is set to anything but empty, "false" or "0".

Environment: SPK_BUILD_MODE (release or test), SPK_PRIVILEGE (see -Privilege),
SOURCE_DATE_EPOCH, SPK_TOOLKIT_VOLUME (default spk-toolkit), SPK_TOOLKIT_IMAGE (default
spk-toolkit:dev); NODE_IMAGE, GO_IMAGE and FFMPEG_FETCH_IMAGE are passed to the Dockerfile when
set; BUILDKIT_PROGRESS=plain shows every log line of step 1.

.PARAMETER Version
Server version, e.g. 0.1.0-alpha.2 (default 0.0.0-dev); checked against the version scheme of
spkmeta before anything runs.

.PARAMETER Release
Release build (also SPK_BUILD_MODE=release).

.PARAMETER TestBuild
Local test build (also SPK_BUILD_MODE=test).

.PARAMETER Revision
Git commit recorded in dist/stage/build-info.txt (default: git rev-parse HEAD, empty without git).

.PARAMETER PackageRevision
Packaging-only revision 0-9 of the same server version (default 0).

.PARAMETER SourceDateEpoch
File time of the staged projects in seconds since 1970 (default: SOURCE_DATE_EPOCH, else the
commit time of HEAD, else 0).

.PARAMETER Privilege
Privileges of the toolkit container (also SPK_PRIVILEGE): sys-admin (--cap-add SYS_ADMIN,
default; the tested setting of ADR 0026 Notes), sys-admin-apparmor (--cap-add SYS_ADMIN
--security-opt apparmor=unconfined, for Linux hosts with AppArmor such as GitHub-hosted Ubuntu
runners; not verified yet) or privileged (--privileged, fallback only).

.EXAMPLE
pwsh -NoProfile -File packaging/synology/build.ps1 -Version 0.1.0-alpha.2

.EXAMPLE
$env:SPK_PRIVILEGE = 'sys-admin-apparmor'; packaging/synology/build.ps1 0.1.0-alpha.2 -Release
#>
[CmdletBinding()]
param(
    [Parameter(Position = 0)]
    [ValidateNotNullOrEmpty()]
    [string] $Version = '0.0.0-dev',
    [switch] $Release,
    [switch] $TestBuild,
    [ValidateNotNullOrEmpty()]
    [string] $Revision = '',
    [ValidateRange(0, 9)]
    [int] $PackageRevision = 0,
    [ValidateNotNullOrEmpty()]
    [string] $SourceDateEpoch = '',
    [ValidateNotNullOrEmpty()]
    [string] $Privilege = ''
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

$DevVersion = '0.0.0-dev'
$Families = @('x86_64', 'armv8', 'armv7')
$SumsName = 'SHA256SUMS'
$MarkerName = 'NOT-FOR-RELEASE.txt'
$NoticePath = 'prebuilt/share/licenses/ffmpeg/SOURCE.txt'
$PackageNamePattern = '^[A-Za-z0-9][A-Za-z0-9._-]*$'
# The server versions spkmeta maps (internal/spkmeta/version.go): SemVer core without leading
# zeros, optionally -alpha.N, -beta.N or -rc.N (N 0-99), or the development version.
$VersionPattern = '^(0\.0\.0-dev|(0|[1-9][0-9]{0,9})\.(0|[1-9][0-9]{0,9})\.(0|[1-9][0-9]{0,9})(-(alpha|beta|rc)\.(0|[1-9][0-9]?))?)$'
# DSM version of spkmeta: <core>-<build>, the build number ends with the package revision.
$DsmVersionPattern = '^[0-9]+\.[0-9]+\.[0-9]+-[0-9]{4,}$'
$PrivilegeOptions = @{
    'sys-admin'          = @('--cap-add', 'SYS_ADMIN')
    'sys-admin-apparmor' = @('--cap-add', 'SYS_ADMIN', '--security-opt', 'apparmor=unconfined')
    'privileged'         = @('--privileged')
}

$RepoRoot = (Resolve-Path (Join-Path $PSScriptRoot '../..')).Path
$Dist = Join-Path $PSScriptRoot 'dist'
$Stage = Join-Path $Dist 'stage'
$SourcesLock = Join-Path $PSScriptRoot 'ffmpeg/sources.lock'
$Volume = if ($env:SPK_TOOLKIT_VOLUME) { $env:SPK_TOOLKIT_VOLUME } else { 'spk-toolkit' }
$Image = if ($env:SPK_TOOLKIT_IMAGE) { $env:SPK_TOOLKIT_IMAGE } else { 'spk-toolkit:dev' }

function Write-BuildLog([string] $Message) {
    [Console]::Error.WriteLine("[spk-build] $Message")
}

function Exit-Build([string] $Message) {
    [Console]::Error.WriteLine("[spk-build] failed: $Message")
    exit 1
}

# Runs docker with the given arguments and stops the build when it fails; -Quiet discards its
# standard output (the image ID of `docker build -q`).
function Invoke-Docker([string] $Step, [string[]] $Arguments, [switch] $Quiet) {
    if ($Quiet) { & docker @Arguments | Out-Null } else { & docker @Arguments }
    if ($LASTEXITCODE -ne 0) { Exit-Build "$Step failed (docker exit code $LASTEXITCODE)" }
}

function Test-CiHost {
    $ci = "$env:CI".ToLowerInvariant()
    return $ci -and $ci -ne 'false' -and $ci -ne '0'
}

# A build is a release build when asked for, and by default on CI hosts: anything a CI run
# uploads can be handed out, so it must never carry an interim FFmpeg unless the workflow asks
# for a test build explicitly.
function Get-BuildMode {
    if ($Release -and $TestBuild) { Exit-Build '-Release and -TestBuild exclude each other' }
    $mode = if ($Release) { 'release' } elseif ($TestBuild) { 'test' } elseif ($env:SPK_BUILD_MODE) {
        $env:SPK_BUILD_MODE
    } elseif (Test-CiHost) { 'release' } else { 'test' }
    if ($mode -cnotin @('release', 'test')) { Exit-Build "SPK_BUILD_MODE must be release or test, not '$mode'" }
    return $mode
}

# Runs git in the repository and returns its trimmed output, or $null without git or when the
# command fails.
function Invoke-RepoGit([string[]] $Arguments) {
    if (-not (Get-Command git -ErrorAction SilentlyContinue)) { return $null }
    $output = & git -C $RepoRoot @Arguments 2>$null
    if ($LASTEXITCODE -ne 0) { return $null }
    return ($output -join "`n").Trim()
}

function Resolve-BuildInput {
    if ($Version -cnotmatch $VersionPattern) {
        Exit-Build "version '$Version' is not <major>.<minor>.<patch>[-alpha.N|-beta.N|-rc.N] or $DevVersion"
    }
    $script:Mode = Get-BuildMode
    # A separate variable: the validation attribute of -Revision would refuse an empty git result.
    $script:BuildRevision = if ($script:RevisionGiven) { $Revision } else { "$(Invoke-RepoGit @('rev-parse', 'HEAD'))" }
    $epoch = if ($SourceDateEpoch) { $SourceDateEpoch } elseif ($env:SOURCE_DATE_EPOCH) { $env:SOURCE_DATE_EPOCH } else {
        "$(Invoke-RepoGit @('log', '-1', '--format=%ct'))"
    }
    if (-not $epoch) { $epoch = '0' }
    if ($epoch -notmatch '^[0-9]+$') { Exit-Build "SOURCE_DATE_EPOCH must be seconds since 1970, not '$epoch'" }
    $script:Epoch = $epoch
    $privilegeName = if ($Privilege) { $Privilege } elseif ($env:SPK_PRIVILEGE) { $env:SPK_PRIVILEGE } else { 'sys-admin' }
    if (-not $PrivilegeOptions.ContainsKey($privilegeName)) {
        Exit-Build "-Privilege must be sys-admin, sys-admin-apparmor or privileged, not '$privilegeName'"
    }
    $script:DockerPrivilege = $PrivilegeOptions[$privilegeName]
}

# Families whose sources.lock entry is interim (same test as SPK-16's acceptance check).
function Get-InterimFamily {
    if (-not (Test-Path -LiteralPath $SourcesLock -PathType Leaf)) { Exit-Build "$SourcesLock not found" }
    $families = foreach ($line in Get-Content -LiteralPath $SourcesLock) {
        if ($line -match '^\s*#') { continue }
        $fields = -split $line
        if ($fields.Count -gt 1 -and $fields[1] -ceq 'interim') { $fields[0] }
    }
    return @($families) -join ' '
}

function Write-InterimBanner([string] $InterimFamilies) {
    $rule = '=' * 90
    @(
        $rule
        'WARNING: TEST BUILD WITH INTERIM FFMPEG - NOT FOR RELEASE OR DISTRIBUTION'
        "  ffmpeg/sources.lock has interim entries for: $InterimFamilies"
        '  Interim builds miss criteria of ADR 0026 section 4 (static, licence, kernel); the'
        '  release build (-Release) refuses them until SPK-16 switches to the own FFmpeg build.'
        $rule
    ) | ForEach-Object { [Console]::Error.WriteLine($_) }
}

# A release must be rebuildable from its commit, so uncommitted or untracked files are refused
# (dist/ is ignored by packaging/synology/.gitignore). Without git or outside a checkout there
# is nothing to compare against.
function Assert-CleanTree {
    if ((Invoke-RepoGit @('rev-parse', '--is-inside-work-tree')) -ne 'true') { return }
    $changes = Invoke-RepoGit @('status', '--porcelain')
    if ($null -eq $changes) { Exit-Build "git status failed in $RepoRoot" }
    if ($changes) {
        $first = ($changes -split "`n" | Select-Object -First 10) -join "`n"
        Exit-Build "release refused: the working tree has uncommitted changes:`n$first"
    }
}

function Assert-ReleaseInput([string] $InterimFamilies) {
    if ($script:Mode -eq 'release') {
        if ($Version -eq $DevVersion) { Exit-Build "a release build needs a version, not the default $DevVersion" }
        if ($InterimFamilies) {
            Exit-Build "release refused: ffmpeg/sources.lock has interim FFmpeg entries for $InterimFamilies"
        }
        Assert-CleanTree
    } elseif ($InterimFamilies) {
        Write-InterimBanner $InterimFamilies
    }
}

function Initialize-Dist {
    if (Test-Path -LiteralPath $Stage) { Remove-Item -LiteralPath $Stage -Recurse -Force }
    New-Item -ItemType Directory -Force -Path $Dist | Out-Null
    Get-ChildItem -LiteralPath $Dist -File |
        Where-Object { $_.Name -like '*.spk' -or $_.Name -in @($SumsName, $MarkerName) } |
        Remove-Item -Force
}

function Get-BuildArgument {
    $releaseFlag = if ($script:Mode -eq 'release') { 'true' } else { 'false' }
    $arguments = @(
        '--build-arg', "VERSION=$Version", '--build-arg', "REVISION=$script:BuildRevision",
        '--build-arg', "PACKAGE_REVISION=$PackageRevision",
        '--build-arg', "SOURCE_DATE_EPOCH=$script:Epoch", '--build-arg', "RELEASE=$releaseFlag"
    )
    foreach ($name in @('NODE_IMAGE', 'GO_IMAGE', 'FFMPEG_FETCH_IMAGE')) {
        $value = [Environment]::GetEnvironmentVariable($name)
        if ($value) { $arguments += @('--build-arg', "$name=$value") }
    }
    return $arguments
}

# --output is a CSV list; a comma or quote in the path needs a quoted field. PowerShell 7.2
# passes embedded quotes unescaped to native programs, which breaks paths with blanks, so the
# field is quoted only when the path needs it (build.sh always quotes).
function Get-OutputArgument {
    if ($Stage -notmatch '[,"]') { return "type=local,dest=$Stage" }
    return 'type=local,"dest=' + $Stage.Replace('"', '""') + '"'
}

function Invoke-StageStep {
    $watch = [Diagnostics.Stopwatch]::StartNew()
    Write-BuildLog "step 1: prebuild and stage ($script:Mode build, version $Version)"
    $arguments = @('build', '-f', (Join-Path $PSScriptRoot 'Dockerfile')) + (Get-BuildArgument) +
        @('--output', (Get-OutputArgument), $RepoRoot)
    Invoke-Docker 'step 1 (docker build)' $arguments
    Write-BuildLog "step 1 finished in $([int]$watch.Elapsed.TotalSeconds)s"
}

# Reads one value of a staged build.env (lines NAME='literal', checked by spkstage).
function Get-BuildEnvValue([string] $Project, [string] $Name) {
    foreach ($line in Get-Content -LiteralPath (Join-Path $Project 'build.env')) {
        if ($line -cmatch "^$Name='([^']*)'$") { return $Matches[1] }
    }
    return ''
}

# Returns status and licence of a staged FFmpeg SOURCE.txt; a notice with no or several Status
# or Licence lines is refused, since either value could hide behind the other.
function Get-FfmpegNotice([string] $Project) {
    $notice = Join-Path $Project $NoticePath
    if (-not (Test-Path -LiteralPath $notice -PathType Leaf)) { Exit-Build "$notice missing in the staged project" }
    $lines = @(Get-Content -LiteralPath $notice)
    $status = @($lines | Where-Object { $_ -cmatch '^Status:' })
    $licence = @($lines | Where-Object { $_ -cmatch '^Licence:' })
    if ($status.Count -ne 1 -or $licence.Count -ne 1) {
        Exit-Build "$notice needs exactly one Status: and one Licence: line (has $($status.Count) and $($licence.Count))"
    }
    $result = @{
        Status  = if ($status[0] -cmatch '^Status:\s*(\S+)') { $Matches[1] } else { '' }
        Licence = if ($licence[0] -cmatch '^Licence:\s*(\S+)') { $Matches[1] } else { '' }
    }
    if (-not $result.Status -or -not $result.Licence) { Exit-Build "$notice has an empty Status: or Licence: value" }
    return $result
}

# Reads what was staged, not what sources.lock says: the release check holds for any stage
# that supplies FFmpeg. Test builds list the families in NOT-FOR-RELEASE.txt.
function Assert-StagedProject {
    $notEligible = @()
    foreach ($family in $Families) {
        $project = Join-Path $Stage $family
        if (-not (Test-Path -LiteralPath (Join-Path $project 'build.env') -PathType Leaf)) {
            Exit-Build "step 1 staged no project for $family ($project)"
        }
        $notice = Get-FfmpegNotice $project
        if ($notice.Status -cne 'selected' -or $notice.Licence -like '*nonfree*') {
            $notEligible += "$family ($($notice.Status), $($notice.Licence))"
        }
    }
    if ($script:Mode -eq 'release') {
        if ($notEligible) { Exit-Build "release refused: FFmpeg builds not release-eligible: $($notEligible -join ' ')" }
        return
    }
    $lines = @(
        'These SPKs come from a TEST BUILD (build.sh/build.ps1 without --release / -Release).'
        'Do not publish, upload or hand them out; build again with --release for that.'
    )
    if ($notEligible) { $lines += "FFmpeg builds that are not release-eligible (status, licence): $($notEligible -join ' ')" }
    Write-LfFile (Join-Path $Dist $MarkerName) $lines
}

function Write-LfFile([string] $Path, [string[]] $Lines) {
    $text = ($Lines -join "`n") + "`n"
    [IO.File]::WriteAllText($Path, $text, [Text.UTF8Encoding]::new($false))
}

# Returns the toolkit's file name and the release file name of a packed project:
# <id>-<version>-<family>.spk, with -r<N> before the family for package revision N >= 1. N is
# read back from the staged DSM version (spkmeta: the last digit of the build number) and must
# be the revision that was asked for.
function Get-SpkName([string] $Family) {
    $project = Join-Path $Stage $Family
    $id = Get-BuildEnvValue $project 'ID'
    $serverVersion = Get-BuildEnvValue $project 'VERSION'
    $dsmVersion = Get-BuildEnvValue $project 'DSM_VERSION'
    if ($id -cnotmatch $PackageNamePattern) { Exit-Build "staged build.env of $Family has no valid ID ('$id')" }
    if ($serverVersion -cne $Version) { Exit-Build "staged build.env of $Family has VERSION '$serverVersion', not '$Version'" }
    if ($dsmVersion -cnotmatch $DsmVersionPattern) { Exit-Build "staged build.env of $Family has no valid DSM_VERSION ('$dsmVersion')" }
    $stagedRevision = [int]"$($dsmVersion[-1])"
    if ($stagedRevision -ne $PackageRevision) {
        Exit-Build "staged DSM_VERSION $dsmVersion of $Family encodes package revision $stagedRevision, not $PackageRevision"
    }
    $suffix = if ($stagedRevision -eq 0) { '' } else { "-r$stagedRevision" }
    return @{ Packed = "$id-$Family-$dsmVersion.spk"; Target = "$id-$serverVersion$suffix-$Family.spk" }
}

# Packs one family with the toolkit and renames the toolkit's <id>-<family>-<dsm>.spk.
function Invoke-PackFamily([string] $Family) {
    $watch = [Diagnostics.Stopwatch]::StartNew()
    $name = Get-SpkName $Family
    $project = Join-Path $Stage $Family
    Write-BuildLog "step 2: packing $Family with the toolkit"
    $arguments = @('run', '--rm') + $script:DockerPrivilege + @(
        '-v', "${Volume}:/toolkit", '-v', "${project}:/project:ro", '-v', "${Dist}:/output", $Image, $Family
    )
    Invoke-Docker "toolkit run for $Family" $arguments
    $packed = Join-Path $Dist $name.Packed
    if (-not (Test-Path -LiteralPath $packed -PathType Leaf)) { Exit-Build "the toolkit did not write $($name.Packed)" }
    Move-Item -LiteralPath $packed -Destination (Join-Path $Dist $name.Target) -Force
    Write-BuildLog "wrote $($name.Target) in $([int]$watch.Elapsed.TotalSeconds)s"
}

# SHA256SUMS in the format of `sha256sum` (hash, two blanks, name), sorted by name, LF endings.
function Write-Sha256Sum {
    [string[]] $names = @(Get-ChildItem -LiteralPath $Dist -File -Filter '*.spk' | ForEach-Object Name)
    if ($names.Count -ne $Families.Count) { Exit-Build "expected $($Families.Count) SPKs in $Dist, found $($names.Count)" }
    [Array]::Sort($names, [StringComparer]::Ordinal)
    $lines = foreach ($name in $names) {
        $hash = (Get-FileHash -Algorithm SHA256 -LiteralPath (Join-Path $Dist $name)).Hash.ToLowerInvariant()
        "$hash  $name"
    }
    Write-LfFile (Join-Path $Dist $SumsName) $lines
    $lines | ForEach-Object { [Console]::Error.WriteLine($_) }
}

$watch = [Diagnostics.Stopwatch]::StartNew()
$script:RevisionGiven = $PSBoundParameters.ContainsKey('Revision')
Resolve-BuildInput
$interimFamilies = Get-InterimFamily
Assert-ReleaseInput $interimFamilies
if (-not (Get-Command docker -ErrorAction SilentlyContinue)) { Exit-Build 'docker not found' }
Initialize-Dist
Write-BuildLog "building the toolkit image $Image"
Invoke-Docker 'toolkit image build' @('build', '-q', '-t', $Image, (Join-Path $PSScriptRoot 'toolkit')) -Quiet
Invoke-StageStep
Assert-StagedProject
foreach ($family in $Families) { Invoke-PackFamily $family }
Write-Sha256Sum
Write-BuildLog "done in $([int]$watch.Elapsed.TotalSeconds)s ($script:Mode build): $Dist"
if ($script:Mode -ne 'release' -and $interimFamilies) { Write-InterimBanner $interimFamilies }
