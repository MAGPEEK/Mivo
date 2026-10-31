#!/usr/bin/env bats
# Static checks of the package templates of task SPK-06: wizard, conf/privilege, conf/resource
# and the firewall service file. They do not depend on a shell, so they run once. How to run
# them: ../README.md.

bats_require_minimum_version 1.11.0

load helpers
load ../fixtures/share-names

PACKAGE_DIR=$REPO_ROOT/packaging/synology/package
WIZARD_DIR=$PACKAGE_DIR/WIZARD_UIFILES
# Every JSON file of the package templates; a test below proves that the list is complete.
JSON_FILES=(
	conf/privilege
	conf/resource
	WIZARD_UIFILES/install_uifile
	WIZARD_UIFILES/install_uifile_ger
	WIZARD_UIFILES/uninstall_uifile
	WIZARD_UIFILES/uninstall_uifile_ger
)

# product_id prints brand.ID from internal/platform/brand/brand.go.
product_id() {
	sed -n 's/^[[:space:]]*ID = "\(.*\)"$/\1/p' "$REPO_ROOT/internal/platform/brand/brand.go"
}

# render_template fills the build-time tokens of template $1 with ID $2, the fixture display name
# and port, as the staging step does with the brand values.
render_template() {
	sed -e "s/@ID@/$2/g" -e "s/@DISPLAY_NAME@/$FIXTURE_DISPLAY_NAME/g" -e "s/@PORT@/9999/g" "$1"
}

# share_name_verdicts prints "valid" or "invalid" for each argument, as the wizard file $1 judges
# it in JavaScript.
share_name_verdicts() {
	local uifile=$1
	shift
	printf '%s\0' "$@" | node "$TEST_DIR/share-name-check.js" "$uifile"
}

# --- JSON ----------------------------------------------------------------------------------

@test "every JSON template parses as it is in the repository" {
	local file
	for file in "${JSON_FILES[@]}"; do
		run jq empty "$PACKAGE_DIR/$file"
		assert_success
	done
}

# conf/ and WIZARD_UIFILES/ hold only JSON, apart from dynamic wizard scripts (*.sh).
@test "the list of JSON templates is complete" {
	local file listed entry
	while IFS= read -r -d '' file; do
		listed=no
		for entry in "${JSON_FILES[@]}"; do
			[[ $file == "$PACKAGE_DIR/$entry" ]] && listed=yes
		done
		[[ $listed == yes ]] || fail "$file is not in JSON_FILES"
	done < <(find "$PACKAGE_DIR/conf" "$WIZARD_DIR" -type f ! -name '*.sh' -print0)
}

@test "build tokens in JSON templates appear only inside string values" {
	local file in_file in_strings
	for file in "${JSON_FILES[@]}"; do
		in_file=$(grep -o '@[A-Z_]*@' "$PACKAGE_DIR/$file" | wc -l)
		in_strings=$(jq -r '[.. | strings] | join("\n")' "$PACKAGE_DIR/$file" | grep -o '@[A-Z_]*@' | wc -l)
		assert_equal "$in_strings" "$in_file"
	done
}

@test "rendered JSON templates still parse" {
	local file
	for file in "${JSON_FILES[@]}"; do
		render_template "$PACKAGE_DIR/$file" "$(product_id)" | jq empty || fail "rendered $file is no JSON"
	done
}

# --- conf ----------------------------------------------------------------------------------

@test "privilege runs everything as the package user and nothing as root" {
	run jq -c . "$PACKAGE_DIR/conf/privilege"
	assert_success
	assert_output '{"defaults":{"run-as":"package"}}'
}

@test "resource grants the wizard's share read-only and registers the port file" {
	run jq -c '."data-share"' "$PACKAGE_DIR/conf/resource"
	assert_output '{"shares":[{"name":"{{wizard_media_share}}","permission":{"ro":["@ID@"]}}]}'
	run jq -c '."port-config"' "$PACKAGE_DIR/conf/resource"
	assert_output '{"protocol-file":"port_conf/@ID@.sc"}'
	run jq -r 'keys | join(" ")' "$PACKAGE_DIR/conf/resource"
	assert_output "data-share port-config"
	run grep -rn -e '"rw"' -e 'root' "$PACKAGE_DIR/conf"
	assert_failure 1
}

# --- firewall service file -----------------------------------------------------------------

