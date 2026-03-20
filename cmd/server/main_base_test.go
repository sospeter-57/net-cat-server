// main_base_test.go covers the original project requirements.
package main

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"net-cat/internal/server"
	"net-cat/internal/utils"
)

// TestMain silences server-side log output so it does not pollute go test output.
func TestMain(m *testing.M) {
	log.SetOutput(io.Discard)
	if f, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0); err == nil {
		os.Stderr = f
	}

	os.Exit(m.Run())
}

// ── helpers ───────────────────────────────────────────────────────────────────

const testTimeout = 500 * time.Millisecond

func listenAddr(s *server.Server) string { return s.Listener.Addr().String() }

func startTestServer(t *testing.T, cfg server.Config) *server.Server {
	t.Helper()
	cfg.Port = "0"
	s, err := server.NewServer(cfg)
	if err != nil {
		t.Fatalf("server.New: %v", err)
	}

	go s.Run()

	t.Cleanup(func() { s.Stop() })
	return s
}

func dialClient(t *testing.T, addr string) (net.Conn, *bufio.Reader) {
	t.Helper()
	conn, err := net.DialTimeout("tcp", addr, testTimeout)
	if err != nil {
		t.Fatalf("dial %s: %v", addr, err)
	}

	t.Cleanup(func() { conn.Close() })
	return conn, bufio.NewReader(conn)
}

// readUntil reads from r until substr appears or timeout elapses.
func readUntil(t *testing.T, r *bufio.Reader, substr string, timeout time.Duration) string {
	t.Helper()
	deadline := time.Now().Add(timeout)
	var buf strings.Builder

	for time.Now().Before(deadline) {
		line, err := r.ReadString('\n')

		buf.WriteString(line)
		if strings.Contains(buf.String(), substr) {
			return buf.String()
		}

		if err != nil {
			break
		}
	}

	return buf.String()
}

// readUntilConn is like readUntil but sets per-read deadlines on conn.
func readUntilConn(t *testing.T, conn net.Conn, r *bufio.Reader, substr string, timeout time.Duration) string {
	t.Helper()
	deadline := time.Now().Add(timeout)

	defer conn.SetDeadline(time.Time{})
	var buf strings.Builder

	for time.Now().Before(deadline) {
		iter := time.Now().Add(25 * time.Millisecond)

		if iter.After(deadline) {
			iter = deadline
		}

		conn.SetDeadline(iter)
		line, err := r.ReadString('\n')

		buf.WriteString(line)
		if strings.Contains(buf.String(), substr) {
			return buf.String()
		}

		if err != nil {
			break
		}
	}

	return buf.String()
}

func sendLine(t *testing.T, conn net.Conn, msg string) {
	t.Helper()
	if _, err := fmt.Fprintln(conn, msg); err != nil {
		t.Errorf("sendLine(%q): %v", msg, err)
	}
}

// joinChat performs the name-handshake and returns the greeting text.
func joinChat(t *testing.T, conn net.Conn, r *bufio.Reader, name string) string {
	t.Helper()
	greeting := readUntilConn(t, conn, r, server.NamePrompt, testTimeout)

	if !strings.Contains(greeting, server.NamePrompt) {
		t.Fatalf("name prompt not received; got: %q", greeting)
	}

	sendLine(t, conn, name)
	return greeting
}

// ── 1. Port & args ────────────────────────────────────────────────────────────

func TestDefaultPort(t *testing.T) {
	t.Parallel()
	s, err := server.NewServer(server.Config{})
	if err != nil {
		t.Skipf("port 8989 unavailable: %v", err)
	}
	defer s.Stop()
	if !strings.HasSuffix(listenAddr(s), ":"+server.PortDefault) {
		t.Errorf("want port %s, got addr %s", server.PortDefault, listenAddr(s))
	}
}

