// Package adapters groups the implementations of the ports declared in app
// (http, sqlite, ffmpeg, fs, metadata, dlna, netaccess).
//
// Import rule: adapters may import app, domain and platform. They never import cmd, and
// app and domain never import adapters. See ADR 0003.
package adapters
