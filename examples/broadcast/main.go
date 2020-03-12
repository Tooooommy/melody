package main

import (
	"log"
	"melody"

	"github.com/gin-gonic/gin"
)

func main() {
	m := melody.New()
	router := gin.Default()
	router.GET("/chat", func(context *gin.Context) {
		if err := m.HandleRequest(context.Writer, context.Request); err != nil {
			log.Println(err)
		}
	})

	m.HandleConnect(func(session *melody.Session) {
		ch := session.Request.URL.Query().Get("channel")
		if err := session.Subscribe(ch); err != nil {
			log.Println(err)
		}
		session.Set("channel", ch)
	})

	m.HandleMessage(func(session *melody.Session, msg []byte) {
		channel, ok1 := session.Get("channel")
		if ok1 {
			_ = m.BroadcastFilter(msg, func(s *melody.Session) bool {
				ch, ok2 := s.Get("channel")
				if ok2 && ch.(string) == channel.(string) {
					return true
				}
				return false
			})
		}
	})

	m.HandleSentMessage(func(session *melody.Session, bytes []byte) {
		log.Printf("%+v", session.Channel().Online())
		log.Printf("%+v", string(bytes))
	})
	router.Run(":8080")
}
