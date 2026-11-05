#!/usr/bin/env bats
# Tests of the lifecycle scripts preinst, postinst, preuninst, postuninst, preupgrade and
# postupgrade and of the common.sh helpers they use (task SPK-06). Each case runs once per shell
# of SCRIPT_SHELLS (helpers.bash); how to run them: ../README.md.

bats_require_minimum_version 1.11.0

load helpers

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

case_preinst_asks_nothing() {
	local status
	for status in INSTALL UPGRADE ''; do
		export SYNOPKG_PKG_STATUS=$status
		run_lifecycle preinst
		assert_success
		assert_no_output
		# A leftover wizard key of an earlier version changes nothing.
		export wizard_media_share=missing
		run_lifecycle preinst
		assert_success
		assert_no_output
		unset wizard_media_share
	done
	[[ ! -s $TEMP_LOGFILE ]] || fail "preinst wrote a message: $(cat "$TEMP_LOGFILE")"
}

# --- postinst ------------------------------------------------------------------------------

case_postinst_creates_the_directories_and_writes_no_config() {
	local status
	for status in INSTALL UPGRADE ''; do
		rm -rf "$SYNOPKG_PKGVAR/data" "$SYNOPKG_PKGVAR/cache" "$SYNOPKG_PKGTMP/transcode"
		export SYNOPKG_PKG_STATUS=$status wizard_media_share=video
		run_lifecycle postinst
		assert_success
		assert_no_output
		[[ ! -e $CONFIG && ! -L $CONFIG && ! -e $CONFIG.new ]] || fail "postinst wrote config.yaml with status '$status'"
		assert_mode "$SYNOPKG_PKGVAR/data" 750
		assert_mode "$SYNOPKG_PKGVAR/cache" 750
		assert_mode "$SYNOPKG_PKGTMP/transcode" 750
	done
}

