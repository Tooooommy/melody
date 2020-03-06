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
	})

	m.HandleMessage(func(session *melody.Session, msg []byte) {
		if err := session.Publish(msg); err != nil {
			log.Println(err)
		}
	})

	m.HandleSentMessage(func(session *melody.Session, bytes []byte) {
		log.Printf("%+v", string(bytes))
	})
	router.Run(":8080")
}
