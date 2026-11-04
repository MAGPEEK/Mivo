#Requires -Version 7.2
<#
.SYNOPSIS
Builds the server SPKs of all three architecture families (x86_64, armv8, armv7) with Docker.

.DESCRIPTION
PowerShell twin of build.sh (ADR 0026 section 2, task SPK-07): same steps, options and output.
Nothing but Docker and PowerShell 7.2 or newer is needed on the host.

Step 0 (docker build of packaging/synology/ffmpeg/build, unprivileged): the own static FFmpeg
of every family and its corresponding source archive -> the local image SPK_FFMPEG_IMAGE
(cached by BuildKit; the first build takes 10-14 minutes, ffmpeg/build/README.md).
Step 1 (docker build of packaging/synology/Dockerfile, unprivileged): web app, server binaries,
FFmpeg from that image and the staged projects -> packaging/synology/dist/stage/<family>/, the
source archive -> dist/. The staged ffmpeg and ffprobe must match the pins of
ffmpeg/sources.lock, the archive the Source SHA-256 of every SOURCE.txt.
Step 2 (docker run of the toolkit image with CAP_SYS_ADMIN, one family after the other because
the toolkit volume serialises runs): packs each project -> dist/<id>-<version>-<family>.spk
(package revision N >= 1: <id>-<version>-r<N>-<family>.spk), then dist/SHA256SUMS over the SPKs
and the source archive. The first error stops the build.

A test build (the default outside CI) packs an interim or unpinned FFmpeg with a loud warning
and writes dist/NOT-FOR-RELEASE.txt. A release build (-Release; the default on CI hosts)
refuses interim or non-free FFmpeg builds, FFmpeg binaries that differ from the pins of
ffmpeg/sources.lock, spkstage warnings, the default version 0.0.0-dev and, when git is present,
a dirty working tree.

Empty values: a parameter given with an empty value is refused; an empty environment variable
counts as unset. A host counts as CI when CI is set to anything but empty, "false" or "0".

Environment: SPK_BUILD_MODE (release or test), SPK_PRIVILEGE (see -Privilege),
SOURCE_DATE_EPOCH, SPK_TOOLKIT_VOLUME (default spk-toolkit), SPK_TOOLKIT_IMAGE (default
spk-toolkit:dev), SPK_FFMPEG_IMAGE (default spk-ffmpeg-static:dev, the local image of the FFmpeg
recipe); NODE_IMAGE and GO_IMAGE are passed to the Dockerfile when set; BUILDKIT_PROGRESS=plain
shows every log line of steps 0 and 1.

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
$FfmpegBinPath = 'prebuilt/bin'
$SourceStageName = 'ffmpeg-source'
$SourceArchiveSuffix = '-source.tar'
$LockFieldCount = 6
$Sha256Pattern = '^[0-9a-f]{64}$'
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
$FfmpegRecipe = Join-Path $PSScriptRoot 'ffmpeg/build'
$Volume = if ($env:SPK_TOOLKIT_VOLUME) { $env:SPK_TOOLKIT_VOLUME } else { 'spk-toolkit' }
$Image = if ($env:SPK_TOOLKIT_IMAGE) { $env:SPK_TOOLKIT_IMAGE } else { 'spk-toolkit:dev' }
$FfmpegImage = if ($env:SPK_FFMPEG_IMAGE) { $env:SPK_FFMPEG_IMAGE } else { 'spk-ffmpeg-static:dev' }

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

