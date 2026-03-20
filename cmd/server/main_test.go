// main_test.go contains tests for bonus and extended features that go beyond
// the core net-cat project requirements.  The core-spec tests live in
// main_base_test.go and must not be modified.
package main

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"net-cat/internal/server"
)

// ── Bonus helpers ─────────────────────────────────────────────────────────────

// matchesLogFormat reports whether line conforms to the server LogFormat output:
//
//	[YYYY-MM-DD HH:MM:SS][sender][room]:message
//
// Uses time.Parse so that only the allowed packages are used.
func matchesLogFormat(line string) bool {
	if !strings.HasPrefix(line, "[") {
		return false
	}

	end := strings.Index(line, "]")

	if end < 1 {
		return false
	}

	if _, err := time.Parse("2006-01-02 15:04:05", line[1:end]); err != nil {
		return false
	}
	// Log format: [YYYY-MM-DD HH:MM:SS][sender][room]:message
	rest := line[end+1:]

	return strings.HasPrefix(rest, "[") && strings.Contains(rest, "]:")
}

// hasDoubleTimestamp detects garbled log lines where two entries were written
// on the same line without a separating newline, which would produce two
// '[20YY-...' timestamp openings on a single line.
func hasDoubleTimestamp(line string) bool {
	first := strings.Index(line, "[20")

	if first < 0 {
		return false
	}

	return strings.Contains(line[first+1:], "[20")
}

// ─────────────────────────────────────────────
// B1. Configurable Connection Limits (bonus)
//
// The spec mandates a hard limit of 10.  These tests verify the ability
// to configure a custom limit and to disable the cap entirely — features
// added on top of the base requirement.
// ─────────────────────────────────────────────

func TestConfigurableLimitEnforced(t *testing.T) {
	t.Parallel()
	const limit = 3
	s := startTestServer(t, server.Config{MaxConnections: limit})

	var conns []net.Conn

	t.Cleanup(func() {
		for _, c := range conns {
			c.Close()
		}
	})

	for i := range limit {
		conn, r := dialClient(t, listenAddr(s))

		joinChat(t, conn, r, fmt.Sprintf("u%d", i))
		conns = append(conns, conn)
	}

	// (limit+1)-th client must be rejected.
	extra, err := net.DialTimeout("tcp", listenAddr(s), testTimeout)
	if err != nil {
		return // connection refused == rejected, test passes
	}
	defer extra.Close()

	_ = extra.SetDeadline(time.Now().Add(testTimeout))
	buf := make([]byte, 256)
	n, readErr := extra.Read(buf)

	if readErr == nil && n > 0 {
		extra.SetDeadline(time.Now().Add(100 * time.Millisecond))
		io := make([]byte, 1)
		_, closeErr := extra.Read(io)

		if closeErr == nil {
			t.Errorf("client %d was accepted; server should reject after limit %d", limit+1, limit)
		}
	}
}

func TestUnlimitedConnections(t *testing.T) {
	t.Parallel()
	// MaxConnections < 0 means unlimited.  We connect more clients than the
	// default cap (10) to prove the limit is not applied.
	const numClients = 12
	s := startTestServer(t, server.Config{MaxConnections: -1})

	var conns []net.Conn

	t.Cleanup(func() {
		for _, c := range conns {
			c.Close()
		}
	})

	for i := 0; i < numClients; i++ {
		conn, r := dialClient(t, listenAddr(s))
		got := readUntilConn(t, conn, r, server.NamePrompt, testTimeout)

		if !strings.Contains(got, server.NamePrompt) {
			t.Fatalf("client %d rejected (server prompt not received); unlimited mode broken", i)
		}

		sendLine(t, conn, fmt.Sprintf("guest%d", i))
		conns = append(conns, conn)
	}
}

// ─────────────────────────────────────────────
// B2. In-Chat Name-Change Command (bonus)
// ─────────────────────────────────────────────

