// Package account holds the pure rules for accounts and profiles (ADR 0032 §1): account and
// profile names, password and PIN shape, roles, avatar colour tokens, the last-admin
// invariant, the managed-profile limit and who may act as admin or manage a profile.
//
// Hashing (argon2id) and storage live in adapters; this package never sees a hash. Like the
// rest of the domain it imports only the standard library, so Unicode NFC composition is
// supplied by the caller as a Composer (golang.org/x/text at the adapter boundary, ADR 0027).
package account