# Reads the one sources.lock entry of every family (format in the lock's header) into a table
# family -> entry; a malformed lock stops the build before anything runs.
function Read-Lock {
    if (-not (Test-Path -LiteralPath $SourcesLock -PathType Leaf)) { Exit-Build "$SourcesLock not found" }
    $lock = @{}
    foreach ($line in Get-Content -LiteralPath $SourcesLock) {
        if ($line -match '^\s*(#|$)') { continue }
        $fields = -split $line
        $family = $fields[0]
        if ($fields.Count -ne $LockFieldCount) {
            Exit-Build "ffmpeg/sources.lock: the entry of $family has $($fields.Count) fields, expected $LockFieldCount"
        }
        if ($lock.ContainsKey($family)) { Exit-Build "ffmpeg/sources.lock has two entries for $family" }
        if ($fields[1] -cnotin @('selected', 'interim')) { Exit-Build "ffmpeg/sources.lock: unknown status '$($fields[1])' for $family" }
        if ($fields[4] -cnotmatch $Sha256Pattern -or $fields[5] -cnotmatch $Sha256Pattern) {
            Exit-Build "ffmpeg/sources.lock: the entry of $family has no lowercase SHA-256 for ffmpeg and ffprobe"
        }
        $lock[$family] = @{ Status = $fields[1]; Version = $fields[2]; Licence = $fields[3]; Ffmpeg = $fields[4]; Ffprobe = $fields[5] }
    }
    foreach ($family in $Families) {
        if (-not $lock.ContainsKey($family)) { Exit-Build "ffmpeg/sources.lock has no entry for $family" }
    }
    return $lock
}

# Families whose sources.lock entry is interim (same test as SPK-16's acceptance check).
function Get-InterimFamily([hashtable] $Lock) {
    return @($Families | Where-Object { $Lock[$_].Status -ceq 'interim' }) -join ' '
}