@test "service file has the keys DSM requires and no port forwarding" {
	local sc=$PACKAGE_DIR/port/service.sc
	assert_equal "$(head -n 1 "$sc")" "[@ID@]"
	assert_file_line "$sc" 'title="@DISPLAY_NAME@"'
	assert_file_line "$sc" 'desc="@DISPLAY_NAME@ media server"'
	assert_file_line "$sc" 'port_forward="no"'
	assert_file_line "$sc" 'dst.ports="@PORT@/tcp"'
	assert_equal "$(grep -c . "$sc")" 5
}

@test "rendered service file has a section name DSM accepts" {
	local id rendered
	for id in "$(product_id)" "$FIXTURE_ID"; do
		[[ -n $id ]] || fail "brand.ID not found in internal/platform/brand/brand.go"
		rendered=$(render_template "$PACKAGE_DIR/port/service.sc" "$id")
		# Letters, digits, "-" and "." only; no underscore (guide "Port", DSM-NOTES section 7).
		[[ $(head -n 1 <<<"$rendered") =~ ^\[[A-Za-z0-9.-]+\]$ ]] ||
			fail "section of the service file for ID '$id' is not allowed: $(head -n 1 <<<"$rendered")"
		grep -qx 'dst.ports="9999/tcp"' <<<"$rendered" || fail "port not rendered: $rendered"
	done
}

# --- wizard --------------------------------------------------------------------------------

@test "install wizards ask for the media share with default video" {
	local uifile
	for uifile in install_uifile install_uifile_ger; do
		run jq -c '[.[].items[] | select(.type == "textfield") | .subitems[]
			| {key, defaultValue, allowBlank: .validator.allowBlank}]' "$WIZARD_DIR/$uifile"
		assert_output '[{"key":"wizard_media_share","defaultValue":"video","allowBlank":false}]'
		run jq -r '.[].items[].desc' "$WIZARD_DIR/$uifile"
		assert_output --partial "@DISPLAY_NAME@"
		assert_output --partial "@PORT@"
	done
}

@test "English and German install wizards use the same validator regex" {
	assert_equal "$(jq -r '.. | .regex? | .expr? // empty' "$WIZARD_DIR/install_uifile_ger")" \
		"$(jq -r '.. | .regex? | .expr? // empty' "$WIZARD_DIR/install_uifile")"
	run jq -r '.. | .regex? | .errorText? // empty' "$WIZARD_DIR/install_uifile_ger"
	assert_output --regexp '.+'
}

@test "wizard regex accepts valid share names" {
	local uifile index
	for uifile in install_uifile install_uifile_ger; do
		run share_name_verdicts "$WIZARD_DIR/$uifile" "${VALID_SHARE_NAMES[@]}"
		assert_success
		assert_equal "${#lines[@]}" "${#VALID_SHARE_NAMES[@]}"
		for index in "${!VALID_SHARE_NAMES[@]}"; do
			[[ ${lines[index]} == valid ]] || fail "$uifile rejects '${VALID_SHARE_NAMES[index]}'"
		done
	done
}

@test "wizard regex rejects invalid share names" {
	local uifile index
	for uifile in install_uifile install_uifile_ger; do
		run share_name_verdicts "$WIZARD_DIR/$uifile" "${INVALID_SHARE_NAMES[@]}"
		assert_success
		assert_equal "${#lines[@]}" "${#INVALID_SHARE_NAMES[@]}"
		for index in "${!INVALID_SHARE_NAMES[@]}"; do
			[[ ${lines[index]} == invalid ]] || fail "$uifile accepts '${INVALID_SHARE_NAMES[index]}'"
		done
	done
}

@test "uninstall wizards offer deleting the data, unchecked by default" {
	local uifile
	for uifile in uninstall_uifile uninstall_uifile_ger; do
		run jq -c '[.[].items[] | select(.subitems) | {type, subitems: [.subitems[] | {key, defaultValue}]}]' \
			"$WIZARD_DIR/$uifile"
		assert_output '[{"type":"multiselect","subitems":[{"key":"wizard_delete_data","defaultValue":false}]}]'
	done
}

@test "wizards name the product only through the display name token" {
	run grep -rli -- "$(product_id)" "$WIZARD_DIR"
	assert_failure 1
	# No promise of access from outside the local network (server.lan_only until milestone 7).
	run grep -rliE 'internet|remote|quickconnect|anywhere' "$WIZARD_DIR"
	assert_failure 1
}

@test "package templates and test files have no CR characters" {
	run grep -rl $'\r' "$PACKAGE_DIR/conf" "$PACKAGE_DIR/port" "$WIZARD_DIR"
	assert_failure 1
}
