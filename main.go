package main

import (
	"io"
	"log"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// Upgrade configuration
var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		// Allow cross-origin requests
		return true
	},
}

var (
	clients = make(map[*websocket.Conn]bool) // Map to store all active clients
	mu      sync.Mutex                       // Mutex to ensure thread safety
)

// handleConnections handles incoming websocket connections.
func handleConnections(w http.ResponseWriter, r *http.Request) {
	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("error upgrading to websocket: %v", err)
		return
	}
	defer ws.Close()

	mu.Lock()
	clients[ws] = true
	mu.Unlock()

	// Keep the connection open
	for {
		if _, _, err := ws.ReadMessage(); err != nil {
			mu.Lock()
			delete(clients, ws)
			mu.Unlock()
			ws.Close()
			break
		}
	}
}

// broadcastMessages reads from the lines channel and sends to all clients.
func broadcastMessages(lines <-chan string) {
	for {
		msg := <-lines
		mu.Lock()
		for client := range clients {
			err := client.WriteMessage(websocket.TextMessage, []byte(msg))
			if err != nil {
				client.Close()
				delete(clients, client)
			}
		}
		mu.Unlock()
	}
}

// tailFile watches the given file for changes and sends new lines to the lines channel.
func tailFile(filePath string, lines chan<- string) {
	file, err := os.Open(filePath)
	if err != nil {
		log.Fatalf("failed to open file: %v", err)
	}
	defer file.Close()

	fi, err := file.Stat()
	if err != nil {
		log.Fatalf("failed to get file stats: %v", err)
	}

	// Start reading from end of file
	file.Seek(0, io.SeekEnd)
	offset := fi.Size()

	for {
		fi, err := file.Stat()
		if err != nil {
			log.Fatalf("failed to get file stats: %v", err)
		}

		if fi.Size() > offset {
			// Seek to the last position
			file.Seek(offset, io.SeekStart)
			buf := make([]byte, fi.Size()-offset)
			_, err := file.Read(buf)
			if err != nil && err != io.EOF {
				log.Fatalf("failed to read file: %v", err)
			}

			lines <- string(buf)
			offset = fi.Size()
		}

		time.Sleep(1 * time.Second)
	}
}

// main function to start the server and initialize goroutines.
func main() {
	lines := make(chan string)

	go tailFile("/test.log", lines) // Start file tailing in a goroutine
	go broadcastMessages(lines)     // Start broadcasting messages in a goroutine

	http.HandleFunc("/ws", handleConnections) // Websocket endpoint

	log.Println("Server started on :8080")
	err := http.ListenAndServe(":8080", nil) // Start HTTP server
	if err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}
