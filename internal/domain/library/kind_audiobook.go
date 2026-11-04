package library

// KindAudiobooks is the library kind for audiobooks (ADR 0027 second wave). A book is one
// folder of audio files or one single-file M4B/M4A/MP3.
const KindAudiobooks Kind = "audiobooks"

// ItemKindAudiobook is the top-level item kind of a book. It owns its part files; chapters
// are values (media.Chapter), not items.
const ItemKindAudiobook ItemKind = "audiobook"
