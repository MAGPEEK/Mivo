package folderbrowse

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/magpeek/mivo/internal/domain"
)

// Check verifies that relativePath is an existing, readable directory inside the root and
// returns its cleaned absolute path (root + relativePath, links not evaluated: ADR 0022).
// Hidden and system folders (by name, on Windows also by attribute) and reserved directories
// are refused with domain.ErrInvalid, so they cannot become a library folder.
func (b *Browser) Check(ctx context.Context, rootID, relativePath string) (string, error) {
	root, err := b.rootByID(rootID)
	if err != nil {
		return "", err
	}

	native, err := nativePath(relativePath)
	if err != nil {
		return "", err
	}

	if err := b.gate(ctx, root); err != nil {
		return "", err
	}

	absolute := filepath.Join(root.path, native)

	return bounded(ctx, b.timeout, func(context.Context) (string, error) {
		if err := b.checkFolder(root, relativePath, native); err != nil {
			return "", err
		}

		return absolute, nil
	})
}

func (b *Browser) checkFolder(root rootEntry, relativePath, native string) error {
	opened, err := openRoot(root.path)
	if err != nil {
		return err
	}

	defer func() { _ = opened.Close() }()

	if hiddenSegment(opened, native) {
		return fmt.Errorf("folder %q is hidden: %w", relativePath, domain.ErrInvalid)
	}

	info, err := opened.Stat(native)
	if err != nil {
		return classify(relativePath, err)
	}

	if !info.IsDir() {
		return fmt.Errorf("folder %q is not a directory: %w", relativePath, domain.ErrInvalid)
	}

	if b.isReservedResolved(filepath.Join(root.path, native)) {
		return fmt.Errorf("folder %q is reserved: %w", relativePath, domain.ErrInvalid)
	}

	if !isReadable(opened, native) {
		return fmt.Errorf("folder %q is not readable: %w", relativePath, domain.ErrForbidden)
	}

	return nil
}
