package melody

import (
	"sync"
)

type hub struct {
	sessions   map[*Session]bool
	broadcast  chan *envelope
	register   chan *Session
	unregister chan *Session
	exit       chan *envelope
	open       bool
	rwmutex    *sync.RWMutex
	// extends
	channels    map[string]*Session
	subscribe   chan *Session
	publish     chan *envelope
	unsubscribe chan *Session
}

func newHub() *hub {
	return &hub{
		sessions:   make(map[*Session]bool),
		broadcast:  make(chan *envelope),
		register:   make(chan *Session),
		unregister: make(chan *Session),
		exit:       make(chan *envelope),
		open:       true,
		rwmutex:    &sync.RWMutex{},
		// extends
		channels:    make(map[string]*Session),
		subscribe:   make(chan *Session),
		publish:     make(chan *envelope),
		unsubscribe: make(chan *Session),
	}
}

func (h *hub) run() {
loop:
	for {
		select {
		case s := <-h.register:
			h.rwmutex.Lock()
			h.sessions[s] = true
			h.rwmutex.Unlock()
		case s := <-h.unregister:
			if _, ok := h.sessions[s]; ok {
				h.rwmutex.Lock()
				delete(h.sessions, s)
				h.rwmutex.Unlock()
			}
		case m := <-h.broadcast:
			h.rwmutex.RLock()
			for s := range h.sessions {
				if m.filter != nil {
					if m.filter(s) {
						s.writeMessage(m)
					}
				} else {
					s.writeMessage(m)
				}
			}
			h.rwmutex.RUnlock()
		case m := <-h.exit:
			h.rwmutex.Lock()
			for s := range h.sessions {
				s.writeMessage(m)
				delete(h.sessions, s)
				s.Close()
			}
			h.open = false
			h.rwmutex.Unlock()
			break loop

		case s := <-h.subscribe:
			// extends
			h.rwmutex.Lock()
			if _, ok := h.channels[s.channel]; !ok {
				h.channels[s.channel] = s
			} else {
				h.channels[s.channel].prev = s // 原来的上一个指向现在的
				s.next = h.channels[s.channel] // 现在的下一个是原来的
				s.prev = nil                   // 成为队头
				h.channels[s.channel] = s      // 现在的替代原来的位置
			}
			h.rwmutex.Unlock()
		case s := <-h.unsubscribe:
			if _, ok := h.channels[s.channel]; ok {
				h.rwmutex.Lock()
				if s.next != nil {
					s.next.prev = s.prev
				}
				if s.prev != nil {
					s.prev.next = s.next
				} else {
					h.channels[s.channel] = s.next
				}
				s.channel = "" // 置空
				h.rwmutex.Unlock()
			}
		case m := <-h.publish:
			h.rwmutex.RLock()
			if _, ok := h.channels[m.c]; ok {
				for s := h.channels[m.c]; s != nil; s = s.next {
					if m.filter != nil {
						if m.filter(s) {
							s.writeMessage(m)
						}
					} else {
						s.writeMessage(m)
					}
				}
			}
			h.rwmutex.RUnlock()
		}
	}
}

func (h *hub) closed() bool {
	h.rwmutex.RLock()
	defer h.rwmutex.RUnlock()
	return !h.open
}

func (h *hub) len() int {
	h.rwmutex.RLock()
	defer h.rwmutex.RUnlock()

	return len(h.sessions)
}