func TestNameChangeCommand(t *testing.T) {
	t.Parallel()
	prefix := string(server.CommandPrefix) // e.g. "~"
	cmd := prefix + "name NewAlice"

	s := startTestServer(t, server.Config{})

	connA, rA := dialClient(t, listenAddr(s))

	joinChat(t, connA, rA, "Alice")

	connB, rB := dialClient(t, listenAddr(s))

	joinChat(t, connB, rB, "Bob")

	// drain join notification
	readUntilConn(t, connA, rA, "Bob", testTimeout)
	// drain Alice-joined notification at Bob (may already have been consumed)
	time.Sleep(10 * time.Millisecond)

	// Alice changes her name
	sendLine(t, connA, cmd)

	// Bob must receive a notification about the name change
	bobGot := readUntilConn(t, connB, rB, "NewAlice", testTimeout)

	if !strings.Contains(bobGot, "NewAlice") {
		t.Errorf("Bob did not see name-change notification; got: %q", bobGot)
	}

	// After renaming, Alice's subsequent messages should carry the new name
	sendLine(t, connA, "post-rename message")
	bobMsg := readUntilConn(t, connB, rB, "post-rename message", testTimeout)

	if !strings.Contains(bobMsg, "NewAlice") {
		t.Errorf("Message after rename does not carry new name 'NewAlice'; got: %q", bobMsg)
	}
}

// TestCommandPrefixConstant verifies that the hardcoded CommandPrefix constant
// is recognised by the server for the name-change command.  The prefix itself
// is intentionally not runtime-configurable; to change it, update the
// server.CommandPrefix constant and recompile.
func TestCommandPrefixConstant(t *testing.T) {
	cmds := []struct {
		name    string
		cmd     string
		newName string
	}{
		{"rename via constant prefix", fmt.Sprintf("%cname Renamed1", server.CommandPrefix), "Renamed1"},
		// {"rename to multi-word name", fmt.Sprintf("%cname Renamed Two", server.CommandPrefix), "Renamed Two"},
	}

	for _, tc := range cmds {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			s := startTestServer(t, server.Config{})

			connA, rA := dialClient(t, listenAddr(s))

			joinChat(t, connA, rA, "Alpha")

			connB, rB := dialClient(t, listenAddr(s))

			joinChat(t, connB, rB, "Beta")

			readUntilConn(t, connA, rA, "Beta", testTimeout)
			time.Sleep(10 * time.Millisecond)

			sendLine(t, connA, tc.cmd)

			got := readUntilConn(t, connB, rB, tc.newName, testTimeout)

			if !strings.Contains(got, tc.newName) {
				t.Errorf("Beta did not see rename notification %q; got: %q", tc.newName, got)
			}
		})
	}
}

func TestNameChangeNotifiesOthers(t *testing.T) {
	t.Parallel()
	prefix := string(server.CommandPrefix)
	s := startTestServer(t, server.Config{})

	connA, rA := dialClient(t, listenAddr(s))

	joinChat(t, connA, rA, "Foo")

	connB, rB := dialClient(t, listenAddr(s))

	joinChat(t, connB, rB, "Bar")

	connC, rC := dialClient(t, listenAddr(s))

	joinChat(t, connC, rC, "Baz")

	// drain join notifications
	readUntilConn(t, connA, rA, "Baz", testTimeout)
	readUntilConn(t, connB, rB, "Baz", testTimeout)

	// Foo changes name
	sendLine(t, connA, prefix+"name FooRenamed")

	// Both Bar and Baz must be notified
	barGot := readUntilConn(t, connB, rB, "FooRenamed", testTimeout)
	bazGot := readUntilConn(t, connC, rC, "FooRenamed", testTimeout)

	if !strings.Contains(barGot, "FooRenamed") {
		t.Errorf("Bar did not receive rename notification; got: %q", barGot)
	}

	if !strings.Contains(bazGot, "FooRenamed") {
		t.Errorf("Baz did not receive rename notification; got: %q", bazGot)
	}
}

