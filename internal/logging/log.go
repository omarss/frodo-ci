// Package logging configures the structured logger used across frodo-ci.
//
// Local CLI use gets human-friendly console output; CI use (or --json) gets
// line-delimited JSON suitable for log ingestion.
package logging

import (
	"io"
	"os"
	"strings"
	"time"

	"github.com/rs/zerolog"
)

// New builds a logger at the given level. When pretty is true, output is
// console-formatted; otherwise it is line-delimited JSON. A nil writer
// defaults to stderr so logs never pollute machine-readable stdout.
func New(level string, pretty bool, w io.Writer) zerolog.Logger {
	if w == nil {
		w = os.Stderr
	}
	var out io.Writer = w
	if pretty {
		out = zerolog.ConsoleWriter{Out: w, TimeFormat: time.RFC3339}
	}
	return zerolog.New(out).Level(parseLevel(level)).With().Timestamp().Logger()
}

// parseLevel maps a human level string to a zerolog level, defaulting to info.
func parseLevel(level string) zerolog.Level {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "trace":
		return zerolog.TraceLevel
	case "debug":
		return zerolog.DebugLevel
	case "warn", "warning":
		return zerolog.WarnLevel
	case "error":
		return zerolog.ErrorLevel
	default:
		return zerolog.InfoLevel
	}
}
