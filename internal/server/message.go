package server

import (
	"fmt"
	"time"
)

// Message represents a single message in a chat room. It could be either a
// real user message or a server system notification.
type Message struct {
	message   string    // message is the raw text message.
	timeStamp time.Time // timeStamp is the wall-clock time when the message was created.
	roomName  string    // roomName is the name of the room the message sender belongs to.

	Sender         *Client // Sender is the name of the sender of the message.
	IsNotification bool    // FromSystem indicates message is an event triggered by `Sender`.
}

// NewMessage initialises a normal message.
func NewMessage(text string, sender *Client, roomName string) *Message {
	return &Message{
		message:   text,
		timeStamp: time.Now(),
		roomName:  roomName,
		Sender:    sender,
	}
}

// NewSystemMessage initialises a system notification.
func NewSystemMessage(text string, sender *Client, roomName string) *Message {
	return &Message{
		message:        text,
		timeStamp:      time.Now(),
		roomName:       roomName,
		Sender:         sender,
		IsNotification: true,
	}
}

// ChatFormat returns the message as a string formatted for the chat room.
//
// User message format: `[YYYY-MM-DD HH:MM:SS][sender.name]:text`
//
// System messages have the sender part is replaced with the string "System".
// System notifications: `[YYYY-MM-DD HH:MM:SS][System]: text`
func (m *Message) ChatFormat() string {
	sender := m.Sender.Name()

	if m.IsNotification {
		sender = "System"
		m.message = " " + m.message
	}

	formattedTimeStamp := m.timeStamp.Format("2006-01-02 15:04:05")
	formattedChat := fmt.Sprintf("[%s][%s]:%s", formattedTimeStamp, sender, m.message)

	return formattedChat
}

// LogFormat returns the message as a string formatted for logging.
//
// User message format:		`[YYYY-MM-DD HH:MM:SS][sender.name][roomName]:text`
// System notifications:	`[YYYY-MM-DD HH:MM:SS][System][roomName]: text`
func (m *Message) LogFormat() string {
	sender := m.Sender.Name()

	if m.IsNotification {
		sender = "System"
		m.message = " " + m.message
	}

	formattedTimeStamp := m.timeStamp.Format("2006-01-02 15:04:05")
	formattedLog := fmt.Sprintf("[%s][%s][%s]:%s", formattedTimeStamp, sender, m.roomName, m.message)

	return formattedLog
}
