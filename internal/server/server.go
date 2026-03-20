package server

import (
	"bufio"
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"strings"
	"sync"
)

// Server contains all the context needed for a TCP-Chat program.
type Server struct {
	net.Listener
	configs Config
	// chatRooms tracks active chatRooms in the server.
	chatRooms map[string]*ChatRoom
	// roomsMu guards chatRooms so multiple goroutines can look up without
	// race conditions.
	roomsMu sync.RWMutex
	// roomsWg blocks Stop() until every ChatRoom.Run() goroutine returns
	// before closing the logger.Logs channel.
	roomsWg sync.WaitGroup
	// logger is the global chat logger.
	logger *Logger
	// loggerWg blocks Stop() until Logger.Run() goroutine returns.
	loggerWg sync.WaitGroup
	// connsCount tracks active connections in the server.
	connsCount int
	// connsMu controls access to `connsCount` by multiple goroutines.
	connsMu sync.Mutex
	// Done is used to broadcast the shutdown signal to the whole server.
	Done chan struct{}
}

// NewServer returns an initialised Server.
func NewServer(configs Config) (*Server, error) {
	if len(configs.Port) < 1 {
		configs.Port = PortDefault
	}

	if configs.MaxConnections == 0 {
		configs.MaxConnections = MaxConnectionsDefault
	}

	listener, err := net.Listen("tcp", ":"+configs.Port)
	if err != nil {
		return nil, err
	}

	logger, err := NewLogger(configs.LogFile, configs.MaxConnections)
	if err != nil {
		listener.Close()
		return nil, err
	}

	generalChatRoom, err := NewChatRoom("general", logger, configs.MaxConnections)
	if err != nil {
		listener.Close()
		logger.Stop()
		return nil, err
	}

	return &Server{
		Listener:  listener,
		configs:   configs,
		logger:    logger,
		chatRooms: map[string]*ChatRoom{"general": generalChatRoom},
		Done:      make(chan struct{}),
	}, nil
}

// ConnsCountAdd adds `delta` (can be negative), if possible, to Server.connsCount
// and returns its value.
// This method is safe to be called by multiple goroutines.
func (s *Server) ConnsCountAdd(delta int) (int, error) {
	s.connsMu.Lock()
	defer s.connsMu.Unlock() // Ensure mutex is unlocked when function returns.
	count := s.connsCount

	if delta > 0 && s.configs.MaxConnections > 0 && count >= s.configs.MaxConnections {
		return count, errors.New("max connections reached. Try again later")
	}

	if count < 0 {
		count = 0
	}

	if delta < 0 && count == 0 {
		return count, errors.New("cannot reduce connections below 0")
	}

	count += delta
	s.connsCount = count
	return count, nil
}

// Run starts all background goroutines and then listens for new connections.
func (s *Server) Run() {
	_, port, _ := net.SplitHostPort(s.Listener.Addr().String())

	fmt.Fprintf(os.Stderr, "Listening on the port :%v\n", port)

	// Start logger loop
	s.loggerWg.Add(1)
	go func() {
		defer s.loggerWg.Done()
		s.logger.Run()
	}()

	// Start all chat room loops
	s.roomsMu.RLock()
	for _, room := range s.chatRooms {
		s.roomsWg.Add(1)
		go func() { defer s.roomsWg.Done(); room.Run(s) }()
	}

	s.roomsMu.RUnlock()

	for {
		conn, err := s.Accept()
		if err != nil {
			select {
			case <-s.Done:
				return // server has been stopped, listener has been closed, exit the loop.
			default:
				log.Println("Accept error:", err)
				continue
			}
		}

		if _, err := s.ConnsCountAdd(1); err != nil {
			go func() { fmt.Fprintln(conn, err); conn.Close() }()

			continue
		}

		go s.handleConnection(conn)
	}
}

// ListChatRooms returns a slice of the names of all the chat rooms in the server.
func (s *Server) ListChatRooms() []string {
	var chatRoomNames []string

	s.roomsMu.RLock()
	for _, chatRoom := range s.chatRooms {
		chatRoomNames = append(chatRoomNames, chatRoom.Name())
	}

	s.roomsMu.RUnlock()
	return chatRoomNames
}

