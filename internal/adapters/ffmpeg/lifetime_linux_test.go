package ffmpeg

import (
	"os"
	"strconv"
	"strings"
)

// processGone reports whether the process has ended. An orphan whose new parent does not
// reap it (PID 1 in a container is often no init) stays a zombie, which counts as ended.
func processGone(pid int) bool {
	stat, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
	if err != nil {
		return true
	}

	// The state follows the command name, which is in parentheses and may contain spaces.
	_, fields, found := strings.Cut(string(stat), ") ")

	return found && (strings.HasPrefix(fields, "Z") || strings.HasPrefix(fields, "X"))
}
