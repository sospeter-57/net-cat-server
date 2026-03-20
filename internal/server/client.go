package server

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"strings"
)

// Client represents one connected TCP participant.
type Client struct {
	name    string    // Display name, should not be empty.
	room    *ChatRoom // The room this client is currently participating in.
	conn    net.Conn
	scanner *bufio.Scanner // Buffered message reader to reduce syscalls.

	// Receive is written to by the ChatRoom and drained by WriteMessages.
	// As ChatRoom is the sole sender on this channel, it should be the one to
	// close the channel.  https://go.dev/tour/concurrency/4
	Receive chan string
	// Done signals the WriteMessages goroutine to return.
	Done chan empty
}

// NewClient allocates a Client for an already-connected socket.
func NewClient(conn net.Conn, name string, room *ChatRoom, scanner *bufio.Scanner) (*Client, error) {
	name = strings.TrimSpace(name)
	if len(name) < 1 {
		return nil, errors.New("Name cannot be empty")
	}

	if room == nil {
		return nil, errors.New("room cannot be nil")
	}

	if scanner == nil {
		return nil, errors.New("scanner cannot be nil")
	}

	return &Client{
		name:    name,
		room:    room,
		conn:    conn,
		scanner: scanner,

		// The channel is buffered so the ChatRoom's event loop is not blocked on a
		// slow connection.
		Receive: make(chan string, MaxClientBufferSize),
		Done:    make(chan empty),
	}, nil
}

// Name returns the Client's name.
func (c *Client) Name() string { return c.name }

// SetName updates the Client's name.
func (c *Client) SetName(newName string) error {
	newName = strings.TrimSpace(newName)
	if len(newName) < 1 {
		return errors.New("name cannot be empty")
	}

	c.name = newName
	return nil
}

// String implements the Stringer interface for Client.
func (c *Client) String() string { return fmt.Sprintf("[%s->%v]", c.name, c.conn.RemoteAddr()) }

// ReadMessages is the inbound half of a Client's I/O pair.
// It runs in its own goroutine and owns the connection's read side.
func (c *Client) ReadMessages(server *Server) {
	defer c.dropConnection(server)

	for c.scanner.Scan() {
		text := c.scanner.Text()

		if strings.HasPrefix(text, CommandPrefix) && c.handleCommand(text, server) {
			continue
		}

		c.room.Broadcast <- NewMessage(text, c, c.room.Name())
	}

	if err := c.scanner.Err(); err != nil && err != io.EOF {
		log.Println("Error reading from connection", c, ":", err)
	}
}

// WriteMessages is the outbound half of a client's I/O pair.
// It runs in its own goroutine and owns the connection's write side.
func (c *Client) WriteMessages() {
	for {
		select {
		case <-c.Done: // Client is disconnecting from server
			return
		case message := <-c.Receive:
			if _, err := fmt.Fprintln(c.conn, message); err != nil {
				log.Println("Error writing to connection", c, ":", err)
			}
		}
	}
}

// dropConnection is called by ReadMessages when it returns. It performs a tear-down
// of the connection to the server in the correct order.
func (c *Client) dropConnection(server *Server) {
	// The ChatRoom (sender) should handle closing of the c.Receive channel.
	c.room.Unregister <- c
	select {
	case <-c.Done:
	default:
		close(c.Done)
	}

	if err := c.conn.Close(); err != nil {
		log.Printf("Error while closing connection %v: %v\n", c, err)
	} else {
		log.Println("Client has disconnected:", c)
	}

	server.connsMu.Lock()
	server.connsCount--
	server.connsMu.Unlock()
}

// BONUS

// handleCommand parses and executes an in-chat command.
//
// Command list:
// ~chatters           list all the chatters in the current room.
// ~help, ~commands    print list of available commands and their purpose.
// ~join <roomName>    join the chat room named by <roomName>.
// ~name <newName>     change your name to whatever you replace <newName> with.
// ~rooms              list all the rooms available in the server.
//
//	Returns true on success, false otherwise.
func (c *Client) handleCommand(cmd string, s *Server) bool {
	if !strings.HasPrefix(cmd, CommandPrefix) {
		return false
	}

	cmd = cmd[1:]
	fields := strings.Fields(cmd)

	if len(fields) == 0 {
		return false
	}

	echoToClient := func(text string) {
		nonBlockingSend(c.Receive, NewSystemMessage(text, c, c.room.Name()).ChatFormat())
	}

	switch fields[0] {
	case "name":
		if len(fields) < 2 {
			echoToClient(fmt.Sprintf("Usage: %sname <newname>", CommandPrefix))
			return false
		}

		newName := strings.TrimSpace(strings.Join(fields[1:], " "))
		oldName := c.Name()

		if err := c.SetName(newName); err != nil {
			echoToClient("Invalid name: " + err.Error())
			return true
		}

		echoToClient(fmt.Sprintf("You are now known as %q", newName))
		c.room.Broadcast <- NewSystemMessage(fmt.Sprintf("%q is now known as %q", oldName, newName), c, c.room.Name())
	case "join":
		if len(fields) != 2 {
			echoToClient(fmt.Sprintf("Usage: %sjoin <roomName>", CommandPrefix))
			return false
		}

		newRoomName := strings.TrimSpace(fields[1])
		target, err := s.GetChatRoom(newRoomName)
		if err != nil {
			echoToClient(fmt.Sprintf("Error creating room %q: %v", newRoomName, err))
			return true
		}

		if c.room.Name() == target.Name() {
			echoToClient(fmt.Sprintf("You are already in room %q", target.Name()))
			return true
		}

		if c.room != nil {
			c.room.Unregister <- c
		}

		target.Register <- c
		c.room = target
		echoToClient(fmt.Sprintf("You have joined room %q", target.Name()))
		c.room.Broadcast <- NewSystemMessage(fmt.Sprintf("%q wants to join room %q", c.Name(), newRoomName), c, c.room.Name())
	case "chatters":
		request := make(retChan[[]string])

		c.room.ChattersListReq <- request
		names := <-request

		if len(names) < 1 {
			// UNCLEAR:
			// Unless command is being used on a room the client is not currently in
			// then there will always be atleast one person.
			echoToClient(fmt.Sprintf("No one is here %q", c.room.Name()))
			return true
		}

		echoToClient(fmt.Sprintf("Chatters in room %q: %q", c.room.Name(), names))
	case "rooms":
		chatRoomSlice := s.ListChatRooms()

		echoToClient(fmt.Sprintf("Rooms Available %q", chatRoomSlice))
	case "h", "help", "commands":
		commands := fmt.Sprintf(`List of commands:
%[1]schatters           list all the chatters in the current room.
%[1]shelp, %[1]scommands    print list of available commands and their purpose.
%[1]sjoin <roomName>    join the chat room named by <roomName>.
%[1]sname <newName>     change your name to whatever you replace <newName> with.
%[1]srooms              list all the rooms available in the server.`, CommandPrefix)
		echoToClient(commands)
	default:
		return false
	}

	return true
}
