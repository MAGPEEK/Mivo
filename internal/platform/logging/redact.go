package logging

import (
	"log/slog"
	"slices"
	"strings"
)

// RedactedValue replaces the value of every sensitive attribute in log output.
const RedactedValue = "[REDACTED]"

// pluralSuffix is the one letter a whole word may carry and still match ("pins").
const pluralSuffix = 's'

// sensitiveWord is one normalised word that marks a key as credential-bearing.
type sensitiveWord struct {
	text string
	// wholeWord restricts matching to whole words of the original key, for short words
	// that occur inside many harmless ones ("pin" in spinner, pinned, mapping, shipping).
	// See normalisedKey.matchesAt for the exact boundary rule.
	wholeWord bool
}

// sensitiveWords returns the words that mark a key as credential-bearing. It is the single
// place to extend when a new kind of secret appears. Words match anywhere in the normalised
// key (see isSensitiveKey), so derived names need no entry of their own: "token" also covers
// accesstoken and tokens, "secret" covers clientsecret and awssecretaccesskey, "cookie"
// covers setcookie and cookieheader.
func sensitiveWords() []sensitiveWord {
	return []sensitiveWord{
		{text: "password"},
		{text: "passwd"},
		{text: "passphrase"},
		{text: "secret"},
		{text: "token"},
		{text: "apikey"},
		{text: "authorization"},
		{text: "cookie"},
		{text: "credential"},
		{text: "privatekey"},
		{text: "signingkey"},
		{text: "encryptionkey"},
		{text: "sessionid"},
		{text: "pin", wholeWord: true},
	}
}

// safeQualifiers returns the normalised trailing qualifiers that describe a credential
// instead of holding one ("tokenCount", "passwordHint"). A key stays visible only when
// everything after a sensitive word is exactly one of these. "id" is deliberately missing:
// identifiers such as tokenId or credentialId often are the secret or a lookup key for it.
func safeQualifiers() []string {
	return []string{"count", "hint", "type", "length", "expiry", "expiresat", "ttl"}
}

// redactSensitive is the slog.HandlerOptions.ReplaceAttr hook. slog calls it for every
// leaf attribute, including those inside groups, after LogValuer values are resolved, but
// never for a group itself. A leaf is therefore also redacted when any enclosing group
// (slog.Group, Logger.WithGroup or a LogValuer resolving to a group) has a sensitive key,
// so a group named "authorization" cannot leak its contents.
func redactSensitive(groups []string, attr slog.Attr) slog.Attr {
	if isSensitiveKey(attr.Key) || anySensitiveKey(groups) {
		return slog.String(attr.Key, RedactedValue)
	}

	return attr
}

func anySensitiveKey(keys []string) bool {
	for _, key := range keys {
		if isSensitiveKey(key) {
			return true
		}
	}

	return false
}

// isSensitiveKey reports whether key holds a secret. It fails closed: the key is sensitive
// when its normalised form contains a word from sensitiveWords anywhere, unless the text
// after every such occurrence is exactly one of safeQualifiers. Unknown qualifiers
// ("passwordHash", "cookieHeader"), plurals ("tokens") and words that merely start with a
// sensitive word ("secretary", "tokenizer") are therefore redacted; over-redacting a
// harmless value is cheap, leaking a secret is not.
func isSensitiveKey(key string) bool {
	normalised := normaliseKey(key)
	for _, word := range sensitiveWords() {
		if normalised.holdsSecret(word) {
			return true
		}
	}

	return false
}

// holdsSecret reports whether any matching occurrence of word is followed by something
// other than a safe qualifier.
func (key normalisedKey) holdsSecret(word sensitiveWord) bool {
	// offset never exceeds len(key.text): it only advances to one past a found occurrence.
	offset := 0
	for {
		index := strings.Index(key.text[offset:], word.text)
		if index < 0 {
			return false
		}

		start := offset + index
		end := start + len(word.text)
		if key.matchesAt(word, start, end) && !slices.Contains(safeQualifiers(), key.text[end:]) {
			return true
		}

		offset = start + 1
	}
}

// matchesAt reports whether the occurrence of word at text[start:end] counts. Ordinary words
// always count. A whole word must end at a word boundary, optionally after a plural "s", and
// must either start at a word boundary or end the key. The second alternative keeps run-together
// lower-case keys such as "profilepin" or "userpin" redacted, at the price of over-redacting
// rare keys such as "spin"; "spinner" and "pinned" stay visible because "pin" neither starts
// a word nor ends the key there.
func (key normalisedKey) matchesAt(word sensitiveWord, start, end int) bool {
	if !word.wholeWord {
		return true
	}

	startsWord := key.wordBoundaries[start] || end == len(key.text)
	pluralEndsWord := end < len(key.text) && key.text[end] == pluralSuffix && key.wordBoundaries[end+1]

	return startsWord && (key.wordBoundaries[end] || pluralEndsWord)
}

// normalisedKey is a log key reduced to lower-case ASCII letters and digits, together with
// the positions where the words of the original key began.
type normalisedKey struct {
	text string
	// wordBoundaries has len(text)+1 entries; entry i reports whether a word of the original
	// key starts at text[i]. The last entry, the end of the key, is always a boundary.
	wordBoundaries []bool
}

// normaliseKey removes every byte that is not an ASCII letter or digit and lower-cases the
// rest, so that "API_KEY", "x-api-key", "api key", "auth:token" and "apiKey" compare equal to
// their plain forms. Word boundaries are recorded where a removed character was, at a
// lower-case-or-digit to upper-case step ("profilePin"), at the end of an upper-case run
// ("PINCode") and between letters and digits ("pin2"). Working on bytes is safe because
// every byte of a multi-byte UTF-8 character is outside the ASCII range and is removed.
func normaliseKey(key string) normalisedKey {
	text := make([]byte, 0, len(key))
	boundaries := make([]bool, 0, len(key)+1)
	afterRemoved := true

	for index := range len(key) {
		character := key[index]
		if !isASCIIAlphanumeric(character) {
			afterRemoved = true

			continue
		}

		boundaries = append(boundaries, afterRemoved || startsWordInside(key, index))
		text = append(text, toASCIILower(character))
		afterRemoved = false
	}

	return normalisedKey{text: string(text), wordBoundaries: append(boundaries, true)}
}

// startsWordInside reports whether key[index] starts a new word although the character before
// it is a letter or digit. The caller guarantees index > 0 and that key[index-1] is kept.
func startsWordInside(key string, index int) bool {
	previous, current := key[index-1], key[index]
	if isASCIIDigit(previous) != isASCIIDigit(current) {
		return true
	}

	if !isASCIIUpper(current) {
		return false
	}

	nextIsLower := index+1 < len(key) && isASCIILower(key[index+1])

	return !isASCIIUpper(previous) || nextIsLower
}

func isASCIIAlphanumeric(character byte) bool {
	return isASCIILower(character) || isASCIIUpper(character) || isASCIIDigit(character)
}

func isASCIILower(character byte) bool { return 'a' <= character && character <= 'z' }

func isASCIIUpper(character byte) bool { return 'A' <= character && character <= 'Z' }

func isASCIIDigit(character byte) bool { return '0' <= character && character <= '9' }

func toASCIILower(character byte) byte {
	if isASCIIUpper(character) {
		return character + ('a' - 'A')
	}

	return character
}
