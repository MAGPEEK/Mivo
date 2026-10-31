#!/usr/bin/env bats
# Tests of the lifecycle scripts preinst, postinst, preuninst, postuninst, preupgrade and
# postupgrade and of the common.sh helpers they use (task SPK-06). Each case runs once per shell
# of SCRIPT_SHELLS (helpers.bash); how to run them: ../README.md.

bats_require_minimum_version 1.11.0

load helpers
load ../fixtures/share-names

setup() {
	setup_package_tree
	CONFIG=$SYNOPKG_PKGVAR/config.yaml
}

teardown() {
	stop_leftover_processes
}

# run_lifecycle runs the package script $1 of the package tree with the remaining arguments.
run_lifecycle() {
	local script=$1
	shift
	run_script "$PKG_ROOT/scripts/$script" "$@"
}

# create_share creates the shared folder $1 on volume $2 (default 1) below the fake NAS root.
create_share() {
	mkdir -p "$BATS_TEST_TMPDIR/volume${2:-1}/$1"
}

# expected_config prints the config.yaml postinst writes for share $1 with the fixture package.
expected_config() {
	cat <<EOF
# Settings of $FIXTURE_DISPLAY_NAME, written once at installation; later installs and upgrades keep this file.
# More libraries: grant the package user read access to the shared folder in Control Panel,
# add an entry below and restart the package.
libraries:
  - name: "$1"
    path: "/var/packages/$FIXTURE_ID/shares/$1"
    kind: "movies"
EOF
}

# fill_user_data creates what a used installation has in var and home.
fill_user_data() {
	mkdir -p "$SYNOPKG_PKGVAR/data" "$SYNOPKG_PKGVAR/cache/artwork" "$SYNOPKG_PKGHOME/.cache/fontconfig"
	printf 'libraries:\n  - path: "/somewhere"\n' >"$CONFIG"
	echo database >"$SYNOPKG_PKGVAR/data/library.db"
	echo poster >"$SYNOPKG_PKGVAR/cache/artwork/poster.jpg"
	echo log >"$SYNOPKG_PKGVAR/$FIXTURE_ID.log"
	echo old log >"$SYNOPKG_PKGVAR/$FIXTURE_ID.log.1"
	echo hidden >"$SYNOPKG_PKGVAR/.hidden"
	echo font >"$SYNOPKG_PKGHOME/.cache/fontconfig/cache"
	create_share video
	echo movie >"$BATS_TEST_TMPDIR/volume1/video/movie.mkv"
	ln -s "$BATS_TEST_TMPDIR/volume1/video" "$SYNOPKG_PKGVAR/share-link"
	echo keep >"$PKG_ROOT/keep"
}

# tree_snapshot prints names, modes, sizes and checksums of everything below directory $1.
tree_snapshot() {
	(
		cd "$1" || exit 1
		find . -print | sort | while IFS= read -r entry; do
			if [[ -f $entry && ! -L $entry ]]; then
				printf '%s %s %s\n' "$entry" "$(stat -c %a "$entry")" "$(sha256sum <"$entry")"
			else
				printf '%s %s\n' "$entry" "$(stat -c %a "$entry")"
			fi
		done
	)
}

# user_data_snapshot covers var, home, the share and a file next to the package directories.
user_data_snapshot() {
	tree_snapshot "$SYNOPKG_PKGVAR"
	tree_snapshot "$SYNOPKG_PKGHOME"
	tree_snapshot "$BATS_TEST_TMPDIR/volume1/video"
	cat "$PKG_ROOT/keep"
}

assert_no_output() {
	assert_output ''
	assert_equal "$stderr" ""
}

# --- preinst -------------------------------------------------------------------------------

case_preinst_accepts_existing_share() {
	export SYNOPKG_PKG_STATUS=INSTALL
	local share
	create_share video
	create_share 'Filme und Serien' 2
	create_share 'Videos äöü ß$' 3
	for share in video 'Filme und Serien' 'Videos äöü ß$'; do
		export wizard_media_share=$share
		run_lifecycle preinst
		assert_success
		assert_no_output
	done
	[[ ! -s $TEMP_LOGFILE ]] || fail "preinst wrote a message: $(cat "$TEMP_LOGFILE")"
}

