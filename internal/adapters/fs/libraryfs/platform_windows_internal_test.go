//go:build windows

package libraryfs

import (
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
