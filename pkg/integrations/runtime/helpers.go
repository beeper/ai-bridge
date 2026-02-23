package runtime

import "github.com/rs/zerolog"

// ZerologFromHost extracts a zerolog.Logger from a Host via RawLoggerAccess.
// Returns zerolog.Nop() if the host does not support raw logger access or
// the underlying logger is not a zerolog.Logger.
func ZerologFromHost(host Host) zerolog.Logger {
	if rl, ok := host.(RawLoggerAccess); ok {
		if zl, ok := rl.RawLogger().(zerolog.Logger); ok {
			return zl
		}
	}
	return zerolog.Nop()
}
