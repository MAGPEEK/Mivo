//go:build windows

package libraryfs

import (
	"errors"
	"fmt"
	"math"
	"testing"

	"golang.org/x/sys/windows"
)

func TestIdentityFromFileIDInfo(t *testing.T) {
	t.Parallel()

	var allOnes [fileID128Bytes]byte
	for index := range allOnes {
		allOnes[index] = math.MaxUint8
	}

	unique := fileIDInfo{VolumeSerialNumber: 9, FileID: [fileID128Bytes]byte{1, 0, 0, 0, 0, 0, 0, 0, 2}}

	testCases := []struct {
		name string
		info fileIDInfo
		want directoryID
	}{
		{name: "128-bit id", info: unique, want: directoryID{device: 9, indexLow: 1, indexHigh: 2}},
		{name: "zero id is unknown", info: fileIDInfo{VolumeSerialNumber: 9}, want: directoryID{device: 9}},
		{name: "invalid id is unknown", info: fileIDInfo{VolumeSerialNumber: 9, FileID: allOnes}, want: directoryID{device: 9}},
	}

	for _, testCase := range testCases {
		got := identityFromFileIDInfo(testCase.info)
		if got != testCase.want || got.known() != testCase.want.known() {
			t.Errorf("%s: identityFromFileIDInfo = %+v, want %+v", testCase.name, got, testCase.want)
		}
	}
}

// The 64-bit file index is only a fallback for file systems without FileIdInfo; any other
// error must not silently downgrade the identity (ReFS indexes are not unique).
func TestIdentityWithFallback(t *testing.T) {
	t.Parallel()

	fileID := fileIDInfo{VolumeSerialNumber: 9, FileID: [fileID128Bytes]byte{5}}
	legacy := directoryID{device: 9, indexLow: 77}
	accessDenied := fmt.Errorf("query: %w", windows.ERROR_ACCESS_DENIED)

	testCases := []struct {
		name       string
		fileIDErr  error
		want       directoryID
		wantErr    error
		wantLegacy bool
	}{
		{name: "file id", want: directoryID{device: 9, indexLow: 5}},
		{name: "invalid parameter", fileIDErr: windows.ERROR_INVALID_PARAMETER, want: legacy, wantLegacy: true},
		{name: "not supported", fileIDErr: windows.ERROR_NOT_SUPPORTED, want: legacy, wantLegacy: true},
		{name: "invalid function", fileIDErr: windows.ERROR_INVALID_FUNCTION, want: legacy, wantLegacy: true},
		{name: "other error", fileIDErr: accessDenied, wantErr: windows.ERROR_ACCESS_DENIED},
	}

	for _, testCase := range testCases {
		legacyCalled := false

		got, err := identityWithFallback(
			func() (fileIDInfo, error) { return fileID, testCase.fileIDErr },
			func() (directoryID, error) {
				legacyCalled = true
				return legacy, nil
			},
		)

		if got != testCase.want || !errors.Is(err, testCase.wantErr) || (testCase.wantErr == nil) != (err == nil) {
			t.Errorf("%s: identityWithFallback = (%+v, %v), want (%+v, %v)", testCase.name, got, err, testCase.want, testCase.wantErr)
		}

		if legacyCalled != testCase.wantLegacy {
			t.Errorf("%s: legacy index read = %t, want %t", testCase.name, legacyCalled, testCase.wantLegacy)
		}
	}
}

func TestQueryFileIDInfo(t *testing.T) {
	t.Parallel()

	directory := openDirectory(t, t.TempDir())

	if info, err := queryFileIDInfo(windows.Handle(directory.Fd())); err != nil || !identityFromFileIDInfo(info).known() {
		t.Errorf("queryFileIDInfo = (%+v, %v), want a known identity", info, err)
	}

	if _, err := queryFileIDInfo(windows.InvalidHandle); err == nil {
		t.Error("queryFileIDInfo(invalid handle) error = nil, want an error")
	}
}

func TestLegacyIdentity(t *testing.T) {
	t.Parallel()

	directory := openDirectory(t, t.TempDir())

	identity, err := legacyIdentity(windows.Handle(directory.Fd()))
	if err != nil || !identity.known() || identity.indexHigh != 0 {
		t.Errorf("legacyIdentity = (%+v, %v), want a known 64-bit identity", identity, err)
	}

	if _, err := legacyIdentity(windows.InvalidHandle); err == nil {
		t.Error("legacyIdentity(invalid handle) error = nil, want an error")
	}

	if got := identityFromIndex(1, math.MaxUint32, math.MaxUint32); got.known() {
		t.Errorf("identityFromIndex(invalid) = %+v, want unknown", got)
	}
}
