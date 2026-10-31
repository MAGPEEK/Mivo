#!/bin/bash
# Tests of the toolkit walking skeleton (SPK-04). Needs Docker (Docker Desktop on Windows with
# Git Bash, or Linux); the pack tests run the toolkit container with CAP_SYS_ADMIN and download
# the platform environments on their first run (README.md "Caching").
#
#   test.sh            lint, build-stage checks, pack and verify all three families
#   test.sh lint       shellcheck only
#   test.sh x86_64 ...   lint, build-stage checks and the named families only
#
# SPK_TOOLKIT_PRIVILEGE replaces the privilege options of the toolkit container on AppArmor
# hosts: "--cap-add SYS_ADMIN --security-opt apparmor=unconfined", "--privileged" as last resort.
set -euo pipefail

here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "$here/../../.." && pwd)"
readonly IMAGE="${SPK_TOOLKIT_IMAGE:-spk-toolkit:dev}"
readonly VOLUME="${SPK_TOOLKIT_VOLUME:-spk-toolkit}"
read -ra PRIVILEGE <<<"${SPK_TOOLKIT_PRIVILEGE:---cap-add SYS_ADMIN}"
readonly PRIVILEGE
readonly SHELLCHECK_IMAGE="koalaman/shellcheck:v0.11.0"
readonly FIXTURE_ID="demo"
readonly FIXTURE_DSM_VERSION="1.2.3-1040"
readonly CHROOT_SOURCE="/source/$FIXTURE_ID"
# build.env lines with an apostrophe in the value, unescaped and shell-escaped; the contract
# has no apostrophe at all, so run.sh refuses both (used by snippets pack_variant evaluates).
# shellcheck disable=SC2034
readonly APOSTROPHE_LINES=("DISPLAY_NAME='It's a demo'" "DISPLAY_NAME='It'\\''s a demo'")
# Project files run.sh checks for CR characters, beyond INFO.sh, build.env and SynoBuildConf/*
# (name|path in the staged project); pack_failure_checks appends a CRLF line to each.
readonly CRLF_FILES=(
	'script|scripts/postinst'
	'conf|conf/privilege'
	'wizard-script|WIZARD_UIFILES/install_uifile.sh'
	'port-file|prebuilt/port_conf/demo.sc'
)
readonly CANNOT_CARRY="^Error: INFO.sh: build.env DISPLAY_NAME contains characters INFO cannot carry"

# Characters INFO.sh must reject (label|value as written into build.env through sed, where
# \\ is one backslash, \t a tab and \n a newline).
# shellcheck disable=SC2016 # the values are literal on purpose
readonly REJECTED_VALUES=(
	'dollar sign|a$b'
	'backquote|a`b'
	'backslash|a\\b'
	'asterisk|a*b'
	'question mark|a?b'
	'opening bracket|a[b'
	'double quote|a"b'
	'double space|a  b'
	'tab|a\tb'
	'newline|a\nb'
	"apostrophe|a'\\\\''b"
)

# Git Bash would rewrite container paths such as /project into Windows paths.
export MSYS_NO_PATHCONV=1

work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT
failures=0

# Docker on Windows needs C:/... paths for bind mounts; cygpath exists only in Git Bash.
host_path() {
	if command -v cygpath >/dev/null 2>&1; then cygpath -m "$1"; else printf '%s' "$1"; fi
}

pass() { printf 'ok   %s\n' "$*"; }
fail() {
	printf 'FAIL %s\n' "$*"
	failures=$((failures + 1))
}

# Passes when the output matches the pattern (or, with "!" as fourth argument, does not).
expect_match() {
	local name="$1" output="$2" pattern="$3" negate="${4:-}"
	if grep -q -- "$pattern" <<<"$output"; then
		if [ -z "$negate" ]; then pass "$name"; else fail "$name -> $output"; fi
	elif [ -n "$negate" ]; then pass "$name"; else fail "$name -> $output"; fi
}

platform_of() {
	case "$1" in x86_64) echo v1000 ;; armv8) echo rtd1619b ;; armv7) echo alpine ;; esac
}