func TestCustomPort(t *testing.T) {
	t.Parallel()
	s := startTestServer(t, server.Config{})
	_, port, err := net.SplitHostPort(listenAddr(s))
	if err != nil {
		t.Fatalf("SplitHostPort: %v", err)
	}

	if port == "" || port == "0" {
		t.Errorf("expected a real port number, got %q", port)
	}

	c, err := net.DialTimeout("tcp", listenAddr(s), testTimeout)
	if err != nil {
		t.Fatalf("cannot connect: %v", err)
	}

	c.Close()
}

func TestParseArgs(t *testing.T) {
	t.Parallel()
	const usage = "[USAGE]: ./TCPChat $port"
	cases := []struct {
		name    string
		args    []string
		port    string
		wantErr string
	}{
		{"no args → default", []string{}, "", ""},
		{"one valid port", []string{"9090"}, "9090", ""},
		{"two args → usage error", []string{"9090", "extra"}, "", usage},
		{"three args → usage error", []string{"a", "b", "c"}, "", usage},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg, err := utils.ParseArgs(tc.args)

			if tc.wantErr != "" {
				if err == nil || err.Error() != tc.wantErr {
					t.Fatalf("error: got %v, want %q", err, tc.wantErr)
				}

				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if cfg.Port != tc.port {
				t.Errorf("port: got %q, want %q", cfg.Port, tc.port)
			}
		})
	}
}

// ── 2. Welcome sequence ───────────────────────────────────────────────────────

// TestWelcomeSequence checks every element the spec mandates on first connect:
// the Linux ASCII logo and the verbatim name prompt (including trailing space).
func TestWelcomeSequence(t *testing.T) {
	t.Parallel()
	s := startTestServer(t, server.Config{})
	conn, r := dialClient(t, listenAddr(s))
	got := readUntilConn(t, conn, r, server.NamePrompt, testTimeout)

	checks := []struct{ name, substr string }{
		{"welcome banner", "Welcome to TCP-Chat!"},
		{"logo _nnnn_", "_nnnn_"},
		{"logo dGGGGMMb", "dGGGGMMb"},
		{"logo MMMMMP", "MMMMMP"},
		{"exact prompt", "[ENTER YOUR NAME]: "},
	}

	for _, c := range checks {
		if !strings.Contains(got, c.substr) {
			t.Errorf("%s: %q not found in greeting:\n%s", c.name, c.substr, got)
		}
	}

	// Complete the handshake so the server does not fall through handleConnection.
	sendLine(t, conn, "WelcomeProbe")
}

// ── 3. Name handling ──────────────────────────────────────────────────────────

func TestNameHandling(t *testing.T) {
	t.Run("RepromptOnFirstEmpty", func(t *testing.T) {
		t.Parallel()
		s := startTestServer(t, server.Config{})
		conn, err := net.DialTimeout("tcp", listenAddr(s), testTimeout)
		if err != nil {
			t.Fatalf("dial: %v", err)
		}
		defer conn.Close()
		r := bufio.NewReader(conn)

		readUntilConn(t, conn, r, server.NamePrompt, testTimeout)

		sendLine(t, conn, "")

		conn.SetDeadline(time.Now().Add(testTimeout))
		buf := make([]byte, 256)
		n, readErr := conn.Read(buf)

		conn.SetDeadline(time.Time{})
		if readErr != nil {
			t.Errorf("server closed after first empty name instead of reprompting: %v", readErr)
			return
		}

		if n == 0 {
			t.Error("server sent nothing after first empty name; expected a reprompt or error message")
		}

		// Complete the handshake so the server goroutine exits cleanly.
		sendLine(t, conn, "RepromptProbe")
	})

	t.Run("RejectionAfterRetries", func(t *testing.T) {
		t.Parallel()
		s := startTestServer(t, server.Config{})

		// Observer: must NOT receive a join notification for the rejected client.
		connA, rA := dialClient(t, listenAddr(s))

		joinChat(t, connA, rA, "Alice")

		conn, err := net.DialTimeout("tcp", listenAddr(s), testTimeout)
		if err != nil {
			t.Fatalf("dial: %v", err)
		}
		defer conn.Close()
		r := bufio.NewReader(conn)

		readUntilConn(t, conn, r, server.NamePrompt, testTimeout)

		for _, bad := range []string{"", "   ", "\t"} {
			sendLine(t, conn, bad)
			time.Sleep(10 * time.Millisecond)
		}

		got := readUntilConn(t, conn, r, "exceeded", testTimeout)

		if !strings.Contains(got, "exceeded") {
			conn.SetDeadline(time.Now().Add(75 * time.Millisecond))
			buf := make([]byte, 1)
			_, closeErr := conn.Read(buf)

			conn.SetDeadline(time.Time{})
			if closeErr == nil {
				t.Errorf("server did not reject client after empty-name retries; got: %q", got)
			}
		}

		time.Sleep(20 * time.Millisecond)
		connA.SetDeadline(time.Now().Add(50 * time.Millisecond))
		notif := make([]byte, 512)
		n, _ := connA.Read(notif)

		connA.SetDeadline(time.Time{})
		if n > 0 && strings.Contains(string(notif[:n]), "joined") {
			t.Errorf("Alice received spurious join notification for rejected client: %q", string(notif[:n]))
		}

		_ = rA
	})
}

