package server

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net"
	"net/http"
	"testing"
	"time"
)

type listenerLog struct{ ready chan string }

func (l listenerLog) Write(p []byte) (int, error) {
	var entry struct {
		Addr string `json:"addr"`
	}
	if err := json.Unmarshal(p, &entry); err != nil {
		return 0, err
	}
	if entry.Addr != "" {
		l.ready <- entry.Addr
	}
	return len(p), nil
}

func TestRunServesAndShutsDownBothListeners(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	logs := listenerLog{ready: make(chan string, 2)}
	done := make(chan error, 1)
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) })
	go func() {
		done <- Run(ctx, time.Second, slog.New(slog.NewJSONHandler(logs, nil)),
			&http.Server{Addr: "127.0.0.1:0", Handler: handler},
			&http.Server{Addr: "127.0.0.1:0", Handler: handler})
	}()
	var addresses []string
	client := &http.Client{Timeout: time.Second}
	for range 2 {
		select {
		case addr := <-logs.ready:
			addresses = append(addresses, addr)
			response, err := client.Get("http://" + addr + "/")
			if err != nil {
				t.Fatal(err)
			}
			response.Body.Close()
			if response.StatusCode != 204 {
				t.Fatalf("status=%d", response.StatusCode)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("listeners did not start")
		}
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("listeners did not stop")
	}
	for _, addr := range addresses {
		listener, err := net.Listen("tcp", addr)
		if err != nil {
			t.Fatalf("listener was not released: %v", err)
		}
		listener.Close()
	}
}

func TestBindFailureReleasesEarlierListener(t *testing.T) {
	first, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := first.Addr().String()
	first.Close()
	occupied, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer occupied.Close()
	err = Run(context.Background(), time.Second, slog.New(slog.NewTextHandler(io.Discard, nil)),
		&http.Server{Addr: addr}, &http.Server{Addr: occupied.Addr().String()})
	if err == nil {
		t.Fatal("expected bind failure")
	}
	rebound, err := net.Listen("tcp", addr)
	if err != nil {
		t.Fatalf("first listener leaked after second bind failure: %v", err)
	}
	rebound.Close()
}
