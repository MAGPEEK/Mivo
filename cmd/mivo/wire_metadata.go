package main

import (
	"fmt"

	"github.com/magpeek/mivo/internal/adapters/metadata/nfo"
	appscan "github.com/magpeek/mivo/internal/app/scan"
)

// nfoParser adapts the NFO reader to the scan's NFOParser port. The port repeats the kinds
// as its own type because app must not import adapters; the values are the same.
type nfoParser struct{}

// Parse reads content as an NFO of the given kind.
func (nfoParser) Parse(data []byte, kind appscan.NFOKind) (appscan.NFOResult, error) {
	result, err := nfo.Parse(data, nfo.Kind(kind))
	if err != nil {
		return appscan.NFOResult{}, fmt.Errorf("parse %s nfo: %w", kind, err)
	}

	return appscan.NFOResult{Records: result.Records, Warnings: result.Warnings}, nil
}