// ── 4. Message format ─────────────────────────────────────────────────────────

// matchesMessageFormat reports whether line conforms to the required chat
// message format: [YYYY-MM-DD HH:MM:SS][name]:
// Uses time.Parse instead of regexp so that only the allowed packages are used.
func matchesMessageFormat(line string) bool {
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

	rest := line[end+1:]
	// After the timestamp must come [name]:
	return strings.HasPrefix(rest, "[") && strings.Contains(rest, "]:")
}

// extractTimestamp parses the leading [YYYY-MM-DD HH:MM:SS] in line and
// returns the parsed time.  ok is false when the prefix is absent or invalid.
func extractTimestamp(line string) (ts time.Time, ok bool) {
	if !strings.HasPrefix(line, "[") {
		return
	}

	end := strings.Index(line, "]")

	if end < 1 {
		return
	}

	t, err := time.ParseInLocation("2006-01-02 15:04:05", line[1:end], time.Local)
	if err != nil {
		return
	}

	return t, true
}

// messageRE is kept as a named sentinel so callers can use strings.Contains on
// the result of matchesMessageFormat instead of this variable directly.
// It is retained only for compile-time documentation purposes.
var _ = matchesMessageFormat // suppress "declared and not used" if inlined

// ── 4. Message format ─────────────────────────────────────────────────────────
// the timestamp+name regex, the exact sender name in brackets, and that the
// timestamp is current (within ±5 s).
func TestMessageFormat(t *testing.T) {
	t.Parallel()
	s := startTestServer(t, server.Config{})

	connA, rA := dialClient(t, listenAddr(s))

	joinChat(t, connA, rA, "Alice")
	connB, rB := dialClient(t, listenAddr(s))

	joinChat(t, connB, rB, "Bob")
	readUntilConn(t, connA, rA, "joined", testTimeout)

	before := time.Now()
	const payload = "hello world"

	sendLine(t, connB, payload)
	got := readUntilConn(t, connA, rA, payload, testTimeout)
	after := time.Now()

	for _, line := range strings.Split(strings.TrimSpace(got), "\n") {
		line = strings.TrimSpace(line)
		if !strings.Contains(line, payload) {
			continue
		}

		if !matchesMessageFormat(line) {
			t.Errorf("format wrong: %q", line)
		}

		if !strings.Contains(line, "[Bob]:") {
			t.Errorf("sender name not in brackets: %q", line)
		}

		ts, ok := extractTimestamp(line)

		if !ok {
			t.Errorf("no valid timestamp in: %q", line)
			return
		}

		const window = 5 * time.Second

		if ts.Before(before.Add(-window)) || ts.After(after.Add(window)) {
			t.Errorf("timestamp %s outside expected window", ts.Format("2006-01-02 15:04:05"))
		}

		return
	}

	t.Errorf("never received %q; full output: %q", payload, got)
	_ = rB
}

// ── 5. Broadcast ──────────────────────────────────────────────────────────────