lint() {
	local src
	src="$(host_path "$repo_root")"
	# SynoBuildConf/depends is an INI file read by the toolkit's configparser, not shell.
	if docker run --rm -v "$src:/src:ro" -w /src "$SHELLCHECK_IMAGE" -s bash \
		packaging/synology/toolkit/run.sh packaging/synology/toolkit/test.sh \
		packaging/synology/toolkit/testdata/stage-demo.sh \
		packaging/synology/package/SynoBuildConf/build packaging/synology/package/SynoBuildConf/install \
		packaging/synology/package/INFO.sh &&
		docker run --rm -v "$src:/src:ro" -w /src "$SHELLCHECK_IMAGE" -s sh \
			packaging/synology/toolkit/testdata/demo/scripts/*; then
		pass "shellcheck"
	else
		fail "shellcheck"
	fi
}

stage() { bash "$here/testdata/stage-demo.sh" "$1" "$2" >/dev/null; }

# Runs SynoBuildConf/build the way the toolkit does (sourced by bash in a subshell) on a copy
# of the project inside a container, where modes are real (a Windows bind mount shows 0777).
# $2 is a shell snippet that breaks the copy first.
build_stage_output() {
	docker run --rm --entrypoint bash -v "$(host_path "$1"):/project:ro" "$IMAGE" -c \
		'cp -R /project /tmp/p && cd /tmp/p && eval "$1" && (. SynoBuildConf/build) 2>&1' _ "$2" || true
}

expect_build_error() {
	local name="$1" project="$2" breakage="$3" message="$4"
	expect_match "build stage: $name" "$(build_stage_output "$project" "$breakage")" "^Error: .*$message"
}

build_stage_checks() {
	local project="$work/build-x86_64" armv7_project="$work/build-armv7" output
	stage x86_64 "$project"
	stage armv7 "$armv7_project"
	output="$(build_stage_output "$project" true)"
	expect_match "build stage: complete x86_64 project" "$output" '^prebuilt files for x86_64 present'
	expect_match "build stage: complete x86_64 project prints no Error" "$output" 'Error' '!'
	expect_match "build stage: arch list, family from the first word" \
		"$(build_stage_output "$armv7_project" "sed -i \"s/^ARCH=.*/ARCH='armv7 armada38x monaco'/\" build.env")" \
		'^prebuilt files for armv7 present'
	expect_build_error "missing ffprobe" "$project" 'rm prebuilt/bin/ffprobe' \
		'prebuilt/bin/ffprobe for x86_64 is missing'
	expect_build_error "server not executable" "$project" "chmod 0644 prebuilt/bin/$FIXTURE_ID" \
		"prebuilt/bin/$FIXTURE_ID for x86_64 is not executable"
	expect_build_error "wrong machine" "$project" "sed -i s/x86_64/armv8/ build.env" \
		'not an ELF executable for armv8'
	expect_build_error "unknown family" "$project" "sed -i s/x86_64/mips/ build.env" \
		"ARCH 'mips' is not a family"
	expect_build_error "no licences" "$project" 'rm -r prebuilt/share/licenses' 'holds no licence files'
	expect_build_error "no firewall service file" "$project" 'rm -r prebuilt/port_conf' \
		"prebuilt/port_conf/$FIXTURE_ID.sc (firewall service file) is missing"
}

# Runs the toolkit container; prints its output and returns its exit status. Arguments after
# the third are extra `docker run` options.
run_toolkit() {
	local family="$1" project="$2" output="$3"
	mkdir -p "$output"
	docker run --rm "${PRIVILEGE[@]}" "${@:4}" -v "$VOLUME:/toolkit" \
		-v "$(host_path "$project"):/project:ro" -v "$(host_path "$output"):/output" \
		"$IMAGE" "$family" 2>&1
}

