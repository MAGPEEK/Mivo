// Package app contains the use cases and declares the small ports (1-3 methods) they need.
//
// Import rule: app imports only domain and the standard library. It never imports
// adapters, platform or cmd; configuration arrives through constructor options.
// See ADR 0003.
package app
