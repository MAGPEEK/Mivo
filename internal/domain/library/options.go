package library

// Options are the per-library scan switches (LS-02). All default to on.
type Options struct {
	ScanExtras   bool
	ReadNFO      bool
	LocalArtwork bool
}

// DefaultOptions returns the options of a new library: everything enabled.
func DefaultOptions() Options {
	return Options{ScanExtras: true, ReadNFO: true, LocalArtwork: true}
}
