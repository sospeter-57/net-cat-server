package server

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"os"
)

// Logger serialises all disk writes through a single goroutine (Run) so that
// concurrent senders never interleave partial lines in the log file.
type Logger struct {
	file   *os.File      // Backing file handle. nil means output is discarded.
	writer *bufio.Writer // Buffered writer layered on top of file for performance.

	// Logs is the inbound channel. Callers send formatted log strings here.
	// The channel is buffered (MaxBufferedMessages) so senders rarely block.
	// The OWNER of the Logger (the Server) is responsible for closing this
	// channel. https://go.dev/tour/concurrency/4
	Logs chan string
}

// NewLogger initialises a Logger that appends to filename.
func NewLogger(filename string, maxChatters int) (*Logger, error) {
	var file *os.File
	writer := bufio.NewWriter(io.Discard)

	if len(filename) > 0 {
		var err error
		// Open filename for writing only, appending to it if it exist, creating it if it doesn't.
		file, err = os.OpenFile(filename, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0o644)
		if err != nil {
			return nil, fmt.Errorf("NewLogger() error: %v", err)
		}

		writer = bufio.NewWriter(file)
	}

	if maxChatters < 1 || maxChatters > MaxBufferedClients {
		maxChatters = MaxBufferedClients
	}

	return &Logger{
		file:   file,
		writer: writer,

		Logs: make(chan string, MaxClientBufferSize*maxChatters),
	}, nil
}

// Run is a writer goroutine for the Logger.
func (l *Logger) Run() {
	defer l.Stop()

	for logMessage := range l.Logs {
		if _, err := fmt.Fprintln(l.writer, logMessage); err != nil {
			log.Println("logger.Run() failed: ", err)
		} else {
			l.writer.Flush()
		}
	}
}

// Stop flushes any buffered data and closes the backing file.
// This method should not be called directly while Run is still alive or
// a double-close might occur.
func (l *Logger) Stop() {
	// The Logs channel should be closed by the sender (i.e, the chat room).
	// https://go.dev/tour/concurrency/4
	l.writer.Flush()
	if l.file != nil {
		l.file.Close()
	}
}
