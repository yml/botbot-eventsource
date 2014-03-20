package main

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/codegangsta/martini"
	"github.com/donovanhide/eventsource"
	"github.com/monnand/goredis"
)

type Message struct {
	Idx           string
	Channel, Html string
}

func (c *Message) Id() string    { return c.Idx }
func (c *Message) Event() string { return c.Channel }
func (c *Message) Data() string {
	return c.Html
}

type Connection struct {
	token   string
	channel string
}

type Hub struct {
	Data       map[string][]string // Key is the channel, value is a slice of token
	Users      map[string]string   // Key is the token, value is a channel
	register   chan Connection
	unregister chan string
	messages   chan goredis.Message
	srv        *eventsource.Server
	client     goredis.Client
}

func (h *Hub) userExists(token string) bool {
	_, ok := h.Users[token]
	return ok
}

func (h *Hub) run() {
	fmt.Println("Start the Hub")
	var payload [3]string
	psub := make(chan string, 0)
	go h.client.Subscribe(nil, nil, psub, nil, h.messages)

	// Listening to all channel updates
	psub <- "channel_update:*"

	for {
		select {
		case conn := <-h.register:
			fmt.Println("register user: ", conn.token)
			// TODO try to get the channel
			h.Users[conn.token] = conn.channel
			//fmt.Println("[DEBUG] After h.Users assignment", h.Users[conn.token])
			h.Data[conn.channel] = append(h.Data[conn.channel], conn.token)
			//fmt.Println("[DEBUG] After h.Data assignment", h.Data[conn.channel])

		case token := <-h.unregister:
			fmt.Println("unregister user: ", token)
			ch, ok := h.Users[token]
			if ok {
				delete(h.Users, token)
				delete(h.Data, ch)
			}

		case msg := <-h.messages:
			err := json.Unmarshal(msg.Message, &payload)
			if err != nil {
				fmt.Println("[Error] An error occured while Unmarshalling the msg: ", msg)
			}
			message := &Message{
				Idx:     payload[2],
				Channel: payload[0],
				Html:    payload[1],
			}
			val, ok := h.Data[msg.Channel]
			if ok && len(val) >= 1 {
				fmt.Println("[DEBUG] msg sent to tokens", val)
				h.srv.Publish(val, message)
			}
		}
	}
}

func NewHub() *Hub {
	h := Hub{
		Data:       make(map[string][]string),
		Users:      make(map[string]string),
		register:   make(chan Connection, 0),
		unregister: make(chan string, 0),
		messages:   make(chan goredis.Message, 0),
		srv:        eventsource.NewServer(),
	}
	// We use the second redis database for the pub/sub
	h.client.Db = 2
	return &h
}

func main() {
	h := NewHub()
	go h.run()

	m := martini.Classic()
	// eventsource endpoints
	m.Get("/push/:token", func(w http.ResponseWriter, req *http.Request, params martini.Params) {
		token := params["token"]

		if h.userExists(token) {
			// TODO proper resonse
			fmt.Fprintf(w, "Not allowed -- User already connected")
		} else {
			fmt.Println("Exchange token against the channel")
			ch, err := h.client.Getset(token, []byte{})
			if err != nil {
				fmt.Fprintf(w, "Not allowed -- Error occured while exchanging the token")
			} else {
				h.register <- Connection{token, string(ch)}
				defer func(u string) {
					h.unregister <- u
				}(token)
				h.srv.Handler(token)(w, req)
			}
		}
	})
	m.Run()

}
