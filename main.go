package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

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
	b, _ := json.Marshal(c)
	return string(b)
}

type IdGenerator struct {
	val int
}

func (idg *IdGenerator) Next() int {
	idg.val += 1
	return idg.val
}

var client goredis.Client

// Proxy all messages to `debug` Channel on the EventSource Server.
// TODO (yml): Delete this function when we are out of the POC
func proxyAllMessages(srv *eventsource.Server, messages <-chan goredis.Message) {
	idx := IdGenerator{}
	for {
		select {
		case msg := <-messages:
			message := &Message{
				Idx:     strconv.Itoa(idx.Next()),
				Channel: msg.Channel,
				Html:    string(msg.Message),
			}
			srv.Publish([]string{"debug"}, message)
		}
	}
}

func main() {
	srv := eventsource.NewServer()
	defer srv.Close()

	psub := make(chan string, 1)
	psub <- "channel_update:*"
	messages := make(chan goredis.Message, 0)

	fmt.Println("Start listening to redis Publication BUS for: `channel_update:*`")
	go client.Subscribe(nil, nil, psub, nil, messages)

	// Start a goroutine that proxy all messages from `channel_update:*` to debug eventsource Channel
	go proxyAllMessages(srv, messages)

	m := martini.Classic()
	// eventsource endpoints
	m.Get("/:channel/eventsource", func(w http.ResponseWriter, req *http.Request, params martini.Params) {
		srv.Handler(params["channel"])(w, req)
	})
	m.Run()

}