func TestBroadcastToOthersOnly(t *testing.T) {
	t.Parallel()
	s := startTestServer(t, server.Config{})

	connA, rA := dialClient(t, listenAddr(s))

	joinChat(t, connA, rA, "Alice")
	connB, rB := dialClient(t, listenAddr(s))

	joinChat(t, connB, rB, "Bob")
	connC, rC := dialClient(t, listenAddr(s))

	joinChat(t, connC, rC, "Carol")

	readUntilConn(t, connA, rA, "Carol", testTimeout)
	readUntilConn(t, connB, rB, "Carol", testTimeout)

	token := "uniquepayload42"

	sendLine(t, connA, token)

	if got := readUntilConn(t, connB, rB, token, testTimeout); !strings.Contains(got, token) {
		t.Errorf("Bob did not receive Alice's message; got: %q", got)
	}

	if got := readUntilConn(t, connC, rC, token, testTimeout); !strings.Contains(got, token) {
		t.Errorf("Carol did not receive Alice's message; got: %q", got)
	}

	// Sender must NOT receive her own message.
	connA.SetDeadline(time.Now().Add(50 * time.Millisecond))
	selfBuf := make([]byte, 512)
	n, _ := connA.Read(selfBuf)

	connA.SetDeadline(time.Time{})
	if strings.Contains(string(selfBuf[:n]), token) {
		t.Error("Alice received her own message (sender must be excluded from broadcast)")
	}
}

func TestEmptyMessageNotBroadcast(t *testing.T) {
	t.Parallel()
	s := startTestServer(t, server.Config{})

	connA, rA := dialClient(t, listenAddr(s))

	joinChat(t, connA, rA, "Alice")
	connB, rB := dialClient(t, listenAddr(s))

	joinChat(t, connB, rB, "Bob")
	readUntilConn(t, connA, rA, "joined", testTimeout)

	// Raw writes: exact bytes, no extra newline from Fprintln.
	for _, payload := range [][]byte{
		[]byte("\n"),
		[]byte("   \n"),
		[]byte("\t\n"),
		[]byte("  \t  \n"),
	} {
		if _, err := connB.Write(payload); err != nil {
			t.Fatalf("Write(%q): %v", payload, err)
		}
	}

	for _, empty := range []string{"", "   ", "\t", "  \t  "} {
		sendLine(t, connB, empty)
	}

	time.Sleep(50 * time.Millisecond)

	connA.SetDeadline(time.Now().Add(50 * time.Millisecond))
	buf := make([]byte, 512)
	n, _ := connA.Read(buf)

	connA.SetDeadline(time.Time{})
	if n > 0 {
		t.Errorf("Alice received data from empty/whitespace messages; got %q", string(buf[:n]))
	}

	_ = rB
}

// ── 6. Chat history ───────────────────────────────────────────────────────────

func TestChatHistoryDeliveredToNewClient(t *testing.T) {
	t.Parallel()
	s := startTestServer(t, server.Config{})

	connA, rA := dialClient(t, listenAddr(s))

	joinChat(t, connA, rA, "Alice")
	connB, rB := dialClient(t, listenAddr(s))

	joinChat(t, connB, rB, "Bob")
	readUntilConn(t, connA, rA, "joined", testTimeout)

	sendLine(t, connB, "first message")
	sendLine(t, connB, "second message")
	readUntilConn(t, connA, rA, "second message", testTimeout)

	// Carol joins late.
	connC, rC := dialClient(t, listenAddr(s))
	carolGreeting := joinChat(t, connC, rC, "Carol")
	extra := readUntilConn(t, connC, rC, "second message", testTimeout)
	all := carolGreeting + extra

	for _, msg := range []string{"first message", "second message"} {
		if !strings.Contains(all, msg) {
			t.Errorf("Carol did not receive history %q", msg)
		}
	}

	// History lines must carry the [YYYY-MM-DD HH:MM:SS][name]: format.
	for _, line := range strings.Split(strings.TrimSpace(extra), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		if strings.Contains(line, "first message") || strings.Contains(line, "second message") {
			if !matchesMessageFormat(line) {
				t.Errorf("history line format wrong: %q", line)
			}
		}
	}

	// System notifications must NOT appear in history.
	if strings.Contains(extra, "has joined our chat") || strings.Contains(extra, "has left our chat") {
		t.Errorf("Carol received system notifications in history (only user messages must be replayed):\n%s", extra)
	}

	_ = rB
	_ = rC
}

