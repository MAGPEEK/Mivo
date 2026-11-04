package library

// nulCharacter ends a string in the C APIs of every operating system, so a path containing
// it would name a different file than the one validated. Library roots (definition.go) and
// relative paths (relative_path.go) both reject it, which is why it lives in its own file.
const nulCharacter = '\x00'
