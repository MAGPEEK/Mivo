// Package recipecheck holds static gates that tie the own FFmpeg build recipe of the Synology
// package (packaging/synology/ffmpeg/build, task SPK-15) to the server's source: the recipe
// compiles exactly the demuxers of the server's -format_whitelist, and the test here fails
// without a build as soon as one of the two lists changes alone (BACKLOG "spk/ffmpeg build",
// task SPK-16); it also builds the concat demuxer and protocol that folder discs need
// (disc_input.go, task M3-16j), the subfile protocol that reads titles inside ISO images (M3-16g,
// task M3-16j-b) and the muxer and encoders of the image renderer. The package
// has no production code; the gates are its tests.
package recipecheck
