package server

// Config is passed to NewServer to control runtime behaviour.
type Config struct {
	Port string // Port is the port the server listens on. Default "8989".
	// LogFile is the path to the file where all chat messages are persisted.
	// When empty, logging is discarded (no file I/O).
	LogFile   string
	PrintHelp bool // PrintHelp tells the sever to print a help message and exit.

	// BONUS: Handle varying max connections.

	// MaxConnections controls how many simultaneous clients the server accepts.
	//   0 - use default (MaxClientsDefault = 10)
	// > 0 - enforce that exact limit
	// < 0 - unlimited ; for load-testing.
	MaxConnections int
}
