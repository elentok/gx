package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elentok/gx/apiclient"
	"github.com/elentok/gx/server"
)

// dupServer refuses every one-off as a live duplicate of gx:p/1, which its
// snapshot reports with the given status.
func dupServer(t *testing.T, status string) *apiclient.Client {
	t.Helper()
	sock := filepath.Join(shortTempDir(t), "a.sock")
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/oneoff", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(server.OneOffResult{Refused: true, Reason: server.ReasonDuplicateLive, Address: "gx:p/1", Message: "dup"})
	})
	mux.HandleFunc("/v1/snapshot", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(server.Snapshot{Tickets: []server.TicketInfo{{Address: "gx:p/1", Status: status}}})
	})
	srv := &http.Server{Handler: mux}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { _ = srv.Close() })
	return apiclient.New(sock)
}

func TestServerOneOff_DuplicateWaitFollowsExistingTicket(t *testing.T) {
	cl := dupServer(t, "needs-answer")
	var out, errOut bytes.Buffer
	err := runServerOneOff(context.Background(), cl, &out, &errOut, false, true, 0, server.OneOffRequest{Prompt: "x"})
	var exitErr *ExitError
	if !errors.As(err, &exitErr) || exitErr.Code != 3 {
		t.Fatalf("err = %v; want exit code 3 from the existing ticket", err)
	}
	if !strings.Contains(errOut.String(), "duplicate") || !strings.Contains(out.String(), "gx:p/1") {
		t.Errorf("stdout = %q, stderr = %q; want the address and a duplicate notice", out.String(), errOut.String())
	}
}

func TestServerOneOff_DuplicateWithoutWaitStillExitsSeven(t *testing.T) {
	cl := dupServer(t, "claimed")
	var out, errOut bytes.Buffer
	err := runServerOneOff(context.Background(), cl, &out, &errOut, false, false, 0, server.OneOffRequest{Prompt: "x"})
	var exitErr *ExitError
	if !errors.As(err, &exitErr) || exitErr.Code != exitDuplicateLive {
		t.Fatalf("err = %v; want exit code 7", err)
	}
}

func TestServerOneOff_JSONDuplicateWithoutWaitExitsSeven(t *testing.T) {
	cl := dupServer(t, "claimed")
	var out, errOut bytes.Buffer
	err := runServerOneOff(context.Background(), cl, &out, &errOut, true, false, 0, server.OneOffRequest{Prompt: "x"})
	var exitErr *ExitError
	if !errors.As(err, &exitErr) || exitErr.Code != exitDuplicateLive {
		t.Fatalf("err = %v; want exit code 7", err)
	}
	if !json.Valid(out.Bytes()) {
		t.Errorf("stdout = %q; want pure JSON", out.String())
	}
}

func TestServerOneOff_JSONDuplicateWaitFollowsExistingTicket(t *testing.T) {
	cl := dupServer(t, "needs-answer")
	var out, errOut bytes.Buffer
	err := runServerOneOff(context.Background(), cl, &out, &errOut, true, true, 0, server.OneOffRequest{Prompt: "x"})
	var exitErr *ExitError
	if !errors.As(err, &exitErr) || exitErr.Code != 3 {
		t.Fatalf("err = %v; want exit code 3 from the existing ticket", err)
	}
	if !json.Valid(out.Bytes()) {
		t.Errorf("stdout = %q; want pure JSON", out.String())
	}
}

func TestServerOneOff_JSONOtherRefusalFails(t *testing.T) {
	sock := filepath.Join(shortTempDir(t), "a.sock")
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/oneoff", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(server.OneOffResult{Refused: true, Reason: "bad-agent", Message: "no such agent"})
	})
	srv := &http.Server{Handler: mux}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { _ = srv.Close() })
	var out, errOut bytes.Buffer
	err = runServerOneOff(context.Background(), apiclient.New(sock), &out, &errOut, true, false, 0, server.OneOffRequest{Prompt: "x"})
	if err == nil {
		t.Fatal("err = nil; want a refusal failure")
	}
	if !json.Valid(out.Bytes()) {
		t.Errorf("stdout = %q; want pure JSON", out.String())
	}
}

func TestServerOneOff_ExitsEightWhenNoServerRunning(t *testing.T) {
	cl := apiclient.New(filepath.Join(shortTempDir(t), "a.sock"))
	var out, errOut bytes.Buffer
	err := runServerOneOff(context.Background(), cl, &out, &errOut, false, false, 0, server.OneOffRequest{Prompt: "x"})
	var exitErr *ExitError
	if !errors.As(err, &exitErr) || exitErr.Code != 8 {
		t.Fatalf("err = %v; want exit code 8", err)
	}
	if !strings.Contains(errOut.String(), "gx server start") {
		t.Errorf("stderr = %q; want the start hint", errOut.String())
	}
}
