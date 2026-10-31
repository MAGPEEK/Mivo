package libraryfs

import "context"

// RootAvailable reports whether the absolute library root can be opened through os.Root and
// is a directory now. It implements the RootChecker port of app/library, which bounds every
// call with its own timeout, because a stat on a hung network mount may ignore ctx; ctx is
// only checked before the file system is touched. A cancelled ctx, a relative root and any
// error count as unavailable.
func (*FileSystem) RootAvailable(ctx context.Context, root string) bool {
	if ctx.Err() != nil {
		return false
	}

	libraryRoot, err := openLibraryRoot(root)
	if err != nil {
		return false
	}

	defer closeRoot(libraryRoot)

	info, err := libraryRoot.Stat(".")

	return err == nil && info.IsDir()
}
