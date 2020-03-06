package melody

import (
	"errors"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// Session wrapper around websocket connections.
type Session struct {
	Request *http.Request
	Keys    map[string]interface{}
	conn    *websocket.Conn
	output  chan *envelope
	melody  *Melody
	open    bool
	rwmutex *sync.RWMutex
	// extends
	channel string
	next    *Session
	prev    *Session
}

func (s *Session) writeMessage(message *envelope) {
	if s.closed() {
		s.melody.errorHandler(s, errors.New("tried to write to closed a session"))
		return
	}

	select {
	case s.output <- message:
	default:
		s.melody.errorHandler(s, errors.New("session message buffer is full"))
	}
}

func (s *Session) writeRaw(message *envelope) error {
	if s.closed() {
		return errors.New("tried to write to a closed session")
	}

	s.conn.SetWriteDeadline(time.Now().Add(s.melody.Config.WriteWait))
	err := s.conn.WriteMessage(message.t, message.msg)

	if err != nil {
		return err
	}

	return nil
}

func (s *Session) closed() bool {
	s.rwmutex.RLock()
	defer s.rwmutex.RUnlock()

	return !s.open
}

func (s *Session) close() {
	if !s.closed() {
		s.rwmutex.Lock()
		s.open = false
		s.conn.Close()
		close(s.output)
		s.rwmutex.Unlock()
	}
}

func (s *Session) ping() {
	s.writeRaw(&envelope{t: websocket.PingMessage, msg: []byte{}})
}

func (s *Session) writePump() {
	ticker := time.NewTicker(s.melody.Config.PingPeriod)
	defer ticker.Stop()

loop:
	for {
		select {
		case msg, ok := <-s.output:
			if !ok {
				break loop
			}

			err := s.writeRaw(msg)

			if err != nil {
				s.melody.errorHandler(s, err)
				break loop
			}

			if msg.t == websocket.CloseMessage {
				break loop
			}

			if msg.t == websocket.TextMessage {
				s.melody.messageSentHandler(s, msg.msg)
			}

			if msg.t == websocket.BinaryMessage {
				s.melody.messageSentHandlerBinary(s, msg.msg)
			}
		case <-ticker.C:
			s.ping()
		}
	}
}

func (s *Session) readPump() {
	s.conn.SetReadLimit(s.melody.Config.MaxMessageSize)
	s.conn.SetReadDeadline(time.Now().Add(s.melody.Config.PongWait))

	s.conn.SetPongHandler(func(string) error {
		s.conn.SetReadDeadline(time.Now().Add(s.melody.Config.PongWait))
		s.melody.pongHandler(s)
		return nil
	})

	if s.melody.closeHandler != nil {
		s.conn.SetCloseHandler(func(code int, text string) error {
			return s.melody.closeHandler(s, code, text)
		})
	}

	for {
		t, message, err := s.conn.ReadMessage()

		if err != nil {
			s.melody.errorHandler(s, err)
			break
		}

		if t == websocket.TextMessage {
			s.melody.messageHandler(s, message)
		}

		if t == websocket.BinaryMessage {
			s.melody.messageHandlerBinary(s, message)
		}
	}
}

// Write writes message to session.
func (s *Session) Write(msg []byte) error {
	if s.closed() {
		return errors.New("session is closed")
	}

	s.writeMessage(&envelope{t: websocket.TextMessage, msg: msg})

	return nil
}

// WriteBinary writes a binary message to session.
func (s *Session) WriteBinary(msg []byte) error {
	if s.closed() {
		return errors.New("session is closed")
	}

	s.writeMessage(&envelope{t: websocket.BinaryMessage, msg: msg})

	return nil
}

// Close closes session.
func (s *Session) Close() error {
	if s.closed() {
		return errors.New("session is already closed")
	}

	s.writeMessage(&envelope{t: websocket.CloseMessage, msg: []byte{}})

	return nil
}

// CloseWithMsg closes the session with the provided payload.
// Use the FormatCloseMessage function to format a proper close message payload.
func (s *Session) CloseWithMsg(msg []byte) error {
	if s.closed() {
		return errors.New("session is already closed")
	}

	s.writeMessage(&envelope{t: websocket.CloseMessage, msg: msg})

	return nil
}

// Set is used to store a new key/value pair exclusivelly for this session.
// It also lazy initializes s.Keys if it was not used previously.
func (s *Session) Set(key string, value interface{}) {
	if s.Keys == nil {
		s.Keys = make(map[string]interface{})
	}

	s.Keys[key] = value
}

// Get returns the value for the given key, ie: (value, true).
// If the value does not exists it returns (nil, false)
func (s *Session) Get(key string) (value interface{}, exists bool) {
	if s.Keys != nil {
		value, exists = s.Keys[key]
	}

	return
}

// MustGet returns the value for the given key if it exists, otherwise it panics.
func (s *Session) MustGet(key string) interface{} {
	if value, exists := s.Get(key); exists {
		return value
	}

	panic("Key \"" + key + "\" does not exist")
}

// IsClosed returns the status of the connection.
func (s *Session) IsClosed() bool {
	return s.closed()
}

// extends
func (s *Session) Subscribe(c string) error {
	//
	if s.closed() {
		return errors.New("tried to write to closed a session")
	}

	if s.channel != "" && s.channel != c { // 存在订阅 并与接下来的channel无法共存
		return errors.New("session already subscribe channel")
	} else {
		s.channel = c
		s.melody.hub.subscribe <- s
	}
	return nil
}

func (s *Session) Unsubscribe() error {
	if s.closed() {
		return errors.New("tried to write to closed a session")
	}
	if s.channel == "" {
		return errors.New("session not yet subscribe channel")
	} else {
		s.melody.hub.unsubscribe <- s
	}
	return nil
}

func (s *Session) Publish(msg []byte) error {
	return s.PublishFilter(msg, nil)
}

func (s *Session) PublishFilter(msg []byte, fn func(*Session) bool) error {
	if s.melody.hub.closed() {
		return errors.New("melody instance is closed")
	}
	message := &envelope{t: websocket.TextMessage, msg: msg, c: s.channel, filter: fn}
	s.melody.hub.publish <- message
	return nil
}

func (s *Session) PublishOthers(msg []byte) error {
	return s.PublishFilter(msg, func(session *Session) bool {
		return s != session
	})
}

func (s *Session) PublishBinary(msg []byte) error {
	return s.PublishBinaryFilter(msg, nil)
}

func (s *Session) PublishBinaryOthers(msg []byte) error {
	return s.PublishBinaryFilter(msg, func(session *Session) bool {
		return s != session
	})
}

func (s *Session) PublishBinaryFilter(msg []byte, fn func(*Session) bool) error {
	if s.melody.hub.closed() {
		return errors.New("melody instance is closed")
	}

	message := &envelope{t: websocket.BinaryMessage, msg: msg, c: s.channel, filter: fn}
	s.melody.hub.publish <- message
	return nil
}

func (s *Session) IsSubscribed() bool {
	return s.channel != ""
}

func (s *Session) Channel() *Channel {
	if c, ok := s.melody.hub.channels[s.channel]; ok {
		return c
	} else {
		return nil
	}
}