// GetChatRoom returns pointer from the list of chat rooms in the server if it exists,
// otherwise creates the chat room and returns its pointer.
func (s *Server) GetChatRoom(name string) (*ChatRoom, error) {
	s.roomsMu.Lock()
	defer s.roomsMu.Unlock()
	if s.chatRooms == nil {
		s.chatRooms = make(map[string]*ChatRoom)
	}

	if r, exists := s.chatRooms[name]; exists {
		return r, nil
	}

	room, err := NewChatRoom(name, s.logger, s.configs.MaxConnections)
	if err != nil {
		return nil, err
	}

	s.chatRooms[name] = room
	s.roomsWg.Add(1)
	go func() {
		defer s.roomsWg.Done()
		room.Run(s)
	}()

	return room, nil
}

// CloseChatRoom signals a chat room to terminate.
func (s *Server) CloseChatRoom(room *ChatRoom) {
	select {
	case <-room.Done: // room has already been closed.
	default:
		close(room.Done)
	}

	s.roomsMu.Lock()
	delete(s.chatRooms, room.Name())
	s.roomsMu.Unlock()
}

// Stop shuts the server down cleanly in a guaranteed order so no goroutine
// reads from or writes to a channel after the channel has been closed.
func (s *Server) Stop() {
	select {
	case <-s.Done: // Check if the server is already stopped.
		return // A closed channel returns immediately.
	default:
		close(s.Done)
	}

	s.Listener.Close()
	s.roomsMu.RLock()
	for _, room := range s.chatRooms {
		select {
		case <-room.Done: // room has already been closed.
		default:
			close(room.Done) // signal room to terminate.
		}
	}

	s.roomsMu.RUnlock()

	// Wait for all chat rooms before closing the logger channel.
	s.roomsWg.Wait()
	s.roomsMu.Lock()
	s.chatRooms = nil
	s.roomsMu.Unlock()
	close(s.logger.Logs)
	// Wait for logger to flush its buffer.
	s.loggerWg.Wait()
	log.Println("Server shutdown. Bye!")
}

// handleConnection is a goroutine that performs a handshake with a Client
// and starts up the Client's I/O pumps.
func (s *Server) handleConnection(conn net.Conn) {
	scanner := bufio.NewScanner(conn)
	name := s.welcomeChatter(conn, scanner)

	if len(name) < 1 {
		conn.Close()
		s.ConnsCountAdd(-1)
		return
	}

	s.roomsMu.RLock()
	generalRoom := s.chatRooms["general"]

	s.roomsMu.RUnlock()
	client, err := NewClient(conn, name, generalRoom, scanner)
	if err != nil {
		log.Println("Error creating client:", err)
		conn.Close()
		s.ConnsCountAdd(-1)
		return
	}

	log.Println("New client connected:", client)
	// Start the write pump BEFORE registering so that when the ChatRoom sends
	// history entries to the client.Receive channel, there is already a
	// goroutine draining it.
	// Without this ordering the Register send could deadlock if Receive fills up.
	go client.WriteMessages()

	select {
	case generalRoom.Register <- client:
	case <-s.Done: // Server is shutting down.
		close(client.Receive)
		conn.Close()
		s.ConnsCountAdd(-1)
		return
	}

	// ReadMessages blocks here until the client disconnects.
	client.ReadMessages(s)
}

// welcomeChatter prints the welcome message and retrieves the Client name.
// Errors are logged to the appropriate channels and `name` will be
// an empty string.
func (s *Server) welcomeChatter(conn net.Conn, scanner *bufio.Scanner) (name string) {
	_, writeErr := fmt.Fprintf(conn, "%s\n%s", WelcomeMessage, NamePrompt)
	if writeErr != nil {
		log.Println("Error writing to connection [", conn.RemoteAddr(), "]:", writeErr)
		return ""
	}

	retries := 3

	for i := 0; len(name) < 1 && writeErr == nil && i < retries && scanner.Scan(); i++ {
		name = strings.TrimSpace(scanner.Text())
		if len(name) < 1 {
			_, writeErr = fmt.Fprintf(conn, "[NAME CANNOT BE EMPTY. RETRIES LEFT %d]: ", retries-i)
		}
	}

	if readErr := scanner.Err(); writeErr != nil || readErr != nil || len(name) < 1 {
		if writeErr != nil {
			log.Println("Error writing to connection [", conn.RemoteAddr(), "]:", writeErr)
		}

		if readErr != nil {
			log.Println("Error reading from connection [", conn.RemoteAddr(), "]:", readErr)
		}

		if len(name) < 1 {
			fmt.Fprintln(conn, "Retries exceeded, closing connection.")
		}

		return ""
	}

	return name
}