case_postinst_keeps_existing_config() {
	export SYNOPKG_PKG_STATUS=INSTALL
	printf 'server:\n  port: 8460\n# the user edited this\n' >"$CONFIG"
	chmod 0600 "$CONFIG"
	local before
	before=$(tree_snapshot "$SYNOPKG_PKGVAR")
	run_lifecycle postinst
	assert_success
	assert_no_output
	assert_equal "$(tree_snapshot "$SYNOPKG_PKGVAR" | grep -v -e '^./data ' -e '^./cache ')" "$before"

	# A link the user made stays a link, even a dangling one.
	rm "$CONFIG"
	ln -s /nonexistent/config.yaml "$CONFIG"
	run_lifecycle postinst
	assert_success
	assert_equal "$(readlink "$CONFIG")" /nonexistent/config.yaml
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

case_postinst_reports_directories_it_cannot_create() {
	export SYNOPKG_PKG_STATUS=INSTALL
	rm "$SYNOPKG_PKGTMP"
	run_lifecycle postinst
	assert_failure 1
	assert_file_line "$TEMP_LOGFILE" \
		"cannot create the directories of $FIXTURE_DISPLAY_NAME in $SYNOPKG_PKGVAR and $SYNOPKG_PKGTMP"
	: >"$TEMP_LOGFILE"
	export SYNOPKG_DSM_LANGUAGE=ger
	run_lifecycle postinst
	assert_failure 1
	assert_file_line "$TEMP_LOGFILE" \
		"Die Verzeichnisse von $FIXTURE_DISPLAY_NAME in $SYNOPKG_PKGVAR und $SYNOPKG_PKGTMP konnten nicht angelegt werden"
}

case_postinst_fails_without_dsm_variables() {
	export SYNOPKG_PKG_STATUS=INSTALL
	unset SYNOPKG_PKGVAR
	run_lifecycle postinst
	assert_failure 1
	assert_stderr_contains "SYNOPKG_PKGVAR is not set"
}

# --- upgrade from 0.1.0-alpha.2 (preupgrade, postupgrade) ------------------------------------

# alpha2_config prints the config.yaml 0.1.0-alpha.2's postinst wrote for share $1.
alpha2_config() {
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

# link_share creates the share directory $1 on volume $2 (default 1) and DSM's link to it, as
# the data-share resource of alpha.2 did.
link_share() {
	create_share "$1" "${2:-1}"
	mkdir -p "$PKG_ROOT/shares"
	ln -sfn "$BATS_TEST_TMPDIR/volume${2:-1}/$1" "$PKG_ROOT/shares/$1"
}

RECORD_FILE_NAME='share-links-before-upgrade.tsv'
BACKUP_FILE_NAME='config.yaml.bak-0.1.0-alpha.2'

case_preupgrade_records_the_targets_of_the_share_links() {
	export SYNOPKG_PKG_STATUS=UPGRADE
	link_share video
	link_share 'Filme und Serien' 2
	link_share 'Médiathèque' 3
	run_lifecycle preupgrade
	assert_success
	refute_error_words
	assert_short_output
	local record=$SYNOPKG_PKGVAR/$RECORD_FILE_NAME
	assert_equal "$(wc -l <"$record" | tr -d ' ')" 3
	assert_file_line "$record" $'video\t/volume1/video'
	assert_file_line "$record" $'Filme und Serien\t/volume2/Filme und Serien'
	assert_file_line "$record" $'Médiathèque\t/volume3/Médiathèque'
	[[ ! -e $record.new ]] || fail "the temporary file is left"
}

case_preupgrade_records_nothing_without_links() {
	export SYNOPKG_PKG_STATUS=UPGRADE
	echo stale >"$SYNOPKG_PKGVAR/$RECORD_FILE_NAME"
	create_share video
	# A directory in shares/ is no link, and without shares/ there is nothing to record.
	run_lifecycle preupgrade
	assert_success
	assert_no_output
	# The old record goes only when a new one replaces it.
	assert_file_line "$SYNOPKG_PKGVAR/$RECORD_FILE_NAME" stale
	mkdir -p "$PKG_ROOT/shares/plain-directory"
	run_lifecycle preupgrade
	assert_success
	assert_no_output
	refute_file_line "$SYNOPKG_PKGVAR/$RECORD_FILE_NAME" "plain-directory"
}

case_preupgrade_survives_a_record_it_cannot_write() {
	export SYNOPKG_PKG_STATUS=UPGRADE
	link_share video
	# A directory in place of the temporary file makes every write to it fail.
	mkdir "$SYNOPKG_PKGVAR/$RECORD_FILE_NAME.new"
	run_lifecycle preupgrade
	assert_success
	assert_stderr_contains "cannot write"
	[[ ! -e $SYNOPKG_PKGVAR/$RECORD_FILE_NAME ]] || fail "a partial record was written"
}

case_preupgrade_ignores_targets_outside_the_volumes() {
	export SYNOPKG_PKG_STATUS=UPGRADE
	mkdir -p "$PKG_ROOT/shares" "$BATS_TEST_TMPDIR/elsewhere/x"
	ln -s "$BATS_TEST_TMPDIR/elsewhere/x" "$PKG_ROOT/shares/outside"
	ln -s "$BATS_TEST_TMPDIR/volume1" "$PKG_ROOT/shares/whole-volume"
	run_lifecycle preupgrade
	assert_success
	assert_no_output
	[[ ! -e $SYNOPKG_PKGVAR/$RECORD_FILE_NAME ]] || fail "a foreign target was recorded"
}

case_postupgrade_keeps_config_when_the_new_file_cannot_be_written() {
	export SYNOPKG_PKG_STATUS=UPGRADE
	create_share video
	alpha2_config video >"$CONFIG"
	mkdir "$CONFIG.new"
	run_lifecycle postupgrade
	assert_success
	assert_stderr_contains "cannot convert"
	assert_equal "$(cat "$CONFIG")" "$(alpha2_config video)"
	[[ ! -e $SYNOPKG_PKGVAR/$BACKUP_FILE_NAME ]] || fail "a backup was made for a failed conversion"
}

case_postupgrade_ignores_dot_names_and_foreign_targets() {
	export SYNOPKG_PKG_STATUS=UPGRADE
	create_share video 2
	local link=/var/packages/$FIXTURE_ID/shares
	printf 'libraries:\n  - name: "a"\n    path: "%s/."\n  - name: "b"\n    path: "%s/.."\n' \
		"$link" "$link" >"$CONFIG"
	run_lifecycle postupgrade
	assert_success
	assert_equal "$(grep -c '/volume' "$CONFIG")" 0
	# A recorded target outside the volumes is not used; the volume directory is.
	alpha2_config video >"$CONFIG"
	printf 'video\t/etc\nvideo\t/volume1/../etc\n' >"$SYNOPKG_PKGVAR/$RECORD_FILE_NAME"
	run_lifecycle postupgrade
	assert_success
	assert_file_line "$CONFIG" '    path: "/volume2/video"'
}

case_postupgrade_rewrites_the_alpha2_path_and_keeps_a_backup() {
	export SYNOPKG_PKG_STATUS=UPGRADE
	link_share video
	alpha2_config video >"$CONFIG"
	local original old_path=/var/packages/$FIXTURE_ID/shares/video
	original=$(cat "$CONFIG")
	run_lifecycle preupgrade
	assert_success
	# DSM removes the link when the data-share resource goes away.
	rm -rf "$PKG_ROOT/shares"
	run_lifecycle postupgrade
	assert_success
	refute_error_words
	assert_stderr_contains "-> /volume1/video"
	assert_equal "$(cat "$CONFIG")" "${original/"$old_path"//volume1/video}"
	assert_equal "$(cat "$SYNOPKG_PKGVAR/$BACKUP_FILE_NAME")" "$original"
	[[ ! -e $SYNOPKG_PKGVAR/$RECORD_FILE_NAME && ! -e $CONFIG.new && ! -e $CONFIG.rewritten ]] ||
		fail "a temporary or record file is left"
	assert_mode "$CONFIG" 640

	# A second run finds nothing to rewrite and keeps the first backup.
	run_lifecycle postupgrade
	assert_success
	assert_no_output
	assert_equal "$(cat "$SYNOPKG_PKGVAR/$BACKUP_FILE_NAME")" "$original"
}

case_postupgrade_handles_names_with_spaces_and_non_ascii_letters() {
	export SYNOPKG_PKG_STATUS=UPGRADE
	local share
	for share in 'Filme und Serien' 'Médiathèque$' '日本語'; do
		rm -rf "${PKG_ROOT:?}/shares" "${SYNOPKG_PKGVAR:?}/$BACKUP_FILE_NAME"
		link_share "$share" 2
		alpha2_config "$share" >"$CONFIG"
		run_lifecycle preupgrade
		assert_success
		rm -rf "$PKG_ROOT/shares"
		run_lifecycle postupgrade
		assert_success
		assert_file_line "$CONFIG" "    path: \"/volume2/$share\""
		refute_file_line "$CONFIG" "    path: \"/var/packages/$FIXTURE_ID/shares/$share\""
		assert_file_line "$CONFIG" "  - name: \"$share\""
	done
}

case_postupgrade_without_a_record_uses_the_one_directory_of_that_name() {
	export SYNOPKG_PKG_STATUS=UPGRADE
	create_share video 2
	alpha2_config video >"$CONFIG"
	run_lifecycle postupgrade
	assert_success
	assert_file_line "$CONFIG" '    path: "/volume2/video"'
	assert_file_line "$SYNOPKG_PKGVAR/$BACKUP_FILE_NAME" "    path: \"/var/packages/$FIXTURE_ID/shares/video\""
}

case_postupgrade_leaves_an_ambiguous_or_missing_share_alone() {
	export SYNOPKG_PKG_STATUS=UPGRADE
	create_share video 1
	create_share video 2
	alpha2_config video >"$CONFIG"
	alpha2_config video >"$BATS_TEST_TMPDIR/expected"
	run_lifecycle postupgrade
	assert_success
	assert_stderr_contains "no folder known for the share link video"
	assert_equal "$(cat "$CONFIG")" "$(cat "$BATS_TEST_TMPDIR/expected")"
	[[ ! -e $SYNOPKG_PKGVAR/$BACKUP_FILE_NAME ]] || fail "a backup was made without a change"

	alpha2_config gone >"$CONFIG"
	run_lifecycle postupgrade
	assert_success
	assert_equal "$(cat "$CONFIG")" "$(alpha2_config gone)"
}

case_postupgrade_rewrites_only_lines_in_alpha2_form() {
	export SYNOPKG_PKG_STATUS=UPGRADE
	create_share video
	local link=/var/packages/$FIXTURE_ID/shares
	cat >"$CONFIG" <<EOF
libraries:
  - name: "hand edited"
    path: $link/video
  - name: "indented"
    path: "$link/video"
  - name: "quote"
    path: '$link/video'
  - name: "other package"
    path: "/var/packages/other/shares/video"
  - name: "nested"
    path: "$link/video/sub"
  - name: "alpha2"
    path: "$link/video" # note
  - name: "exact"
    path: "$link/video"
    kind: "movies"
  - name: "elsewhere"
    path: "/volume1/video"
EOF
	# The entry "indented" gets two more spaces than alpha.2 wrote.
	sed -i '5s/^    path/      path/' "$CONFIG"
	local before old_text new_text
	before=$(cat "$CONFIG")
	old_text=$'\n'"    path: \"$link/video\""$'\n'"    kind"
	new_text=$'\n'"    path: \"/volume1/video\""$'\n'"    kind"
	run_lifecycle postupgrade
	assert_success
	assert_stderr_contains "unusual line left as it is"
	assert_equal "$(cat "$CONFIG")" "${before//"$old_text"/"$new_text"}"
	assert_equal "$(cat "$SYNOPKG_PKGVAR/$BACKUP_FILE_NAME")" "$before"
}

case_postupgrade_without_config_does_nothing() {
	export SYNOPKG_PKG_STATUS=UPGRADE
	create_share video
	run_lifecycle postupgrade
	assert_success
	assert_no_output
	[[ ! -e $CONFIG && ! -e $SYNOPKG_PKGVAR/$BACKUP_FILE_NAME ]] || fail "postupgrade created a file"
}

case_postupgrade_skips_install_and_uninstall() {
	local status
	create_share video
	for status in INSTALL UNINSTALL; do
		export SYNOPKG_PKG_STATUS=$status
		alpha2_config video >"$CONFIG"
		run_lifecycle postupgrade
		assert_success
		assert_equal "$(cat "$CONFIG")" "$(alpha2_config video)"
	done
}

# Whole upgrade from alpha.2 in DSM's order: the new preupgrade runs while the old links exist,
# the old package is removed (links gone), the new one is installed.
case_upgrade_from_alpha2_keeps_the_library() {
	link_share video
	fill_user_data
	alpha2_config video >"$CONFIG"
	export SYNOPKG_PKG_STATUS=UPGRADE wizard_delete_data=true
	local script
	for script in preupgrade preuninst postuninst; do
		run_lifecycle "$script"
		assert_success
	done
	rm -rf "$PKG_ROOT/shares" "${SYNOPKG_PKGTMP:?}"/*
	unset wizard_delete_data
	for script in preinst postinst postupgrade; do
		run_lifecycle "$script"
		assert_success
	done
	assert_file_line "$CONFIG" '    path: "/volume1/video"'
	assert_equal "$(cat "$SYNOPKG_PKGVAR/data/library.db")" database
	run_sss start
	assert_success
	assert_file_line "$FAKE_STATE_DIR/server.env" "DEMO_BROWSE_ROOTS=/volume1"
	run_sss stop
	assert_success
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
	local before directory home=$SYNOPKG_PKGHOME volume=$BATS_TEST_TMPDIR/volume1
	mkdir -p "$BATS_TEST_TMPDIR/elsewhere" "$volume/@apphome/other" "$volume/@appdata/other" \
		"$volume/nested/@apphome/$FIXTURE_ID"
	echo precious >"$BATS_TEST_TMPDIR/elsewhere/file"
	ln -s "$volume/@apphome/other" "$volume/@apphome/other-link"
	before=$(user_data_snapshot)
	# Not the package's home: another place, ".." and "." elements, a path outside DSM's
	# /var/packages, the var directory of the package, another package, a share.
	for directory in "$BATS_TEST_TMPDIR/elsewhere" / "$PKG_ROOT/var/../../.." "$PKG_ROOT/missing" \
		"$DSM_PKG_ROOT/home/." "$DSM_PKG_ROOT/home/../home" "$PKG_ROOT/home" "$DSM_PKG_ROOT/var" \
		"$volume/@appdata/$FIXTURE_ID" "$volume/@apphome/other" "$DSM_PACKAGES_DIR/other/home" \
		"$DSM_PACKAGES_DIR//$FIXTURE_ID/home" "$volume/video" "$DSM_PKG_ROOT/home/data" \
		"$volume/nested/@apphome/$FIXTURE_ID"; do
		export SYNOPKG_PKGHOME=$directory
		: >"$TEMP_LOGFILE"
		run_lifecycle postuninst
		assert_failure 1
		assert_stderr_contains "not deleting $directory"
		assert_file_line "$TEMP_LOGFILE" "$stderr"
	done
	# The right path, but its link leads to a share, another package or another directory.
	export SYNOPKG_PKGHOME=$home
	for directory in "$volume/video" "$volume/@apphome/other" "$volume/@apphome/other-link" \
		"$volume/@appdata/$FIXTURE_ID" "$BATS_TEST_TMPDIR/elsewhere"; do
		ln -sfn "$directory" "$PKG_ROOT/home"
		run_lifecycle postuninst
		assert_failure 1
		assert_stderr_contains "not deleting $home"
	done
	ln -sfn "$volume/@apphome/$FIXTURE_ID" "$PKG_ROOT/home"
	assert_equal "$(user_data_snapshot)" "$before"
	assert_equal "$(cat "$BATS_TEST_TMPDIR/elsewhere/file")" precious
}

# DSM's shapes as they may arrive: the link below /var/packages (with a trailing slash) or the
# real directory on the volume.
case_postuninst_accepts_every_dsm_shape() {
	local volume=$BATS_TEST_TMPDIR/volume1 shape
	export SYNOPKG_PKG_STATUS=UNINSTALL wizard_delete_data=true
	for shape in links slashes volume; do
		fill_user_data
		case $shape in
		slashes) export SYNOPKG_PKGVAR=$DSM_PKG_ROOT/var/ SYNOPKG_PKGHOME=$DSM_PKG_ROOT/home/ ;;
		volume) export SYNOPKG_PKGVAR=$volume/@appdata/$FIXTURE_ID SYNOPKG_PKGHOME=$volume/@apphome/$FIXTURE_ID ;;
		esac
		run_lifecycle postuninst
		assert_success
		assert_equal "$(ls -A "$volume/@appdata/$FIXTURE_ID")" ""
		assert_equal "$(ls -A "$volume/@apphome/$FIXTURE_ID")" ""
		assert_equal "$(cat "$volume/video/movie.mkv")" movie
	done
}

case_postuninst_speaks_german() {
	fill_user_data
	export SYNOPKG_PKG_STATUS=UNINSTALL wizard_delete_data=true SYNOPKG_DSM_LANGUAGE=ger
	export SYNOPKG_PKGHOME=/
	run_lifecycle postuninst
	assert_failure 1
	assert_file_line "$TEMP_LOGFILE" \
		"/ wird nicht gelöscht: es ist nicht das home-Verzeichnis des Pakets $FIXTURE_ID; die Daten von $FIXTURE_DISPLAY_NAME bleiben erhalten"
	export SYNOPKG_PKGHOME=$DSM_PKG_ROOT/home
	: >"$TEMP_LOGFILE"
	fake_failing_command rm '*/data'
	run_lifecycle postuninst
	assert_failure 1
	assert_file_line "$TEMP_LOGFILE" \
		"Die Daten von $FIXTURE_DISPLAY_NAME in $SYNOPKG_PKGVAR und $SYNOPKG_PKGHOME konnten nicht vollständig gelöscht werden"
}

case_postuninst_reports_data_it_cannot_delete() {
	fill_user_data
	export SYNOPKG_PKG_STATUS=UNINSTALL wizard_delete_data=true
	fake_failing_command rm '*/data'
	run_lifecycle postuninst
	assert_failure 1
	# Named by the resolved directory, which is where the deletion happens.
	assert_stderr_contains "cannot remove $(cd -P "$SYNOPKG_PKGVAR" && pwd -P)/data"
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
	export SYNOPKG_PKG_STATUS=INSTALL
	run_lifecycle preinst
	assert_success
	run_lifecycle postinst
	assert_success
	run_sss start
	assert_success
	assert_file_line "$FAKE_STATE_DIR/server.env" "DEMO_CONFIG_DIR=$SYNOPKG_PKGVAR"
	assert_file_line "$FAKE_STATE_DIR/server.env" "DEMO_INSTALL_KIND=synology"
	# A fresh installation has no config.yaml: the server starts with zero libraries.
	[[ ! -e $CONFIG ]] || fail "installation left a config.yaml"
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

# Uninstall with the box unchecked, then a new installation: the old settings and database are
# used again.
case_uninstall_keeping_data_then_reinstall() {
	fill_user_data
	local before script
	before=$(user_data_snapshot)
	export SYNOPKG_PKG_STATUS=UNINSTALL wizard_delete_data=false
	for script in preuninst postuninst; do
		run_lifecycle "$script"
		assert_success
	done
	assert_equal "$(user_data_snapshot)" "$before"

	unset wizard_delete_data
	export SYNOPKG_PKG_STATUS=INSTALL
	for script in preinst postinst; do
		run_lifecycle "$script"
		assert_success
	done
	assert_equal "$(user_data_snapshot)" "$before"
}

# --- common.sh helpers ---------------------------------------------------------------------

case_browse_roots_lists_existing_volume_directories_sorted() {
	# The package tree of setup holds volume1 only.
	run_common pkg_browse_roots
	assert_success
	assert_output '/volume1'
	mkdir "$BATS_TEST_TMPDIR/volume2" "$BATS_TEST_TMPDIR/volumeUSB1" "$BATS_TEST_TMPDIR/volumeUSB2"
	run_common pkg_browse_roots
	assert_output '/volume1:/volume2:/volumeUSB1:/volumeUSB2'
	# Neither a name with more than digits, a file, nor a directory deeper down is a volume.
	mkdir "$BATS_TEST_TMPDIR/volume1x" "$BATS_TEST_TMPDIR/volumeUSBx" "$BATS_TEST_TMPDIR/volumeUSB" \
		"$BATS_TEST_TMPDIR/volume1/volume3"
	touch "$BATS_TEST_TMPDIR/volume3"
	run_common pkg_browse_roots
	assert_output '/volume1:/volume2:/volumeUSB1:/volumeUSB2'
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
	local volume=$BATS_TEST_TMPDIR/volume1 directory real_var real_home
	real_var=$(cd -P "$volume/@appdata/$FIXTURE_ID" && pwd -P)
	real_home=$(cd -P "$volume/@apphome/$FIXTURE_ID" && pwd -P)
	mkdir -p "$BATS_TEST_TMPDIR/demonstration" "$volume/@appdata/${FIXTURE_ID}x" "$volume/@appdata/sub/$FIXTURE_ID/x" \
		"$volume/nested/@appdata/$FIXTURE_ID" "$BATS_TEST_TMPDIR/deeper/volume1/@appdata/$FIXTURE_ID" \
		"$BATS_TEST_TMPDIR/share/@appdata/$FIXTURE_ID" "$BATS_TEST_TMPDIR/volume/@appdata/$FIXTURE_ID" \
		"$BATS_TEST_TMPDIR/volumeX/@appdata/$FIXTURE_ID" "$BATS_TEST_TMPDIR/volume1x/@appdata/$FIXTURE_ID"
	touch "$PKG_ROOT/keep"
	for directory in "$SYNOPKG_PKGVAR" "$SYNOPKG_PKGVAR/" "$volume/@appdata/$FIXTURE_ID"; do
		run_common pkg_resolve_package_directory var "$directory"
		assert_success
		assert_output "$real_var"
	done
	for directory in "$SYNOPKG_PKGHOME" "$volume/@apphome/$FIXTURE_ID/"; do
		run_common pkg_resolve_package_directory home "$directory"
		assert_success
		assert_output "$real_home"
	done
	for directory in / '' relative/demo/var "$BATS_TEST_TMPDIR" "$BATS_TEST_TMPDIR/demonstration" \
		"$SYNOPKG_PKGVAR/../../.." "$SYNOPKG_PKGVAR/./missing" "$PKG_ROOT/missing" "$PKG_ROOT/keep" \
		"$PKG_ROOT/var" "$SYNOPKG_PKGHOME" "$SYNOPKG_PKGVAR//" "$SYNOPKG_PKGVAR/data" "$SYNOPKG_PKGTMP" \
		"$SYNOPKG_PKGDEST" "$volume/@appdata/${FIXTURE_ID}x" "$volume/@appdata/sub/$FIXTURE_ID/x" \
		"$volume/@apptemp/$FIXTURE_ID" "$DSM_PACKAGES_DIR/$FIXTURE_ID" "/var/packages/$FIXTURE_ID/var/." \
		"$volume/@appdata/$FIXTURE_ID/../$FIXTURE_ID" "$volume/nested/@appdata/$FIXTURE_ID" \
		"$BATS_TEST_TMPDIR/deeper/volume1/@appdata/$FIXTURE_ID" "$BATS_TEST_TMPDIR/share/@appdata/$FIXTURE_ID" \
		"$BATS_TEST_TMPDIR/volume/@appdata/$FIXTURE_ID" "$BATS_TEST_TMPDIR/volumeX/@appdata/$FIXTURE_ID" \
		"$BATS_TEST_TMPDIR/volume1x/@appdata/$FIXTURE_ID"; do
		run_common pkg_resolve_package_directory var "$directory"
		[[ $status -eq 1 ]] || fail "'$directory' passed as the package's var directory"
		assert_output ''
	done
	# The volume is one path component right below / of the NAS (PKG_TEST_VOLUMES_ROOT here):
	# without the test root, the volumes of the test tree lie too deep, also behind the link.
	for directory in "$volume/@appdata/$FIXTURE_ID" "$SYNOPKG_PKGVAR"; do
		PKG_TEST_VOLUMES_ROOT='' run_common pkg_resolve_package_directory var "$directory"
		[[ $status -eq 1 ]] || fail "'$directory' passed with a volume below $BATS_TEST_TMPDIR and no test root"
		assert_output ''
	done
	# A var link that leads to the home directory resolves to the wrong kind.
	ln -sfn "$volume/@apphome/$FIXTURE_ID" "$PKG_ROOT/var"
	run_common pkg_resolve_package_directory var "$SYNOPKG_PKGVAR"
	assert_failure 1
}

register_cases \
	preinst_asks_nothing \
	postinst_creates_the_directories_and_writes_no_config \
	postinst_keeps_existing_config \
	postinst_keeps_existing_directories \
	postinst_reports_directories_it_cannot_create \
	postinst_fails_without_dsm_variables \
	preupgrade_records_the_targets_of_the_share_links \
	preupgrade_records_nothing_without_links \
	postupgrade_rewrites_the_alpha2_path_and_keeps_a_backup \
	postupgrade_handles_names_with_spaces_and_non_ascii_letters \
	postupgrade_without_a_record_uses_the_one_directory_of_that_name \
	postupgrade_leaves_an_ambiguous_or_missing_share_alone \
	postupgrade_rewrites_only_lines_in_alpha2_form \
	postupgrade_without_config_does_nothing \
	postupgrade_skips_install_and_uninstall \
	preupgrade_survives_a_record_it_cannot_write \
	preupgrade_ignores_targets_outside_the_volumes \
	postupgrade_keeps_config_when_the_new_file_cannot_be_written \
	postupgrade_ignores_dot_names_and_foreign_targets \
	upgrade_from_alpha2_keeps_the_library \
	postuninst_deletes_data_only_when_asked_on_uninstall \
	postuninst_keeps_data_for_every_other_combination \
	postuninst_never_deletes_outside_the_package_directories \
	postuninst_accepts_every_dsm_shape \
	postuninst_speaks_german \
	postuninst_reports_data_it_cannot_delete \
	postuninst_without_dsm_variables_deletes_nothing \
	other_scripts_keep_everything \
	install_then_start \
	upgrade_order_keeps_user_data \
	uninstall_keeping_data_then_reinstall \
	browse_roots_lists_existing_volume_directories_sorted \
	yaml_quote_escapes_backslash_and_double_quote \
	package_directory_check_accepts_only_package_paths

# --- static checks, independent of the shell -----------------------------------------------

@test "all seven package scripts exist and start with #!/bin/sh" {
	local script
	for script in preinst postinst preuninst postuninst preupgrade postupgrade start-stop-status; do
		assert_equal "$(head -n 1 "$PACKAGE_SCRIPTS/$script")" '#!/bin/sh'
	done
}
