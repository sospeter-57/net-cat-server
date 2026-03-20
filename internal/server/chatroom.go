package server

import (
	"errors"
	"fmt"
	"strings"
)

// empty is a zero-size sentinel type. It can be used as a value where it
// doesn't matter what the value is.
type empty struct{}

// retChan is a generic return channel.
type retChan[T any] chan T

// ChatRoom is a hub for conversations.
// One goroutine runs the method Run() and all state changes go through
// its channels so no extra locking is needed for chatters or history.
type ChatRoom struct {
	name   string
	logger *Logger
	// chatters holds every client currently in this room.
	chatters map[*Client]empty
	// history is a flat slice of every ChatFormat()-ted message ever sent to
	// this room.
	history []string
	// Done is used to signal the event loop to exit gracefully.
	// Using a channel (rather than a boolean + mutex) means the Run() method
	// can select on it alongside Register/Unregister/Broadcast without polling.
	Done chan empty

	ChattersListReq chan retChan[[]string]
	Register        chan *Client  // Server sends a new `*Client`s here.
	Unregister      chan *Client  // Clients leaving the room are sent here first.
	Broadcast       chan *Message // Clients send their outgoing messages here.
}

// nonBlockingSend pushes `data` onto a channel, if the channel is blocked,
// it pops the oldest item in the channel and retries this process until a
// successful send.
func nonBlockingSend[T any](to chan T, data T) {
	for {
		select {
		case to <- data: // send is successful
			return
		default: // channel is full
			<-to // pop oldest item
		}
	}
}

// NewChatRoom allocates a ChatRoom with all channels initialized.
func NewChatRoom(name string, logger *Logger, maxChatters int) (*ChatRoom, error) {
	name = strings.TrimSpace(name)
	if len(name) < 1 {
		return nil, errors.New("name cannot be empty")
	}

	if logger == nil {
		return nil, errors.New("Logger cannot be nil")
	}

	if maxChatters < 1 || maxChatters > MaxBufferedClients {
		maxChatters = MaxBufferedClients
	}

	return &ChatRoom{
		name:     name,
		logger:   logger,
		chatters: make(map[*Client]empty),
		Done:     make(chan empty),

		Register:        make(chan *Client, maxChatters),
		Unregister:      make(chan *Client, maxChatters),
		ChattersListReq: make(chan retChan[[]string], maxChatters),
		Broadcast:       make(chan *Message, MaxClientBufferSize*maxChatters),
	}, nil
}

// Name returns the name of the ChatRoom.
func (cr *ChatRoom) Name() string { return cr.name }

// Run listens to the register, unregister and broadcast channels continuously.
func (cr *ChatRoom) Run(s *Server) {
	defer cr.stop(s)

	for {
		select {
		case <-cr.Done:
			return // Room should close immediately, exit the loop.
		case newClient := <-cr.Register:
			cr.chatters[newClient] = empty{}
			for _, msg := range cr.history {
				newClient.Receive <- msg
			}

			// broadcast client has joined
			joinMsg := fmt.Sprintf("%q has joined our chat", newClient.Name())

			cr.Broadcast <- NewSystemMessage(joinMsg, newClient, cr.name)
		case toRemove := <-cr.Unregister:
			if _, exists := cr.chatters[toRemove]; !exists {
				continue
			}

			cr.dropClient(toRemove)
			if len(cr.chatters) < 1 {
				s.CloseChatRoom(cr)
			}
		case newMessage := <-cr.Broadcast:
			// ignore empty messages
			if strings.TrimSpace(newMessage.message) == "" {
				continue
			}

			chatMessage := newMessage.ChatFormat()

			if !newMessage.IsNotification {
				cr.history = append(cr.history, chatMessage)
				cr.logger.Logs <- newMessage.LogFormat()
			}

			// broadcast message to everyone else except sender
			for client := range cr.chatters {
				// compare client pointers: newMessage.Sender is a *Client
				if client != newMessage.Sender {
					nonBlockingSend(client.Receive, chatMessage)
				}
			}
		case returnChannel := <-cr.ChattersListReq:
			names := make([]string, 0, len(cr.chatters))

			for chatter := range cr.chatters {
				names = append(names, chatter.Name())
			}

			returnChannel <- names
			close(returnChannel)
		}
	}
}

// dropClient removes a client from the chatters map and closes its Receive
// channel in one step.
func (cr *ChatRoom) dropClient(toDrop *Client) {
	delete(cr.chatters, toDrop)
	leaveMsg := fmt.Sprintf("%s has left our chat", toDrop.name)

	cr.Broadcast <- NewSystemMessage(leaveMsg, toDrop, cr.name)
}

// stop runs after the event loop exits and ensures no goroutines are
// left blocked on a Receive channel.
func (cr *ChatRoom) stop(s *Server) {
	cr.chatters = nil
}