case_preinst_rejects_empty_share() {
	export SYNOPKG_PKG_STATUS=INSTALL
	create_share video
	run_lifecycle preinst
	assert_failure 1
	assert_file_line "$TEMP_LOGFILE" "Enter the name of the shared folder that holds your videos."
	refute_error_words

	export wizard_media_share=
	run_lifecycle preinst
	assert_failure 1
}

case_preinst_rejects_missing_share() {
	export SYNOPKG_PKG_STATUS=INSTALL wizard_media_share=movies
	create_share video
	# A file is no shared folder.
	touch "$BATS_TEST_TMPDIR/volume1/movies"
	run_lifecycle preinst
	assert_failure 1
	assert_stderr_contains 'The shared folder "movies" does not exist.'
	assert_file_line "$TEMP_LOGFILE" "$stderr"
	refute_error_words
	[[ ! -e $BATS_TEST_TMPDIR/volume1/movies/ ]] || fail "preinst created the share"
}

# A path that exists is still no share name: "/" and ".." would point the data-share resource
# and config.yaml at another directory.
case_preinst_rejects_invalid_share_name_even_if_the_path_exists() {
	export SYNOPKG_PKG_STATUS=INSTALL
	local share
	create_share video/movies
	for share in video/movies .. . ' video' $'video\n'; do
		export wizard_media_share=$share
		: >"$TEMP_LOGFILE"
		run_lifecycle preinst
		assert_failure 1
		assert_stderr_contains "is not a valid shared folder name"
	done
}

case_preinst_speaks_german() {
	export SYNOPKG_PKG_STATUS=INSTALL SYNOPKG_DSM_LANGUAGE=ger wizard_media_share=Filme
	run_lifecycle preinst
	assert_failure 1
	assert_file_line "$TEMP_LOGFILE" \
		'Der freigegebene Ordner "Filme" existiert nicht. Legen Sie ihn zuerst unter Systemsteuerung > Freigegebener Ordner an oder geben Sie den Namen eines vorhandenen freigegebenen Ordners ein.'
}

case_preinst_without_temp_logfile_still_refuses() {
	export SYNOPKG_PKG_STATUS=INSTALL wizard_media_share=movies
	unset SYNOPKG_TEMP_LOGFILE
	run_lifecycle preinst
	assert_failure 1
	assert_stderr_contains "does not exist"
}

case_preinst_skips_the_check_on_upgrade() {
	local status
	for status in UPGRADE ''; do
		export SYNOPKG_PKG_STATUS=$status
		unset wizard_media_share
		run_lifecycle preinst
		assert_success
		assert_no_output
		export wizard_media_share=missing
		run_lifecycle preinst
		assert_success
	done
}

# --- postinst ------------------------------------------------------------------------------

case_postinst_writes_config_on_install() {
	export SYNOPKG_PKG_STATUS=INSTALL wizard_media_share=video
	run_lifecycle postinst
	assert_success
	assert_output "wrote $CONFIG with the library \"video\" (/var/packages/$FIXTURE_ID/shares/video)"
	assert_equal "$stderr" ""
	refute_error_words
	assert_equal "$(cat "$CONFIG")" "$(expected_config video)"
	assert_mode "$CONFIG" 640
	assert_mode "$SYNOPKG_PKGVAR/data" 750
	assert_mode "$SYNOPKG_PKGVAR/cache" 750
	assert_mode "$SYNOPKG_PKGTMP/transcode" 750
	[[ ! -e $CONFIG.new ]] || fail "the temporary file is left"
}

case_postinst_quotes_special_share_names() {
	export SYNOPKG_PKG_STATUS=INSTALL
	local share
	for share in 'Filme und Serien' 'Videos äöü ß' 'Médiathèque$' '日本語'; do
		rm -f "$CONFIG"
		export wizard_media_share=$share
		run_lifecycle postinst
		assert_success
		assert_equal "$(cat "$CONFIG")" "$(expected_config "$share")"
	done
}

case_postinst_keeps_existing_config() {
	export SYNOPKG_PKG_STATUS=INSTALL wizard_media_share=video
	printf 'server:\n  port: 8460\n# the user edited this\n' >"$CONFIG"
	chmod 0600 "$CONFIG"
	local before
	before=$(tree_snapshot "$SYNOPKG_PKGVAR")
	run_lifecycle postinst
	assert_success
	assert_output "keeping the existing $CONFIG"
	refute_error_words
	assert_equal "$(tree_snapshot "$SYNOPKG_PKGVAR" | grep -v -e '^./data ' -e '^./cache ')" "$before"

	# A link the user made stays a link, even a dangling one.
	rm "$CONFIG"
	ln -s /nonexistent/config.yaml "$CONFIG"
	run_lifecycle postinst
	assert_success
	assert_equal "$(readlink "$CONFIG")" /nonexistent/config.yaml
}