# Asserts members, modes and INFO of one SPK inside the toolkit image (GNU tar with xz).
verify_spk() {
	local family="$1" output="$2" spk="$FIXTURE_ID-$1-$FIXTURE_DSM_VERSION.spk" result
	if [ ! -f "$output/$spk" ]; then
		fail "$family: $spk not produced"
		return
	fi
	result="$(docker run --rm --entrypoint bash -v "$(host_path "$output"):/output:ro" "$IMAGE" -c '
		spk="/output/$1"; family="$2"; id="$3"; version="$4"; problems=""
		members="$(tar -tf "$spk")"
		for member in INFO package.tgz scripts/ scripts/start-stop-status conf/ conf/privilege LICENSE; do
			grep -qx "$member" <<<"$members" || problems="$problems missing:$member"
		done
		grep -q "signature" <<<"$members" && problems="$problems signature-member"
		info="$(tar -xOf "$spk" INFO)"
		for line in "package=\"$id\"" "version=\"$version\"" "arch=\"$family\"" "beta=\"yes\"" \
			"adminport=\"9999\"" "adminprotocol=\"http\"" "support_move=\"no\""; do
			grep -qx "$line" <<<"$info" || problems="$problems info:$line"
		done
		grep -q "^adminurl=" <<<"$info" && problems="$problems info:adminurl-present"
		modes="$(tar -xOf "$spk" package.tgz | tar -tvJf -)"
		for entry in "-rwxr-xr-x bin/$id" "-rwxr-xr-x bin/ffmpeg" "-rwxr-xr-x bin/ffprobe" \
			"-rw-r--r-- share/licenses/ffmpeg/LICENSE.md" "-rw-r--r-- share/licenses/ffmpeg/SOURCE.txt" \
			"-rw-r--r-- port_conf/$id.sc"; do
			awk -v want="$entry" "{ sub(/^\.\//, \"\", \$6); if (\$1 \" \" \$6 == want) found = 1 } END { exit !found }" <<<"$modes" ||
				problems="$problems package.tgz:$entry"
		done
		echo "${problems:-none}"
	' _ "$spk" "$family" "$FIXTURE_ID" "$FIXTURE_DSM_VERSION")"
	if [ "$result" = "none" ]; then pass "$family: $spk members, modes and INFO"; else fail "$family: $spk ->$result"; fi
}

# Runs INFO.sh inside the deployed chroot on the project the pack run left in source/, once
# unchanged and with broken build.env values. $2 is a sed expression applied to a copy.
info_sh_output() {
	local platform="$1" breakage="$2"
	# A unique copy per call: a fixed path let the clean-up of one call race the next one.
	docker run --rm --entrypoint bash -v "$VOLUME:/toolkit" "$IMAGE" -c '
		root="/toolkit/build_env/ds.$1-7.2"
		copy="$(mktemp -d "$root/tmp/info-test.XXXXXX")" || exit 1
		cp -R "$root$2/." "$copy/" && sed -i "$3" "$copy/build.env"
		chroot "$root" bash -c "cd ${copy#"$root"} && bash ./INFO.sh" 2>&1
		rm -rf "$copy"
	' _ "$platform" "$CHROOT_SOURCE" "$breakage" || true
}

expect_info_sh() {
	local name="$1" platform="$2" breakage="$3" pattern="$4" negate="${5:-}"
	expect_match "$name" "$(info_sh_output "$platform" "$breakage")" "$pattern" "$negate"
}

info_sh_checks() {
	local family="$1" platform
	platform="$(platform_of "$family")"
	expect_info_sh "$family: INFO.sh in the chroot" "$platform" 's/^$//' "^arch=\"$family\"$"
	expect_info_sh "$family: INFO.sh rejects a foreign family" "$platform" "s/^ARCH=.*/ARCH='i686'/" \
		"^Error: INFO.sh: build.env ARCH 'i686' does not start with a family"
	if [ "$family" = armv7 ]; then
		# The arch list of ADR 0026 Notes for the armv7 SPK.
		expect_info_sh "armv7: INFO.sh writes the arch list" "$platform" \
			"s/^ARCH=.*/ARCH='armv7 armada38x monaco'/" '^arch="armv7 armada38x monaco"$'
		expect_info_sh "armv7: INFO.sh rejects an unknown arch value" "$platform" \
			"s/^ARCH=.*/ARCH='armv7 armada370'/" "'armada370' is not an arch value for family armv7"
	else
		expect_info_sh "$family: INFO.sh rejects armada38x outside armv7" "$platform" \
			"s/^ARCH=.*/ARCH='$family armada38x'/" "'armada38x' is not an arch value for family $family"
	fi
}