// TestHistoryPreservesChronologicalOrder verifies messages are replayed to
// late-joining clients in the exact order they were originally sent.
func TestHistoryPreservesChronologicalOrder(t *testing.T) {
	t.Parallel()
	s := startTestServer(t, server.Config{})

	connA, rA := dialClient(t, listenAddr(s))

	joinChat(t, connA, rA, "Alice")
	connB, rB := dialClient(t, listenAddr(s))

	joinChat(t, connB, rB, "Bob")
	readUntilConn(t, connA, rA, "joined", testTimeout)

	ordered := []string{"seq-alpha", "seq-beta", "seq-gamma", "seq-delta"}

	for _, m := range ordered {
		sendLine(t, connA, m)
		time.Sleep(5 * time.Millisecond)
	}

	readUntilConn(t, connB, rB, ordered[len(ordered)-1], testTimeout)

	connC, rC := dialClient(t, listenAddr(s))

	joinChat(t, connC, rC, "Carol")
	history := readUntilConn(t, connC, rC, ordered[len(ordered)-1], testTimeout)

	lineIndex := make(map[string]int)

	for idx, line := range strings.Split(strings.TrimSpace(history), "\n") {
		for _, m := range ordered {
			if strings.Contains(line, m) {
				lineIndex[m] = idx
			}
		}
	}

	for i := 1; i < len(ordered); i++ {
		prev, cur := ordered[i-1], ordered[i]
		ip, okp := lineIndex[prev]
		ic, okc := lineIndex[cur]

		if !okp {
			t.Errorf("history missing %q", prev)
			continue
		}

		if !okc {
			t.Errorf("history missing %q", cur)
			continue
		}

		if ip >= ic {
			t.Errorf("history out of order: %q (line %d) should precede %q (line %d)", prev, ip, cur, ic)
		}
	}

	_ = rB
	_ = rC
}

// ── 7. Notifications ──────────────────────────────────────────────────────────

func TestNotifications(t *testing.T) {
	t.Run("JoinNotifiesExistingClients", func(t *testing.T) {
		t.Parallel()
		s := startTestServer(t, server.Config{})

		connA, rA := dialClient(t, listenAddr(s))

		joinChat(t, connA, rA, "Alice")
		connB, rB := dialClient(t, listenAddr(s))

		joinChat(t, connB, rB, "Bob")

		got := readUntilConn(t, connA, rA, "joined", testTimeout)

		if !strings.Contains(got, "Bob") {
			t.Errorf("join notification missing 'Bob'; got: %q", got)
		}

		if !strings.Contains(got, "has joined our chat") {
			t.Errorf("join notification missing phrase; got: %q", got)
		}

		_ = rB
	})

	t.Run("JoinDoesNotNotifySelf", func(t *testing.T) {
		t.Parallel()
		s := startTestServer(t, server.Config{})

		conn, r := dialClient(t, listenAddr(s))

		joinChat(t, conn, r, "Alice")

		conn.SetDeadline(time.Now().Add(50 * time.Millisecond))
		buf := make([]byte, 512)
		n, _ := conn.Read(buf)

		conn.SetDeadline(time.Time{})

		if strings.Contains(string(buf[:n]), "Alice has joined") {
			t.Errorf("joining client received its own join notification: %q", string(buf[:n]))
		}

		_ = r
	})

	// LeaveNotifiesAllPeers covers both single-peer and multi-peer leave scenarios.
	t.Run("LeaveNotifiesAllPeers", func(t *testing.T) {
		t.Parallel()
		s := startTestServer(t, server.Config{})

		connA, rA := dialClient(t, listenAddr(s))

		joinChat(t, connA, rA, "Alice")
		connB, rB := dialClient(t, listenAddr(s))

		joinChat(t, connB, rB, "Bob")
		connC, rC := dialClient(t, listenAddr(s))

		joinChat(t, connC, rC, "Carol")

		readUntilConn(t, connA, rA, "Carol", testTimeout)
		readUntilConn(t, connB, rB, "Carol", testTimeout)

		connC.Close()

		for _, tc := range []struct {
			who  string
			conn net.Conn
			r    *bufio.Reader
		}{
			{"Alice", connA, rA},
			{"Bob", connB, rB},
		} {
			got := readUntilConn(t, tc.conn, tc.r, "left", testTimeout)

			if !strings.Contains(got, "Carol") {
				t.Errorf("%s: leave notification missing 'Carol'; got: %q", tc.who, got)
			}

			if !strings.Contains(got, "has left our chat") {
				t.Errorf("%s: leave notification missing phrase; got: %q", tc.who, got)
			}
		}

		_ = rC
	})
}