case_postinst_writes_nothing_on_upgrade() {
	local status
	for status in UPGRADE ''; do
		export SYNOPKG_PKG_STATUS=$status wizard_media_share=video
		run_lifecycle postinst
		assert_success
		assert_no_output
		[[ ! -e $CONFIG ]] || fail "postinst wrote config.yaml with status '$status'"
		assert_mode "$SYNOPKG_PKGTMP/transcode" 750
	done
}

case_postinst_keeps_existing_directories() {
	export SYNOPKG_PKG_STATUS=UPGRADE
	mkdir -m 0700 "$SYNOPKG_PKGVAR/data"
	echo database >"$SYNOPKG_PKGVAR/data/library.db"
	run_lifecycle postinst
	assert_success
	assert_mode "$SYNOPKG_PKGVAR/data" 700
	assert_equal "$(cat "$SYNOPKG_PKGVAR/data/library.db")" database
}

case_postinst_rejects_invalid_share_name() {
	export SYNOPKG_PKG_STATUS=INSTALL
	local share
	for share in '' $'video\nlibraries: []' 'a"b' 'video/../..'; do
		export wizard_media_share=$share
		run_lifecycle postinst
		assert_failure 1
		assert_stderr_contains "was not written"
		[[ ! -e $CONFIG && ! -e $CONFIG.new ]] || fail "postinst wrote a config for '$share'"
	done
}

case_postinst_fails_when_config_cannot_be_written() {
	export SYNOPKG_PKG_STATUS=INSTALL wizard_media_share=video
	mkdir "$CONFIG.new"
	run_lifecycle postinst
	assert_failure 1
	assert_file_line "$TEMP_LOGFILE" "cannot write $CONFIG"
	[[ ! -e $CONFIG ]] || fail "a config.yaml was written"
}

case_postinst_fails_without_dsm_variables() {
	export SYNOPKG_PKG_STATUS=INSTALL wizard_media_share=video
	unset SYNOPKG_PKGVAR
	run_lifecycle postinst
	assert_failure 1
	assert_stderr_contains "SYNOPKG_PKGVAR is not set"
}

# --- postuninst ----------------------------------------------------------------------------

case_postuninst_deletes_data_only_when_asked_on_uninstall() {
	fill_user_data
	export SYNOPKG_PKG_STATUS=UNINSTALL wizard_delete_data=true
	run_lifecycle postuninst
	assert_success
	assert_output "deleted the data of $FIXTURE_DISPLAY_NAME in $SYNOPKG_PKGVAR and $SYNOPKG_PKGHOME"
	refute_error_words
	assert_equal "$(ls -A "$SYNOPKG_PKGVAR")" ""
	assert_equal "$(ls -A "$SYNOPKG_PKGHOME")" ""
	# Outside var and home nothing changed: the share behind a link, the package's neighbours.
	assert_equal "$(cat "$BATS_TEST_TMPDIR/volume1/video/movie.mkv")" movie
	assert_equal "$(cat "$PKG_ROOT/keep")" keep
	[[ -x $SYNOPKG_PKGDEST/bin/$FIXTURE_ID ]] || fail "postuninst touched target"
}

case_postuninst_keeps_data_for_every_other_combination() {
	fill_user_data
	local before status flag
	before=$(user_data_snapshot)
	for status in UNINSTALL UPGRADE INSTALL ''; do
		for flag in true false '' TRUE yes 1 unset; do
			[[ $status == UNINSTALL && $flag == true ]] && continue
			export SYNOPKG_PKG_STATUS=$status wizard_delete_data=$flag
			[[ $flag == unset ]] && unset wizard_delete_data
			run_lifecycle postuninst
			assert_success
			assert_no_output
			assert_equal "$(user_data_snapshot)" "$before"
		done
	done
}

