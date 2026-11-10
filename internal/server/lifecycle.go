package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"
)

// Run binds every port before serving. Any bind or serve failure stops the host;
// cancellation drains both listeners before their database pools are closed.
func Run(ctx context.Context, timeout time.Duration, logger *slog.Logger, servers ...*http.Server) error {
	listeners := make([]net.Listener, 0, len(servers))
	defer func() {
		for _, listener := range listeners {
			_ = listener.Close()
		}
		for _, server := range servers {
			_ = server.Close()
		}
	}()
	for _, server := range servers {
		listener, err := net.Listen("tcp", server.Addr)
		if err != nil {
			return fmt.Errorf("listen %s: %w", server.Addr, err)
		}
		listeners = append(listeners, listener)
	}
	stopped := make(chan error, len(servers))
	for i, server := range servers {
		logger.Info("http listener ready", "addr", listeners[i].Addr().String())
		go func(server *http.Server, listener net.Listener) {
			err := server.Serve(listener)
			if err != nil && !errors.Is(err, http.ErrServerClosed) {
				err = fmt.Errorf("serve %s: %w", server.Addr, err)
			} else {
				err = nil
			}
			stopped <- err
		}(server, listeners[i])
	}
	var runErr error
	select {
	case runErr = <-stopped:
	case <-ctx.Done():
	}
	shutdown, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	shutdownErrors := make(chan error, len(servers))
	for _, server := range servers {
		go func(server *http.Server) { shutdownErrors <- server.Shutdown(shutdown) }(server)
	}
	for range servers {
		runErr = errors.Join(runErr, <-shutdownErrors)
	}
	return runErr
}
