// Package domain holds the pure business logic of the media server.
//
// Import rule: domain is the innermost layer. It imports only the standard library and
// never I/O packages (os, os/exec, net, net/http, database/sql, ...). It never imports
// app, adapters, platform or cmd. See ADR 0003.
package domain
