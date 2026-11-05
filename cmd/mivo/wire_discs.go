package main

import (
	"github.com/magpeek/mivo/internal/adapters/disc"
	"github.com/magpeek/mivo/internal/adapters/ffmpeg"
	"github.com/magpeek/mivo/internal/adapters/fs/libraryfs"
	appplayback "github.com/magpeek/mivo/internal/app/playback"
	appscan "github.com/magpeek/mivo/internal/app/scan"
)

// scanDiscPorts lets the scan catalogue DVD and Blu-ray folders and ISO images in movies and
// shows libraries (ADR 0029): the folder is resolved inside its library root, the disc
// resolver selects the main title and refuses protected discs, and ffprobe describes it.
func scanDiscPorts(files *libraryfs.FileSystem, prober *ffmpeg.Prober) appscan.DiscPorts {
	return appscan.DiscPorts{Directories: files, Titles: disc.NewResolver(), Prober: prober}
}

// playbackDiscPorts lets sessions play the main title of a disc: the title is resolved again
// at start, and remux seeks find their keyframe in the joined title.
func playbackDiscPorts(files *libraryfs.FileSystem, prober *ffmpeg.Prober) appplayback.DiscPorts {
	return appplayback.DiscPorts{Directories: files, Titles: disc.NewResolver(), Keyframes: prober}
}
