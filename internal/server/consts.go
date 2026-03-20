package server

const (
	// MaxClientBufferSize is the maximum buffer size allowed for a single client's
	// channel in the server.
	MaxClientBufferSize = 1 << 7
	// MaxConnectionsDefault is the maximum number of clients connected to the
	// server at a time.
	MaxConnectionsDefault = 10
	// MaxBufferedClients is the maximum number of client requests the server can
	// buffer at a time.
	MaxBufferedClients = 1 << 12

	// CommandPrefix is the special character used to start chat commands.
	CommandPrefix  = "~"
	PortDefault    = "8989"
	NamePrompt     = "[ENTER YOUR NAME]: "
	WelcomeMessage = "Welcome to TCP-Chat!\n" +
		"         _nnnn_\n" +
		"        dGGGGMMb\n" +
		"       @p~qp~~qMb\n" +
		"       M|@||@) M|\n" +
		"       @,----.JM|\n" +
		"      JS^\\__/  qKL\n" +
		"     dZP        qKRb\n" +
		"    dZP          qKKb\n" +
		"   fZP            SMMb\n" +
		"   HZM            MMMM\n" +
		"   FqM            MMMM\n" +
		" __| \".        |\\dS\"qML\n" +
		" |    `.       | `' \\Zq\n" +
		"_)      \\.___.,|     .'\n" +
		"\\____   )MMMMMP|   .'\n" +
		"     `-'       `--'"
)