function Write-InterimBanner([string] $InterimFamilies) {
    $rule = '=' * 90
    @(
        $rule
        'WARNING: TEST BUILD WITH INTERIM FFMPEG - NOT FOR RELEASE OR DISTRIBUTION'
        "  ffmpeg/sources.lock has interim entries for: $InterimFamilies"
        '  Interim builds miss criteria of ADR 0026 section 4 (static, licence, kernel); the'
        '  release build (-Release) refuses them.'
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
        Where-Object { $_.Name -like '*.spk' -or $_.Name -like "*$SourceArchiveSuffix" -or $_.Name -in @($SumsName, $MarkerName) } |
        Remove-Item -Force
}

# Step 0: the FFmpeg recipe's "all" target as a local image, which step 1 copies from. BuildKit
# keys every stage by its inputs, so an unchanged recipe is loaded from the cache in seconds.
function Invoke-FfmpegStep {
    $watch = [Diagnostics.Stopwatch]::StartNew()
    Write-BuildLog "step 0: FFmpeg (own static build of ffmpeg/build, cached by BuildKit) -> $FfmpegImage"
    Invoke-Docker 'step 0 (FFmpeg recipe build)' @('build', '-t', $FfmpegImage, $FfmpegRecipe)
    Write-BuildLog "step 0 finished in $([int]$watch.Elapsed.TotalSeconds)s"
}

function Get-BuildArgument {
    $releaseFlag = if ($script:Mode -eq 'release') { 'true' } else { 'false' }
    $arguments = @(
        '--build-arg', "VERSION=$Version", '--build-arg', "REVISION=$script:BuildRevision",
        '--build-arg', "PACKAGE_REVISION=$PackageRevision",
        '--build-arg', "SOURCE_DATE_EPOCH=$script:Epoch", '--build-arg', "RELEASE=$releaseFlag",
        '--build-arg', "FFMPEG_IMAGE=$FfmpegImage"
    )
    foreach ($name in @('NODE_IMAGE', 'GO_IMAGE')) {
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

# Returns the single value of a "Field:" line of a staged FFmpeg SOURCE.txt: everything after
# the colon, trimmed, so a word such as "nonfree" cannot hide behind the first one. A notice
# with no or several such lines is refused, since a second line could hide behind the first.
function Get-NoticeValue([string[]] $Lines, [string] $Notice, [string] $Field) {
    $matching = @($Lines | Where-Object { $_.StartsWith("${Field}:", [StringComparison]::Ordinal) })
    if ($matching.Count -ne 1) { Exit-Build "$Notice needs exactly one ${Field}: line (has $($matching.Count))" }
    $value = $matching[0].Substring($Field.Length + 1).Trim()
    if (-not $value) { Exit-Build "$Notice has an empty ${Field}: value" }
    return $value
}

# Returns status, licence, version and source SHA-256 of a staged FFmpeg SOURCE.txt.
function Get-FfmpegNotice([string] $Project) {
    $notice = Join-Path $Project $NoticePath
    if (-not (Test-Path -LiteralPath $notice -PathType Leaf)) { Exit-Build "$notice missing in the staged project" }
    $lines = @(Get-Content -LiteralPath $notice)
    return @{
        Status       = Get-NoticeValue $lines $notice 'Status'
        Licence      = Get-NoticeValue $lines $notice 'Licence'
        Version      = Get-NoticeValue $lines $notice 'Version'
        SourceSha256 = Get-NoticeValue $lines $notice 'Source SHA-256'
    }
}

function Get-Sha256([string] $Path) {
    return (Get-FileHash -Algorithm SHA256 -LiteralPath $Path).Hash.ToLowerInvariant()
}

# Returns the differences between a family's staged FFmpeg and its sources.lock entry: the
# SHA-256 of the staged ffmpeg and ffprobe and the version and licence of SOURCE.txt must be
# the pinned ones, so an SPK carries exactly the binaries that were verified under emulation.
function Get-PinDifference([string] $Family, [hashtable] $Entry, [hashtable] $Notice) {
    $differences = @()
    foreach ($tool in @('ffmpeg', 'ffprobe')) {
        $path = Join-Path (Join-Path $Stage $Family) "$FfmpegBinPath/$tool"
        if (-not (Test-Path -LiteralPath $path -PathType Leaf)) { Exit-Build "$path missing in the staged project" }
        $expected = if ($tool -eq 'ffmpeg') { $Entry.Ffmpeg } else { $Entry.Ffprobe }
        if ((Get-Sha256 $path) -cne $expected) { $differences += $tool }
    }
    if ($Notice.Version -cne $Entry.Version) { $differences += "version $($Notice.Version) (pinned $($Entry.Version))" }
    if ($Notice.Licence -cne $Entry.Licence) { $differences += "licence $($Notice.Licence) (pinned $($Entry.Licence))" }
    return $differences -join ' '
}

# Returns the one corresponding source archive step 1 staged, with its SHA-256; every
# SOURCE.txt must name it (Source SHA-256), so the archive next to the SPKs is the source of
# exactly their binaries.
function Get-SourceArchive {
    $directory = Join-Path $Stage $SourceStageName
    $archives = @()
    if (Test-Path -LiteralPath $directory -PathType Container) {
        $archives = @(Get-ChildItem -LiteralPath $directory -File -Filter "*$SourceArchiveSuffix")
    }
    if ($archives.Count -ne 1) { Exit-Build "step 1 staged $($archives.Count) FFmpeg source archives in $directory, expected 1" }
    return @{ Name = $archives[0].Name; Path = $archives[0].FullName; Sha256 = Get-Sha256 $archives[0].FullName }
}

# Reads what was staged, not what sources.lock says: the release check holds for any stage
# that supplies FFmpeg. Test builds list the families in NOT-FOR-RELEASE.txt.
function Assert-StagedProject([hashtable] $Lock) {
    $notEligible = @()
    $unpinned = @()
    $archive = Get-SourceArchive
    foreach ($family in $Families) {
        $project = Join-Path $Stage $family
        if (-not (Test-Path -LiteralPath (Join-Path $project 'build.env') -PathType Leaf)) {
            Exit-Build "step 1 staged no project for $family ($project)"
        }
        $notice = Get-FfmpegNotice $project
        if ($notice.Status -cne 'selected' -or $notice.Licence -like '*nonfree*') {
            $notEligible += "$family ($($notice.Status), $($notice.Licence))"
        }
        $differences = Get-PinDifference $family $Lock[$family] $notice
        if ($differences) { $unpinned += "$family ($differences)" }
        if ($archive.Sha256 -cne $notice.SourceSha256) {
            Exit-Build "the staged source archive $($archive.Name) ($($archive.Sha256)) is not the one SOURCE.txt of $family names ($($notice.SourceSha256))"
        }
    }
    if ($script:Mode -eq 'release') {
        if ($notEligible) { Exit-Build "release refused: FFmpeg builds not release-eligible: $($notEligible -join ' ')" }
        if ($unpinned) { Exit-Build "release refused: FFmpeg builds differ from the pins of ffmpeg/sources.lock: $($unpinned -join ' ')" }
        return $archive
    }
    if ($unpinned) {
        Write-BuildLog "WARNING: FFmpeg builds differ from the pins of ffmpeg/sources.lock (recipe changed? update the pins on purpose): $($unpinned -join ' ')"
    }
    $lines = @(
        'These SPKs come from a TEST BUILD (build.sh/build.ps1 without --release / -Release).'
        'Do not publish, upload or hand them out; build again with --release for that.'
    )
    if ($notEligible) { $lines += "FFmpeg builds that are not release-eligible (status, licence): $($notEligible -join ' ')" }
    if ($unpinned) { $lines += "FFmpeg builds that differ from the pins of ffmpeg/sources.lock: $($unpinned -join ' ')" }
    Write-LfFile (Join-Path $Dist $MarkerName) $lines
    return $archive
}

# Moves the corresponding source archive next to the SPKs (its .sha256 sidecar stays in
# stage/; SHA256SUMS covers the archive).
function Move-SourceArchive([hashtable] $Archive) {
    Move-Item -LiteralPath $Archive.Path -Destination (Join-Path $Dist $Archive.Name) -Force
    Write-BuildLog "FFmpeg corresponding source: $($Archive.Name)"
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

# SHA256SUMS in the format of `sha256sum` (hash, two blanks, name) over the SPKs and the FFmpeg
# source archive, sorted by name, LF endings.
function Write-Sha256Sum {
    [string[]] $spks = @(Get-ChildItem -LiteralPath $Dist -File -Filter '*.spk' | ForEach-Object Name)
    if ($spks.Count -ne $Families.Count) { Exit-Build "expected $($Families.Count) SPKs in $Dist, found $($spks.Count)" }
    [string[]] $names = $spks + @(Get-ChildItem -LiteralPath $Dist -File -Filter "*$SourceArchiveSuffix" | ForEach-Object Name)
    [Array]::Sort($names, [StringComparer]::Ordinal)
    $lines = foreach ($name in $names) {
        "$(Get-Sha256 (Join-Path $Dist $name))  $name"
    }
    Write-LfFile (Join-Path $Dist $SumsName) $lines
    $lines | ForEach-Object { [Console]::Error.WriteLine($_) }
}

$watch = [Diagnostics.Stopwatch]::StartNew()
$script:RevisionGiven = $PSBoundParameters.ContainsKey('Revision')
Resolve-BuildInput
$lock = Read-Lock
$interimFamilies = Get-InterimFamily $lock
Assert-ReleaseInput $interimFamilies
if (-not (Get-Command docker -ErrorAction SilentlyContinue)) { Exit-Build 'docker not found' }
Initialize-Dist
Write-BuildLog "building the toolkit image $Image"
Invoke-Docker 'toolkit image build' @('build', '-q', '-t', $Image, (Join-Path $PSScriptRoot 'toolkit')) -Quiet
Invoke-FfmpegStep
Invoke-StageStep
$sourceArchive = Assert-StagedProject $lock
Move-SourceArchive $sourceArchive
foreach ($family in $Families) { Invoke-PackFamily $family }
Write-Sha256Sum
Write-BuildLog "done in $([int]$watch.Elapsed.TotalSeconds)s ($script:Mode build): $Dist"
if ($script:Mode -ne 'release' -and $interimFamilies) { Write-InterimBanner $interimFamilies }