case_postuninst_never_deletes_outside_the_package_directories() {
	fill_user_data
	export SYNOPKG_PKG_STATUS=UNINSTALL wizard_delete_data=true
	local before directory home=$SYNOPKG_PKGHOME
	mkdir -p "$BATS_TEST_TMPDIR/elsewhere"
	echo precious >"$BATS_TEST_TMPDIR/elsewhere/file"
	before=$(user_data_snapshot)
	for directory in "$BATS_TEST_TMPDIR/elsewhere" / "$PKG_ROOT/var/../../.." "$PKG_ROOT/missing"; do
		export SYNOPKG_PKGHOME=$directory
		run_lifecycle postuninst
		assert_failure 1
		assert_stderr_contains "not deleting $directory"
		assert_file_line "$TEMP_LOGFILE" "$stderr"
	done
	export SYNOPKG_PKGHOME=$home
	assert_equal "$(user_data_snapshot)" "$before"
	assert_equal "$(cat "$BATS_TEST_TMPDIR/elsewhere/file")" precious
}

case_postuninst_reports_data_it_cannot_delete() {
	fill_user_data
	export SYNOPKG_PKG_STATUS=UNINSTALL wizard_delete_data=true
	fake_failing_command rm '*/data'
	run_lifecycle postuninst
	assert_failure 1
	assert_stderr_contains "cannot remove $SYNOPKG_PKGVAR/data"
	assert_file_line "$TEMP_LOGFILE" "cannot delete all data of $FIXTURE_DISPLAY_NAME in $SYNOPKG_PKGVAR and $SYNOPKG_PKGHOME"
	[[ ! -e $CONFIG ]] || fail "the deletable rest was kept"
}

case_postuninst_without_dsm_variables_deletes_nothing() {
	fill_user_data
	export SYNOPKG_PKG_STATUS=UNINSTALL wizard_delete_data=true
	local before
	before=$(user_data_snapshot)
	local home=$SYNOPKG_PKGHOME
	unset SYNOPKG_PKGHOME
	run_lifecycle postuninst
	assert_failure 1
	assert_stderr_contains "SYNOPKG_PKGHOME is not set"
	export SYNOPKG_PKGHOME=$home
	rm "$PKG_ROOT/scripts/package.env"
	run_lifecycle postuninst
	assert_failure 1
	assert_equal "$(user_data_snapshot)" "$before"
}

# --- preuninst, preupgrade, postupgrade ----------------------------------------------------

case_other_scripts_keep_everything() {
	fill_user_data
	local before script status
	before=$(user_data_snapshot)
	for script in preuninst preupgrade postupgrade; do
		for status in UNINSTALL UPGRADE ''; do
			export SYNOPKG_PKG_STATUS=$status wizard_delete_data=true
			run_lifecycle "$script"
			assert_success
			assert_no_output
		done
	done
	assert_equal "$(user_data_snapshot)" "$before"
}

# --- whole operations in DSM's order (DSM-NOTES section 5 "Execution order") ---------------

case_install_then_start() {
	create_share video
	export SYNOPKG_PKG_STATUS=INSTALL wizard_media_share=video
	run_lifecycle preinst
	assert_success
	run_lifecycle postinst
	assert_success
	# DSM passes no wizard keys to start-stop-status.
	unset wizard_media_share
	run_sss start
	assert_success
	assert_file_line "$FAKE_STATE_DIR/server.env" "DEMO_CONFIG_DIR=$SYNOPKG_PKGVAR"
	assert_equal "$(cat "$CONFIG")" "$(expected_config video)"
	run_sss stop
	assert_success
}