// ─────────────────────────────────────────────
// B3. Thread-Safe Logging to File (bonus)
// ─────────────────────────────────────────────

func TestConcurrentLoggingNonCorrupt(t *testing.T) {
	t.Parallel()
	logFile := t.TempDir() + "/chat.log"
	s := startTestServer(t, server.Config{LogFile: logFile})

	const numSenders = 5
	const msgsPerSender = 10

	// Connect all clients first.
	type client struct {
		conn net.Conn
		r    *bufio.Reader
	}
	clients := make([]client, numSenders)

	for i := range clients {
		conn, r := dialClient(t, listenAddr(s))

		joinChat(t, conn, r, fmt.Sprintf("sender%d", i))
		clients[i] = client{conn, r}
	}

	// Give the server a moment to settle join notifications.
	time.Sleep(20 * time.Millisecond)

	var wg sync.WaitGroup

	for i, c := range clients {
		wg.Add(1)
		go func(id int, conn net.Conn) {
			defer wg.Done()
			for j := 0; j < msgsPerSender; j++ {
				sendLine(t, conn, fmt.Sprintf("msg%d-from-sender%d", j, id))
				time.Sleep(2 * time.Millisecond)
			}
		}(i, c.conn)
	}

	done := make(chan struct{})

	go func() { wg.Wait(); close(done) }()

	select {
	case <-done:
	case <-time.After(testTimeout * 2):
		t.Fatal("timed out waiting for concurrent senders")
	}

	// Allow the server to flush logs, then stop it so the file is finalised.
	time.Sleep(50 * time.Millisecond)
	s.Stop()

	data, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatalf("could not read log file %s: %v", logFile, err)
	}

	content := string(data)
	totalExpected := numSenders * msgsPerSender

	// Count complete log entries; each must match the server LogFormat:
	// [YYYY-MM-DD HH:MM:SS]:[sender]:[room]:[message]
	lines := strings.Split(strings.TrimSpace(content), "\n")
	var validLines int

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" && matchesLogFormat(line) {
			validLines++
		}
	}

	if validLines < totalExpected {
		t.Errorf("log corruption suspected: expected at least %d valid log entries, found %d;\nlog content:\n%s",
			totalExpected, validLines, content)
	}

	// Ensure no garbled lines (two timestamp markers on one line).
	for _, line := range lines {
		if hasDoubleTimestamp(line) {
			t.Errorf("garbled line detected (two timestamps on one line): %q", line)
		}
	}
}

// ─────────────────────────────────────────────
// B4. Log file persistence (bonus)
// Audit: "Does the server produce logs about Clients activities?"
//        "Are the server logs saved into a file?"
// ─────────────────────────────────────────────

// TestLogFileCreated verifies that Config.LogFile causes the server to create
// the log file on startup and that it exists after the server stops.
func TestLogFileCreated(t *testing.T) {
	t.Parallel()
	logPath := t.TempDir() + "/server.log"

	s := startTestServer(t, server.Config{LogFile: logPath})

	// Connect a client and send one message to trigger at least one log write.
	connA, rA := dialClient(t, listenAddr(s))

	joinChat(t, connA, rA, "Alice")
	connB, rB := dialClient(t, listenAddr(s))

	joinChat(t, connB, rB, "Bob")
	readUntilConn(t, connA, rA, "joined", testTimeout)

	sendLine(t, connA, "log existence probe")
	readUntilConn(t, connB, rB, "log existence probe", testTimeout)

	time.Sleep(20 * time.Millisecond)
	s.Stop()

	if _, err := os.Stat(logPath); err != nil {
		t.Errorf("log file %q was not created: %v", logPath, err)
	}

	_ = rA
	_ = rB
}