// ── 8. Broadcast completeness ─────────────────────────────────────────────────

// TestAllActiveClientsReceiveBroadcast is a stricter N-client variant of
// TestBroadcastToOthersOnly (spec: "All Clients must receive the messages").
func TestAllActiveClientsReceiveBroadcast(t *testing.T) {
	t.Parallel()
	const numReceivers = 4
	s := startTestServer(t, server.Config{})

	connSender, rSender := dialClient(t, listenAddr(s))

	joinChat(t, connSender, rSender, "Sender")

	type peer struct {
		conn net.Conn
		r    *bufio.Reader
		name string
	}
	peers := make([]peer, numReceivers)

	for i := range peers {
		name := fmt.Sprintf("Rcvr%d", i)
		conn, r := dialClient(t, listenAddr(s))

		joinChat(t, conn, r, name)
		peers[i] = peer{conn, r, name}
	}

	readUntilConn(t, connSender, rSender, fmt.Sprintf("Rcvr%d", numReceivers-1), testTimeout)

	token := "broadcast-all-XYZ"

	sendLine(t, connSender, token)

	for _, p := range peers {
		if got := readUntilConn(t, p.conn, p.r, token, testTimeout); !strings.Contains(got, token) {
			t.Errorf("%s did not receive broadcast %q", p.name, token)
		}
	}

	connSender.SetDeadline(time.Now().Add(50 * time.Millisecond))
	selfBuf := make([]byte, 512)
	n, _ := connSender.Read(selfBuf)

	connSender.SetDeadline(time.Time{})
	if strings.Contains(string(selfBuf[:n]), token) {
		t.Error("sender received its own broadcast")
	}
}

// ── 9. Connection limit ───────────────────────────────────────────────────────

func TestDefaultConnectionLimit(t *testing.T) {
	t.Parallel()
	const defaultMax = 10
	s := startTestServer(t, server.Config{})

	var conns []net.Conn

	t.Cleanup(func() {
		for _, c := range conns {
			c.Close()
		}
	})
	for i := 0; i < defaultMax; i++ {
		conn, r := dialClient(t, listenAddr(s))

		joinChat(t, conn, r, fmt.Sprintf("user%d", i))
		conns = append(conns, conn)
	}

	extra, err := net.DialTimeout("tcp", listenAddr(s), testTimeout)
	if err != nil {
		return // connection refused counts as rejection
	}
	defer extra.Close()

	extra.SetDeadline(time.Now().Add(testTimeout))
	buf := make([]byte, 256)
	n, readErr := extra.Read(buf)

	extra.SetDeadline(time.Time{})

	if readErr == nil && n > 0 {
		msg := strings.ToLower(string(buf[:n]))

		if !strings.Contains(msg, "full") && !strings.Contains(msg, "max") &&
			!strings.Contains(msg, "limit") && !strings.Contains(msg, "reject") {
			extra.SetDeadline(time.Now().Add(100 * time.Millisecond))
			_, closeErr := extra.Read(make([]byte, 1))

			if closeErr == nil {
				t.Error("11th connection was accepted; server must enforce the default limit of 10")
			}
		}
	}
}