# Upgrade: new preupgrade, old preuninst and postuninst, new preinst and postinst, new
# postupgrade, all with UPGRADE and without install or uninstall wizard keys; DSM replaces
# target and wipes tmp in between. A delete flag leaking into the upgrade changes nothing.
case_upgrade_order_keeps_user_data() {
	fill_user_data
	mkdir -p "$SYNOPKG_PKGTMP/transcode/session-1"
	local before script
	before=$(user_data_snapshot)
	export SYNOPKG_PKG_STATUS=UPGRADE wizard_delete_data=true
	for script in preupgrade preuninst postuninst; do
		run_lifecycle "$script"
		assert_success
		refute_error_words
	done
	rm -rf "${SYNOPKG_PKGTMP:?}"/*
	unset wizard_delete_data
	for script in preinst postinst postupgrade; do
		run_lifecycle "$script"
		assert_success
		refute_error_words
	done
	assert_equal "$(user_data_snapshot)" "$before"
	assert_mode "$SYNOPKG_PKGTMP/transcode" 750
}

# Uninstall with the box unchecked, then a new installation with another share: the old
# settings and database are used again.
case_uninstall_keeping_data_then_reinstall() {
	fill_user_data
	create_share Filme
	local before script
	before=$(user_data_snapshot)
	export SYNOPKG_PKG_STATUS=UNINSTALL wizard_delete_data=false
	for script in preuninst postuninst; do
		run_lifecycle "$script"
		assert_success
	done
	assert_equal "$(user_data_snapshot)" "$before"

	unset wizard_delete_data
	export SYNOPKG_PKG_STATUS=INSTALL wizard_media_share=Filme
	for script in preinst postinst; do
		run_lifecycle "$script"
		assert_success
	done
	assert_equal "$(user_data_snapshot)" "$before"
}

# --- common.sh helpers ---------------------------------------------------------------------

case_share_name_check_follows_dsm_rules() {
	local name
	for name in "${VALID_SHARE_NAMES[@]}"; do
		run_common pkg_check_share_name "$name"
		[[ $status -eq 0 ]] || fail "pkg_check_share_name rejects '$name' (status $status): $stderr"
	done
	for name in "${INVALID_SHARE_NAMES[@]}"; do
		run_common pkg_check_share_name "$name"
		[[ $status -eq 1 ]] || fail "pkg_check_share_name accepts '$name' (status $status)"
		assert_no_output
	done
}

case_yaml_quote_escapes_backslash_and_double_quote() {
	run_common pkg_yaml_quote 'a"b\c'
	assert_success
	assert_output '"a\"b\\c"'
	run_common pkg_yaml_quote 'Médiathèque $ # : -'
	assert_output '"Médiathèque $ # : -"'
	run_common pkg_yaml_quote ''
	assert_output '""'
}

case_package_directory_check_accepts_only_package_paths() {
	mkdir -p "$BATS_TEST_TMPDIR/appdata/$FIXTURE_ID" "$BATS_TEST_TMPDIR/demonstration"
	local directory
	for directory in "$SYNOPKG_PKGVAR" "$SYNOPKG_PKGHOME" "$BATS_TEST_TMPDIR/appdata/$FIXTURE_ID"; do
		run_common pkg_is_package_directory "$directory"
		assert_success
	done
	for directory in / '' relative/demo/var "$BATS_TEST_TMPDIR" "$BATS_TEST_TMPDIR/demonstration" \
		"$SYNOPKG_PKGVAR/../../.." "$SYNOPKG_PKGVAR/./missing" "$PKG_ROOT/missing" "$PKG_ROOT/keep"; do
		run_common pkg_is_package_directory "$directory"
		[[ $status -eq 1 ]] || fail "'$directory' passed as a package directory"
	done
}

register_cases \
	preinst_accepts_existing_share \
	preinst_rejects_empty_share \
	preinst_rejects_missing_share \
	preinst_rejects_invalid_share_name_even_if_the_path_exists \
	preinst_speaks_german \
	preinst_without_temp_logfile_still_refuses \
	preinst_skips_the_check_on_upgrade \
	postinst_writes_config_on_install \
	postinst_quotes_special_share_names \
	postinst_keeps_existing_config \
	postinst_writes_nothing_on_upgrade \
	postinst_keeps_existing_directories \
	postinst_rejects_invalid_share_name \
	postinst_fails_when_config_cannot_be_written \
	postinst_fails_without_dsm_variables \
	postuninst_deletes_data_only_when_asked_on_uninstall \
	postuninst_keeps_data_for_every_other_combination \
	postuninst_never_deletes_outside_the_package_directories \
	postuninst_reports_data_it_cannot_delete \
	postuninst_without_dsm_variables_deletes_nothing \
	other_scripts_keep_everything \
	install_then_start \
	upgrade_order_keeps_user_data \
	uninstall_keeping_data_then_reinstall \
	share_name_check_follows_dsm_rules \
	yaml_quote_escapes_backslash_and_double_quote \
	package_directory_check_accepts_only_package_paths

# --- static checks, independent of the shell -----------------------------------------------

@test "all seven package scripts exist and start with #!/bin/sh" {
	local script
	for script in preinst postinst preuninst postuninst preupgrade postupgrade start-stop-status; do
		assert_equal "$(head -n 1 "$PACKAGE_SCRIPTS/$script")" '#!/bin/sh'
	done
}
