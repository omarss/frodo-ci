package cli

import (
	"fmt"
	"io"

	"github.com/rs/zerolog"
)

// App carries resolved global options and shared dependencies into every
// subcommand. It is constructed once by NewRootCommand and closed over by each
// command's RunE, which keeps wiring explicit and avoids hidden globals.
type App struct {
	RepoRoot    string // absolute path to the repository root
	Base        string // base git ref for change detection (empty = auto)
	Head        string // head git ref for change detection (empty = HEAD)
	Environment string // target environment for CD stages
	LogLevel    string // trace|debug|info|warn|error
	JSON        bool   // emit machine-readable JSON where supported

	Log zerolog.Logger
	Out io.Writer
	Err io.Writer
}

// notImplemented is a placeholder returned by commands whose behavior is
// delivered in a later build phase. It keeps the CLI surface complete and
// testable while implementation lands incrementally.
func notImplemented(feature string) error {
	return fmt.Errorf("%s is not implemented yet", feature)
}
