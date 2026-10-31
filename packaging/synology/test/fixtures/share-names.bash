# shellcheck shell=bash
# shellcheck disable=SC2034,SC2016,SC1003 # used by the .bats files; "$" and "\" are literal on purpose
# Shared-folder names for the wizard's validator regex and common.sh's pkg_check_share_name
# (task SPK-06), following DSM's naming rules (DSM-NOTES section 9): 1 to 32 characters; none of
# the characters in FORBIDDEN_SHARE_CHARACTERS; no leading "-" or space; no trailing space; "$"
# only at the end. Both implementations must give the same answer for every name.

# Characters DSM forbids in a shared-folder name.
FORBIDDEN_SHARE_CHARACTERS=('!' '"' '#' '%' '&' "'" '(' ')' '*' '+' ',' '/' ':' ';' '<' '=' '>'
	'?' '@' '[' ']' '\' '^' '`' '{' '}' '|' '~')

# repeat_text prints $1 repeated $2 times.
repeat_text() {
	local result='' index
	for ((index = 0; index < $2; index++)); do
		result+=$1
	done
	printf '%s' "$result"
}

VALID_SHARE_NAMES=(
	video
	a
	'Filme und Serien'
	'Videos äöü ß'
	'Médiathèque'
	'日本語'
	'name$'
	'a$'
	'a-'
	'a-b_c.d'
	'.hidden'
	"$(repeat_text x 32)"
	"$(repeat_text x 31)\$"
	"$(repeat_text ä 32)"
)

INVALID_SHARE_NAMES=(
	''
	'-video'
	' video'
	'video '
	"$(repeat_text x 33)"
	"$(repeat_text x 32)\$"
	"$(repeat_text ä 33)"
	'$'
	'$video'
	'a$b'
	'.'
	'..'
	'video/movies'
	$'a\nb'
	$'a\tb'
	$'a\001b'
	$'a\177b'
)
for forbidden_character in "${FORBIDDEN_SHARE_CHARACTERS[@]}"; do
	INVALID_SHARE_NAMES+=("a${forbidden_character}b")
done
unset forbidden_character
