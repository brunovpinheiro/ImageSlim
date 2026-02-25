// Package gm wraps the GraphicsMagick CLI (the "gm" binary) to perform
// batch image resize and compression on a directory tree.
//
// All commands are executed via "bash -lc" so that:
//   - Shell constructs like pipes and while-read loops work correctly.
//   - A login shell PATH is used, which typically includes /usr/local/bin or
//     /opt/homebrew/bin where gm lives on macOS.
package gm

import (
	"bytes"
	"fmt"
	"os/exec"
)

// Options holds all configuration needed for a GraphicsMagick batch run.
type Options struct {
	// Dir is the base directory that contains the images.
	// The gm command is run with this as its working directory.
	Dir string

	// Pattern is the shell glob used to match image files, e.g. "*.jpg".
	// It is passed to find's -iname flag (case-insensitive).
	Pattern string

	// Resize is the geometry string passed to gm -resize, e.g. "1200x1200".
	// GraphicsMagick preserves aspect ratio by default when only one
	// dimension would be exceeded.
	Resize string

	// Quality is the JPEG quality value (1–100) passed to gm -quality.
	Quality int

	// Overwrite controls which gm subcommand is used:
	//   true  → gm mogrify (modifies files in-place)
	//   false → gm convert (writes to an "output/" mirror directory)
	Overwrite bool
}

// Result holds the outcome of a GraphicsMagick run.
type Result struct {
	// Command is a human-readable description of what was executed.
	Command string

	// Output is the combined stdout + stderr captured from the shell command.
	Output string

	// Err is non-nil when the command exited with a non-zero status or
	// could not be started at all.
	Err error
}

// Run executes the appropriate GraphicsMagick command for the given Options
// and returns a Result with the command details and any output or error.
//
// Overwrite mode (opts.Overwrite == true):
//
//	Uses "find … -exec gm mogrify …" to resize and recompress every matching
//	file in-place, recursively under opts.Dir.
//
// Preserve mode (opts.Overwrite == false):
//
//	Creates an "output/" subdirectory inside opts.Dir and mirrors the full
//	folder structure there, writing each converted file with "gm convert".
//	Original files are never modified.
func Run(opts Options) Result {
	var shellCmd string

	if opts.Overwrite {
		// gm mogrify modifies every matching file in-place.
		// find + -exec avoids shell glob-expansion limits and handles
		// arbitrarily deep directory trees.
		shellCmd = fmt.Sprintf(
			`find . -type f -iname %q -exec gm mogrify -resize %s -quality %d {} \;`,
			opts.Pattern,
			opts.Resize,
			opts.Quality,
		)
	} else {
		// gm convert writes output to a mirrored "output/" directory.
		//
		// Breakdown of the shell pipeline:
		//   mkdir -p output          – ensure the output root exists
		//   find … | while IFS= read -r f
		//                            – iterate over matching files safely
		//                              (IFS= and -r preserve whitespace/backslashes)
		//   out="output/${f#./}"     – strip leading "./" to build the mirrored path
		//   mkdir -p "$(dirname …)"  – create any missing subdirectories
		//   gm convert "$f" … "$out" – resize + compress into the output tree
		shellCmd = fmt.Sprintf(
			`mkdir -p output && find . -type f -iname %q | while IFS= read -r f; do `+
				`out="output/${f#./}"; `+
				`mkdir -p "$(dirname "$out")"; `+
				`gm convert "$f" -resize %s -quality %d "$out"; `+
				`done`,
			opts.Pattern,
			opts.Resize,
			opts.Quality,
		)
	}

	// Execute under bash -lc so pipelines and loops work, and so the login
	// shell PATH (which typically includes the gm binary location) is active.
	cmd := exec.Command("bash", "-lc", shellCmd)
	cmd.Dir = opts.Dir

	// Capture both stdout and stderr into a single buffer so that all
	// diagnostic messages from gm are available in Result.Output.
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf

	err := cmd.Run()

	return Result{
		Command: fmt.Sprintf("(in %s)\n%s", opts.Dir, shellCmd),
		Output:  buf.String(),
		Err:     err,
	}
}