# Table of rejected characters and value formats; run once, on the x86_64 environment.
info_sh_value_checks() {
	local platform="v1000" entry label value
	for entry in "${REJECTED_VALUES[@]}"; do
		label="${entry%%|*}"
		value="${entry#*|}"
		expect_info_sh "INFO.sh rejects a $label" "$platform" \
			"s/^DISPLAY_NAME=.*/DISPLAY_NAME='$value'/" "$CANNOT_CARRY"
	done
	for value in 0 65536 08460 84a; do
		expect_info_sh "INFO.sh rejects PORT '$value'" "$platform" "s/^PORT=.*/PORT='$value'/" \
			"^Error: INFO.sh: build.env PORT '$value' is not a TCP port"
	done
	for value in 1.2.3 1.2.3-1040x 1.2-10/../x -1040; do
		expect_info_sh "INFO.sh rejects DSM_VERSION '$value'" "$platform" \
			"s|^DSM_VERSION=.*|DSM_VERSION='$value'|" "^Error: INFO.sh: build.env DSM_VERSION"
	done
	for value in 7.2 7-64570 7.2.2-72806; do
		expect_info_sh "INFO.sh rejects OS_MIN_VER '$value'" "$platform" \
			"s/^OS_MIN_VER=.*/OS_MIN_VER='$value'/" "^Error: INFO.sh: build.env OS_MIN_VER"
	done
	expect_info_sh "INFO.sh writes no beta key for a release" "$platform" "s/^BETA=.*/BETA='no'/" '^beta=' '!'
	expect_info_sh "INFO.sh writes no adminurl" "$platform" 's/^$//' '^adminurl=' '!'
}

pack_family() {
	local family="$1" project="$work/project-$1" output="$work/output-$1" log
	stage "$family" "$project"
	if log="$(run_toolkit "$family" "$project" "$output")"; then
		pass "$family: toolkit run"
	else
		fail "$family: toolkit run"
		printf '%s\n' "$log" | tail -n 40
		return
	fi
	verify_spk "$family" "$output"
	info_sh_checks "$family"
	[ "$family" != x86_64 ] || info_sh_value_checks
	# Second run of the same family without network: the cached environment is reused and
	# nothing can be downloaded.
	if log="$(run_toolkit "$family" "$project" "$work/output-$family-again" --network none)" &&
		grep -q "environment ds.$(platform_of "$family")-7.2 deployed, EnvDeploy skipped" <<<"$log" &&
		[ -f "$work/output-$family-again/$FIXTURE_ID-$family-$FIXTURE_DSM_VERSION.spk" ]; then
		pass "$family: second run without network reuses the environment"
	else
		fail "$family: second run -> $(tail -n 20 <<<"$log")"
	fi
}

# Toolkit runs on broken or unusual x86_64 projects; $1 names the case, $2 is a shell snippet
# run in the staged project directory.
pack_variant() {
	local name="$1" project="$work/project-$1"
	stage x86_64 "$project"
	(cd "$project" && eval "$2")
	run_toolkit x86_64 "$project" "$work/output-$name"
}

