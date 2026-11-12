//go:build !e2e

package main

// defaultRunHooks is the production hook set: none. The E2E suite builds with the e2e tag
// instead (hooks_e2e.go), so a release binary has no way to redirect a provider.
func defaultRunHooks(_ func(key string) (string, bool)) runHooks {
	return runHooks{}
}
