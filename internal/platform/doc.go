// Package platform groups cross-cutting infrastructure: configuration, logging, OS paths,
// brand, build info and the Windows service integration.
//
// Import rule: platform imports nothing from domain, app or adapters. Only adapters and
// cmd may import platform. See ADR 0003.
package platform