// ── 10. Resilience ────────────────────────────────────────────────────────────

func TestRemainingClientsUnaffectedAfterDisconnect(t *testing.T) {
	t.Parallel()
	s := startTestServer(t, server.Config{})

	connA, rA := dialClient(t, listenAddr(s))

	joinChat(t, connA, rA, "Alice")
	connB, rB := dialClient(t, listenAddr(s))

	joinChat(t, connB, rB, "Bob")
	connC, rC := dialClient(t, listenAddr(s))

	joinChat(t, connC, rC, "Carol")

	readUntilConn(t, connA, rA, "Carol", testTimeout)
	readUntilConn(t, connB, rB, "Carol", testTimeout)

	connB.Close()
	time.Sleep(20 * time.Millisecond)

	token := "stillalive99"

	sendLine(t, connA, token)
	if got := readUntilConn(t, connC, rC, token, testTimeout); !strings.Contains(got, token) {
		t.Errorf("Carol lost connection after Bob left; did not receive %q", got)
	}

	connA.SetDeadline(time.Now().Add(50 * time.Millisecond))
	_, err := connA.Read(make([]byte, 1))

	connA.SetDeadline(time.Time{})
	if err != nil && strings.Contains(err.Error(), "EOF") {
		t.Error("Alice was disconnected after Bob left")
	}

	_ = rB
}

// TestServerStopIsClean verifies Stop() completes without panicking when
// active clients are present.
//
// Known failure mode: ChatRoom.stop() writes to s.chatRooms after
// Server.Stop() sets s.chatRooms = nil → nil-map panic.
func TestServerStopIsClean(t *testing.T) {
	t.Parallel()
	cfg := server.Config{Port: "0"}
	s, err := server.NewServer(cfg)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}

	go s.Run()

	time.Sleep(20 * time.Millisecond)

	conn, err := net.DialTimeout("tcp", listenAddr(s), testTimeout)
	if err != nil {
		s.Stop()
		t.Fatalf("dial: %v", err)
	}

	r := bufio.NewReader(conn)

	readUntilConn(t, conn, r, server.NamePrompt, testTimeout)
	fmt.Fprintln(conn, "Stopper")
	time.Sleep(20 * time.Millisecond)

	s.Stop()
	conn.Close()
}

func TestServerGracefulOnSuddenDisconnect(t *testing.T) {
	t.Parallel()
	s := startTestServer(t, server.Config{})

	connA, rA := dialClient(t, listenAddr(s))

	joinChat(t, connA, rA, "Alice")

	// Bob connects then immediately drops.
	connB, err := net.DialTimeout("tcp", listenAddr(s), testTimeout)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}

	connB.Close()
	time.Sleep(20 * time.Millisecond)

	connC, rC := dialClient(t, listenAddr(s))

	joinChat(t, connC, rC, "Carol")
	token := "serveralive"

	sendLine(t, connC, token)

	if got := readUntilConn(t, connA, rA, token, testTimeout); !strings.Contains(got, token) {
		t.Errorf("server appears to have crashed after abrupt disconnect; Alice got: %q", got)
	}

	_ = rC
}

// TestConcurrentClients connects numClients simultaneously. Run with -race.
func TestConcurrentClients(t *testing.T) {
	t.Parallel()
	const numClients = 5
	s := startTestServer(t, server.Config{MaxConnections: numClients})

	var wg sync.WaitGroup

	for i := 0; i < numClients; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			conn, r := dialClient(t, listenAddr(s))
			name := fmt.Sprintf("user%d", id)

			joinChat(t, conn, r, name)
			sendLine(t, conn, "hello from "+name)
			time.Sleep(10 * time.Millisecond)
			conn.Close()
		}(i)
	}

	done := make(chan struct{})

	go func() { wg.Wait(); close(done) }()

	select {
	case <-done:
	case <-time.After(testTimeout):
		t.Error("timed out waiting for concurrent clients")
	}
}