pack_failure_checks() {
	local log
	if log="$(pack_variant missing-ffprobe 'rm prebuilt/bin/ffprobe')"; then
		fail "toolkit run with a missing prebuilt file succeeded"
	else
		expect_match "toolkit run fails on a missing prebuilt file" "$log" \
			"Error: prebuilt file prebuilt/bin/ffprobe for x86_64 is missing"
	fi
	# shellcheck disable=SC2016 # the command substitution must reach build.env unexpanded
	if log="$(pack_variant command-in-build-env 'echo "ID=\$(touch /tmp/sourced)" >>build.env')"; then
		fail "toolkit run with a command in build.env succeeded"
	else
		expect_match "toolkit run refuses build.env lines that are not literal assignments" "$log" \
			"build.env lines other than NAME='literal' assignments"
	fi
	local index
	for index in "${!APOSTROPHE_LINES[@]}"; do
		# shellcheck disable=SC2016 # expanded by eval in pack_variant
		if log="$(pack_variant "apostrophe-$index" \
			'sed -i "/^DISPLAY_NAME=/d" build.env && printf "%s\n" "${APOSTROPHE_LINES['"$index"']}" >>build.env')"; then
			fail "toolkit run with ${APOSTROPHE_LINES[$index]} in build.env succeeded"
		else
			expect_match "toolkit run refuses ${APOSTROPHE_LINES[$index]} in build.env" "$log" \
				"build.env lines other than NAME='literal' assignments"
		fi
	done
	if log="$(pack_variant unknown-field "echo \"EXTRA='x'\" >>build.env")"; then
		fail "toolkit run with an unknown build.env field succeeded"
	else
		expect_match "toolkit run refuses build.env fields outside the known twelve" "$log" \
			"build.env fields other than ID .* PORT: EXTRA"
	fi
	if log="$(pack_variant duplicate-field "echo \"PORT='8460'\" >>build.env")"; then
		fail "toolkit run with a field assigned twice succeeded"
	else
		expect_match "toolkit run refuses a build.env field assigned twice" "$log" \
			"build.env assigns these fields more than once: PORT"
	fi
	if log="$(pack_variant missing-field "sed -i '/^VERSION=/d' build.env")"; then
		fail "toolkit run with a missing build.env field succeeded"
	else
		expect_match "toolkit run refuses a build.env without one of the twelve fields" "$log" \
			"build.env lacks these fields: VERSION"
	fi
	local entry name file
	for entry in "${CRLF_FILES[@]}"; do
		name="${entry%%|*}"
		file="${entry#*|}"
		if log="$(pack_variant "crlf-$name" "mkdir -p \"\$(dirname '$file')\" && printf '#!/bin/sh\r\n' >>'$file'")"; then
			fail "toolkit run with CRLF in $file succeeded"
		else
			expect_match "toolkit run refuses CRLF in $file" "$log" "CRLF line endings in $file;"
		fi
	done
}

# run.sh against a throwaway toolkit root inside the container (the clone copied from the
# volume, fake tarballs, a deploy marker of another toolkit commit): it must redeploy because of
# the commit, then refuse the tarball whose SHA-256 differs from tarballs.sha256 and delete it.
pin_checks() {
	local output
	output="$(docker run --rm --entrypoint bash -v "$VOLUME:/volume:ro" -e TOOLKIT_ROOT=/tmp/root "$IMAGE" -c '
		mkdir -p /tmp/root/.state /tmp/root/toolkit_tarballs /tmp/root/build_env/ds.v1000-7.2 /tmp/project
		cp -a /volume/pkgscripts-ng /tmp/root/
		touch /tmp/root/build_env/ds.v1000-7.2/PkgVersion
		echo "0000000000000000000000000000000000000000 2026-01-01T00:00:00Z" >/tmp/root/.state/ds.v1000-7.2.deployed
		for file in base_env-7.2.txz ds.v1000-7.2.dev.txz ds.v1000-7.2.env.txz; do
			echo tampered >"/tmp/root/toolkit_tarballs/$file"
		done
		touch /tmp/root/.state/base_env-7.2.downloaded /tmp/root/.state/ds.v1000-7.2.downloaded
		spk-toolkit-run x86_64 /tmp/project /tmp/out 2>&1
		echo "exit=$?"
		[ -e /tmp/root/toolkit_tarballs/base_env-7.2.txz ] || echo "tampered tarball deleted"
		[ -e /tmp/root/.state/base_env-7.2.downloaded ] || echo "download marker deleted"
	')" || true
	expect_match "run.sh redeploys an environment of another toolkit commit" "$output" \
		"deployed by toolkit commit 0000000000000000000000000000000000000000; deploying again"
	expect_match "run.sh refuses a tarball that does not match tarballs.sha256" "$output" \
		"failed: SHA-256 of base_env-7.2.txz is .*, pinned ff93a412"
	expect_match "run.sh deletes the mismatching tarball and its marker" "$output" "download marker deleted"
}

main() {
	local families=("$@") family
	lint
	if [ "${families[0]:-}" = "lint" ]; then
		[ "$failures" -eq 0 ]
		return
	fi
	[ "${#families[@]}" -gt 0 ] || families=(x86_64 armv8 armv7)
	docker build -q -t "$IMAGE" "$(host_path "$here")" >/dev/null
	build_stage_checks
	for family in "${families[@]}"; do
		pack_family "$family"
	done
	pack_failure_checks
	pin_checks
	if [ "$failures" -eq 0 ]; then echo "all toolkit tests passed"; else echo "$failures toolkit test(s) failed"; fi
	[ "$failures" -eq 0 ]
}

main "$@"
