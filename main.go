package main

import (
	"encoding/json"
	"fmt"
	"github.com/gorilla/mux"
	"log"
	"net/http"
	"time"
)

type Channel struct {
	Notifier chan []byte

	newClients chan chan []byte

	closingClients chan chan []byte

	clients map[chan []byte]bool
}

func NewChannel() (chanel *Channel) {

	chanel = &Channel{
		Notifier:       make(chan []byte, 1),
		newClients:     make(chan chan []byte),
		closingClients: make(chan chan []byte),
		clients:        make(map[chan []byte]bool),
	}

	go chanel.listen()
	return
}

var id int
var event string

var server = NewChannel()

var chans [10]*Channel

var url [10]string
var chanid int

type Message struct {
	ID      int    `json:"id"`
	Event   string `json:"event"`
	Message string `json:"msg"`
}

func (channel *Channel) listen() {
	for {
		select {
		case s := <-channel.newClients:

			channel.clients[s] = true
			log.Printf("Client added. %d registered clients", len(channel.clients))
		case s := <-channel.closingClients:

			delete(channel.clients, s)
			log.Printf("Removed client. %d registered clients", len(channel.clients))

		case event := <-channel.Notifier:

			for clientMessageChan, _ := range channel.clients {
				clientMessageChan <- event
			}
		}
	}

}

func handle(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	param := vars["topic"]

	for i := 0; i <= chanid+1; i++ {
		if url[i] != param {
			url[i] = param
			chans[i] = NewChannel()
			server = chans[i]
			chanid++
			break
		}
		if url[i] == param {
			server = chans[i]
			break

		}
	}
	if r.Method == http.MethodPost {
		var msg Message

		_ = json.NewDecoder(r.Body).Decode(&msg)

		w.WriteHeader(http.StatusNoContent)

		id++
		msg.ID = id
		event = "msg"
		msg.Event = event

		j, _ := json.Marshal(msg)
		server.Notifier <- []byte(j)
		json.NewEncoder(w).Encode(msg)
	}
	if r.Method == http.MethodGet {
		flusher, err := w.(http.Flusher)

		if !err {
			http.Error(w, "Bad stream", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Connection", "keep-alive")

		messageChan := make(chan []byte)

		server.newClients <- messageChan

		defer func() {
			server.closingClients <- messageChan
		}()

		const interval = 30 * time.Second
		go func() {
			time.Sleep(interval)
			var msg Message
			msg.ID = id
			msg.Event = "timeout"
			msg.Message = "30"
			j, _ := json.Marshal(msg)
			server.Notifier <- []byte(j)
			server.closingClients <- messageChan
		}()

		notify := w.(http.CloseNotifier).CloseNotify()

		go func() {
			<-notify
			server.closingClients <- messageChan

		}()

		for {

			fmt.Fprintf(w, "data: %s\n\n", <-messageChan)

			flusher.Flush()
		}
	}
}

func main() {
	router := mux.NewRouter()
	router.HandleFunc("/infocenter/{topic}", handle).Methods(http.MethodGet, http.MethodPost)
	log.Fatal(http.ListenAndServe(":8000", router))
}
