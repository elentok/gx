// Package pushcapture is the fake push service the fixture points a server's
// stored Web Push endpoints at. Every request becomes one JSON line in a
// file. A control file picks the answer: absent or empty means 201, a status
// number makes requests fail with that status.
package pushcapture

import (
	"encoding/json"
	"io"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Line is one captured request. It never holds the body, only its length.
type Line struct {
	At            time.Time `json:"at"`
	Path          string    `json:"path"`
	TTL           string    `json:"ttl"`
	Urgency       string    `json:"urgency,omitempty"`
	Topic         string    `json:"topic,omitempty"`
	Authorization bool      `json:"authorization"`
	BodyLength    int       `json:"body_length"`
}

// Server is a running fake push service.
type Server struct {
	srv     *http.Server
	url     string
	control string

	mu   sync.Mutex
	file *os.File
}

// Start listens on a loopback port, appends every request to path and reads
// the answer from the control file on each request.
func Start(path, control string) (*Server, error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, err
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		_ = file.Close()
		return nil, err
	}
	s := &Server{file: file, control: control, url: "http://" + ln.Addr().String()}
	s.srv = &http.Server{Handler: http.HandlerFunc(s.handle), ReadHeaderTimeout: 5 * time.Second}
	go func() { _ = s.srv.Serve(ln) }()
	return s, nil
}

// URL is the service's base URL; a subscription endpoint is <URL>/push/<n>.
func (s *Server) URL() string { return s.url }

// Close stops the service and closes the file.
func (s *Server) Close() error {
	err := s.srv.Close()
	s.mu.Lock()
	defer s.mu.Unlock()
	_ = s.file.Close()
	return err
}

func (s *Server) status() int {
	data, err := os.ReadFile(s.control)
	if err != nil {
		return http.StatusCreated
	}
	n, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil || n < 100 || n > 599 {
		return http.StatusCreated
	}
	return n
}

func (s *Server) handle(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	n, _ := io.Copy(io.Discard, r.Body)
	line := Line{
		At: time.Now(), Path: r.URL.Path, TTL: r.Header.Get("TTL"), Urgency: r.Header.Get("Urgency"),
		Topic: r.Header.Get("Topic"), Authorization: r.Header.Get("Authorization") != "", BodyLength: int(n),
	}
	data, _ := json.Marshal(line)
	s.mu.Lock()
	_, err := s.file.Write(append(data, '\n'))
	s.mu.Unlock()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(s.status())
}