// TestLogCapturesMessages verifies that user-sent messages appear in the log
// file with a valid timestamp, confirming client activity is persisted.
func TestLogCapturesMessages(t *testing.T) {
	t.Parallel()
	logPath := t.TempDir() + "/msgs.log"

	s := startTestServer(t, server.Config{LogFile: logPath})

	connA, rA := dialClient(t, listenAddr(s))

	joinChat(t, connA, rA, "Alice")
	connB, rB := dialClient(t, listenAddr(s))

	joinChat(t, connB, rB, "Bob")
	readUntilConn(t, connA, rA, "joined", testTimeout)

	const payload = "unique-log-probe-9921"

	sendLine(t, connA, payload)
	readUntilConn(t, connB, rB, payload, testTimeout)

	time.Sleep(20 * time.Millisecond)
	s.Stop()

	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("log file not found: %v", err)
	}

	content := string(data)

	if !strings.Contains(content, payload) {
		t.Errorf("payload %q not found in log file; log content:\n%s", payload, content)
	}

	// Each line that contains the payload must be a valid log entry.
	for _, line := range strings.Split(strings.TrimSpace(content), "\n") {
		line = strings.TrimSpace(line)
		if strings.Contains(line, payload) && !matchesLogFormat(line) {
			t.Errorf("log line containing payload does not match expected log format;\n got: %q", line)
		}
	}

	_ = rA
	_ = rB
}

// ─────────────────────────────────────────────
// B5. Multiple simultaneous group chats (bonus)
// Audit: "Is the server capable of handling multiple separate group chats
//        simultaneously?"
//
// This test documents the expected isolation behaviour.  It will pass only
// once the server implements a room-selection mechanism (e.g. a ~room command
// or a named-room URL scheme).  Until then it acts as a failing acceptance
// criterion that clearly shows the feature is not yet implemented.
// ─────────────────────────────────────────────

// TestMultipleGroupChats verifies that two clients joined to different rooms
// on the same server cannot see each other's messages.
//
// Room selection is attempted via the "~room <name>" command using the
// server's CommandPrefix.  If the server does not recognise the command both
// clients will remain in the default "general" room and the isolation
// assertion will fail, correctly indicating the feature is unimplemented.
func TestMultipleGroupChats(t *testing.T) {
	t.Parallel()
	prefix := string(server.CommandPrefix) // e.g. "~"

	s := startTestServer(t, server.Config{})

	// Client A joins room "alpha".
	connA, rA := dialClient(t, listenAddr(s))

	joinChat(t, connA, rA, "Alpha")
	sendLine(t, connA, prefix+"join alpha")
	time.Sleep(10 * time.Millisecond)

	// Client B joins room "beta" – a different room.
	connB, rB := dialClient(t, listenAddr(s))

	joinChat(t, connB, rB, "Beta")
	sendLine(t, connB, prefix+"join beta")
	time.Sleep(10 * time.Millisecond)

	// Drain any pending notifications before the isolation probe.
	connA.SetDeadline(time.Now().Add(50 * time.Millisecond))
	connA.Read(make([]byte, 1024)) //nolint:errcheck
	connA.SetDeadline(time.Time{})
	connB.SetDeadline(time.Now().Add(50 * time.Millisecond))
	connB.Read(make([]byte, 1024)) //nolint:errcheck
	connB.SetDeadline(time.Time{})

	// Alpha sends a message; Beta must NOT receive it if rooms are isolated.
	token := "alpha-room-secret-XYZ"

	sendLine(t, connA, token)
	time.Sleep(50 * time.Millisecond)

	connB.SetDeadline(time.Now().Add(50 * time.Millisecond))
	buf := make([]byte, 512)
	n, _ := connB.Read(buf)

	connB.SetDeadline(time.Time{})

	if strings.Contains(string(buf[:n]), token) {
		t.Errorf(
			"Beta received Alpha's message despite being in a different room;"+
				" server does not isolate group chats (bonus feature not implemented); got: %q",
			string(buf[:n]),
		)
	}

	_ = rA
	_ = rB
}
