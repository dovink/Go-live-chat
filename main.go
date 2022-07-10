package main

import (
	"encoding/json"
	"fmt"
	"github.com/gorilla/mux"
	"io"
	"log"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

type Channel struct {
	Notifier chan []byte

	newClients chan chan []byte

	closingClients chan chan []byte

	ClosingOne chan chan []byte

	clients map[chan []byte]bool
}

func NewChannel() (chanel *Channel) {

	chanel = &Channel{
		Notifier:       make(chan []byte, 1),
		newClients:     make(chan chan []byte),
		closingClients: make(chan chan []byte),
		ClosingOne:     make(chan chan []byte),
		clients:        make(map[chan []byte]bool),
	}

	go chanel.listen()

	return
}

var mapMutex = sync.Mutex{}

var i int64 = 0

var id int64

var event string

var server = NewChannel()

var chans [10]*Channel

var url [10]string

var chanid int64 = 0

type Message struct {
	ID      int64  `json:"id"`
	Event   string `json:"event"`
	Message string `json:"msg"`
}

func contains(s [10]string, str string) bool {
	for _, v := range s {
		if v == str {
			return true
		}
	}

	return false
}

func (channel *Channel) listen() {
	for {
		select {
		case s := <-channel.newClients:
			channel.clients[s] = true
			log.Printf("Client added. %d registered clients", len(channel.clients))
		case <-channel.closingClients:
			for close := range channel.clients {
				delete(channel.clients, close)
				log.Printf("Removed client. %d registered clients", len(channel.clients))
			}

		case s := <-channel.ClosingOne:
			delete(channel.clients, s)
			log.Printf("Removed client. %d registered clients", len(channel.clients))

		case event := <-channel.Notifier:
			for clientMessageChan := range channel.clients {
				clientMessageChan <- event
			}
		}
	}

}

func handle(w http.ResponseWriter, r *http.Request) {
	mapMutex.Lock()
	vars := mux.Vars(r)
	param := vars["topic"]

	for i = 0; i <= chanid; i++ {
		if url[i] == param {
			server = chans[i]
			break
		}
	}
	if !contains(url, param) {
		atomic.AddInt64(&chanid, 1)
		url[chanid] = param
		chans[chanid] = NewChannel()
		server = chans[chanid]
	}
	mapMutex.Unlock()
	if r.Method == http.MethodPost {

		w.WriteHeader(http.StatusNoContent)

		var msg Message

		err := json.NewDecoder(r.Body).Decode(&msg)

		if err != nil {
			http.Error(w, "err", http.StatusInternalServerError)
		}

		atomic.AddInt64(&id, 1)
		msg.ID = id
		event = "msg"
		msg.Event = event
		j, err := json.Marshal(msg)
		if err != nil {
			http.Error(w, "err", http.StatusInternalServerError)
		}

		mapMutex.Lock()
		server.Notifier <- j
		mapMutex.Unlock()

		err = json.NewEncoder(w).Encode(msg)
		if err != nil {
			io.EOF.Error()

		}
	}
	if r.Method == http.MethodGet {

		flusher, err := w.(http.Flusher)

		if !err {
			http.Error(w, "Bad stream", http.StatusInternalServerError)
		}

		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Connection", "keep-alive")

		messageChan := make(chan []byte)

		mapMutex.Lock()
		server.newClients <- messageChan
		mapMutex.Unlock()

		q := make(chan bool)

		const interval = 30 * time.Second

		go func() {

			mapMutex.Lock()
			currentChannel := server
			mapMutex.Unlock()

			time.Sleep(interval)

			var msg Message
			msg.ID = atomic.LoadInt64(&id)
			msg.Event = "timeout"
			msg.Message = "30"

			j, err := json.Marshal(msg)
			if err != nil {
				http.Error(w, "err", http.StatusInternalServerError)
			}

			currentChannel.Notifier <- j

			currentChannel.closingClients <- messageChan

			q <- true

		}()

		for {
			select {
			case message := <-messageChan:
				_, err := fmt.Fprintf(w, "%s\n", message)
				if err != nil {
					http.Error(w, "err", http.StatusInternalServerError)
				}
				flusher.Flush()

			case <-q:
				return
			}
		}
	}
}

func main() {
	router := mux.NewRouter()
	router.HandleFunc("/infocenter/{topic}", handle).Methods(http.MethodGet, http.MethodPost)
	log.Fatal(http.ListenAndServe(":8080", router))
}
