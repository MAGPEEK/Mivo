package credentials

import (
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"

	"golang.org/x/crypto/argon2"

	"github.com/magpeek/mivo/internal/domain"
)

const (
	algorithmName = "argon2id"
	// phcFieldCount is the number of "$"-separated fields including the empty one before the
	// leading "$": "", algorithm, version, parameters, salt, key.
	phcFieldCount = 6
	// minimumSaltLength and the key bounds reject truncated or absurd stored encodings.
	minimumSaltLength = 8
	minimumKeyLength  = 16
	maximumKeyLength  = 64
	// The upper parameter bounds stop a corrupt stored hash from exhausting a small NAS.
	maximumMemoryKiB = 256 * 1024
	maximumPasses    = 16
	maximumLanes     = 255
	// memoryKiBPerLane is the argon2 minimum: each lane needs at least 8 KiB blocks.
	memoryKiBPerLane = 8
	parameterBits    = 32
)

// storedHash is a decoded PHC string.
type storedHash struct {
	params parameters
	salt   []byte
	key    []byte
}

// encodePHC renders the PHC string "$argon2id$v=19$m=…,t=…,p=…$<salt>$<key>".
func encodePHC(params parameters, salt, key []byte) string {
	return fmt.Sprintf("$%s$v=%d$m=%d,t=%d,p=%d$%s$%s", algorithmName, argon2.Version,
		params.memoryKiB, params.passes, params.lanes,
		base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(key))
}

// parsePHC decodes a PHC string; every malformed or out-of-bounds input wraps domain.ErrInvalid.
func parsePHC(encoded string) (storedHash, error) {
	fields := strings.Split(encoded, "$")
	if len(fields) != phcFieldCount || fields[0] != "" || fields[1] != algorithmName {
		return storedHash{}, fmt.Errorf("password hash: not an argon2id PHC string: %w", domain.ErrInvalid)
	}
	if fields[2] != "v="+strconv.Itoa(argon2.Version) {
		return storedHash{}, fmt.Errorf("password hash: unsupported version %q: %w", fields[2], domain.ErrInvalid)
	}
	params, err := parseParameters(fields[3])
	if err != nil {
		return storedHash{}, err
	}
	salt, err := decodeField("salt", fields[4], minimumSaltLength, maximumKeyLength)
	if err != nil {
		return storedHash{}, err
	}
	key, err := decodeField("key", fields[5], minimumKeyLength, maximumKeyLength)
	if err != nil {
		return storedHash{}, err
	}
	return storedHash{params: params, salt: salt, key: key}, nil
}

// parseParameters reads "m=…,t=…,p=…" in exactly that order.
func parseParameters(field string) (parameters, error) {
	var values [3]uint64
	names := [3]string{"m", "t", "p"}
	parts := strings.Split(field, ",")
	if len(parts) != len(names) {
		return parameters{}, fmt.Errorf("password hash: parameters %q: %w", field, domain.ErrInvalid)
	}
	for index, part := range parts {
		value, found := strings.CutPrefix(part, names[index]+"=")
		number, err := strconv.ParseUint(value, 10, parameterBits)
		if !found || err != nil {
			return parameters{}, fmt.Errorf("password hash: parameter %q: %w", part, domain.ErrInvalid)
		}
		values[index] = number
	}
	if !withinBounds(values[0], values[1], values[2]) {
		return parameters{}, fmt.Errorf("password hash: parameters %q out of range: %w", field, domain.ErrInvalid)
	}
	return parameters{
		memoryKiB: uint32(values[0]), //nolint:gosec // bounded by maximumMemoryKiB above
		passes:    uint32(values[1]), //nolint:gosec // bounded by maximumPasses above
		lanes:     uint8(values[2]),  //nolint:gosec // bounded by maximumLanes above
	}, nil
}

func withinBounds(memoryKiB, passes, lanes uint64) bool {
	return lanes >= 1 && lanes <= maximumLanes &&
		passes >= 1 && passes <= maximumPasses &&
		memoryKiB >= memoryKiBPerLane*lanes && memoryKiB <= maximumMemoryKiB
}

func decodeField(name, value string, minimumLength, maximumLength int) ([]byte, error) {
	decoded, err := base64.RawStdEncoding.DecodeString(value)
	if err != nil {
		return nil, fmt.Errorf("password hash: %s encoding: %w", name, domain.ErrInvalid)
	}
	if len(decoded) < minimumLength || len(decoded) > maximumLength {
		return nil, fmt.Errorf("password hash: %s length %d: %w", name, len(decoded), domain.ErrInvalid)
	}
	return decoded, nil
}
