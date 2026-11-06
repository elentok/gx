// Package chatcapture is the HTTP endpoint the chat-capture fixture points a
// server's Slack webhook and Telegram Bot API base URL at. Every post becomes
// one JSON line in a file, for a verify step or a Go test to read.
package chatcapture

import (
	"encoding/json"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

// Line is one captured post. ParseMode is set for Telegram only.
type Line struct {
	Transport string    `json:"transport"`
	At        time.Time `json:"at"`
	Text      string    `json:"text"`
	ParseMode string    `json:"parse_mode,omitempty"`
}

// Server is a running capture endpoint.
type Server struct {
	srv *http.Server
	url string

	mu   sync.Mutex
	file *os.File
}

// Start listens on a loopback port and appends every post to path.
func Start(path string) (*Server, error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, err
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		_ = file.Close()
		return nil, err
	}
	s := &Server{file: file, url: "http://" + ln.Addr().String()}
	s.srv = &http.Server{Handler: http.HandlerFunc(s.handle), ReadHeaderTimeout: 5 * time.Second}
	go func() { _ = s.srv.Serve(ln) }()
	return s, nil
}

// SlackURL is the webhook URL to configure.
func (s *Server) SlackURL() string { return s.url + "/slack" }

// TelegramBaseURL is the Bot API base URL; the client posts to <base>/bot<token>/sendMessage.
func (s *Server) TelegramBaseURL() string { return s.url }

// Close stops the endpoint and closes the file.
func (s *Server) Close() error {
	err := s.srv.Close()
	s.mu.Lock()
	defer s.mu.Unlock()
	_ = s.file.Close()
	return err
}

func (s *Server) handle(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Text      string `json:"text"`
		ParseMode string `json:"parse_mode"`
	}
	if r.Method != http.MethodPost || json.NewDecoder(r.Body).Decode(&body) != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	line := Line{Transport: "slack", At: time.Now(), Text: body.Text}
	if strings.HasSuffix(r.URL.Path, "/sendMessage") {
		line.Transport, line.ParseMode = "telegram", body.ParseMode
	}
	data, _ := json.Marshal(line)
	s.mu.Lock()
	_, err := s.file.Write(append(data, '\n'))
	s.mu.Unlock()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if line.Transport == "telegram" {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":1}}`))
		return
	}
	_, _ = w.Write([]byte("ok"))
}
