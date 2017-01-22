// +build go1.8

package gracedown

import (
	"context"
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

// Server provides a graceful equivalent of net/http.Server.
type Server struct {
	*http.Server

	KillTimeOut time.Duration

	mu       sync.Mutex
	closed   int32 // accessed atomically.
	doneChan chan struct{}
}

// NewWithServer wraps an existing http.Server.
func NewWithServer(s *http.Server) *Server {
	return &Server{
		Server:      s,
		KillTimeOut: 10 * time.Second,
	}
}

func (srv *Server) Serve(l net.Listener) error {
	err := srv.Server.Serve(l)

	// Wait for closing all connections.
	if err == http.ErrServerClosed && atomic.LoadInt32(&srv.closed) != 0 {
		ch := srv.getDoneChan()
		<-ch
		return nil
	}
	return err
}

func (srv *Server) getDoneChan() <-chan struct{} {
	srv.mu.Lock()
	defer srv.mu.Unlock()
	return srv.getDoneChanLocked()
}

func (srv *Server) getDoneChanLocked() chan struct{} {
	if srv.doneChan == nil {
		srv.doneChan = make(chan struct{})
	}
	return srv.doneChan
}

func (srv *Server) closeDoneChanLocked() {
	ch := srv.getDoneChanLocked()
	select {
	case <-ch:
		// Already closed. Don't close again.
	default:
		// Safe to close here. We're the only closer, guarded
		// by s.mu.
		close(ch)
	}
}

// Close shuts down the default server used by ListenAndServe, ListenAndServeTLS and
// Serve. It returns true if it's the first time Close is called.
func (srv *Server) Close() bool {
	if !atomic.CompareAndSwapInt32(&srv.closed, 0, 1) {
		return false
	}

	// immediately closes all connection.
	if srv.KillTimeOut == 0 {
		srv.Server.Close()

		srv.mu.Lock()
		defer srv.mu.Unlock()
		srv.closeDoneChanLocked()
		return true
	}

	// graceful shutdown
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), srv.KillTimeOut)
		defer cancel()
		srv.Shutdown(ctx)

		srv.mu.Lock()
		defer srv.mu.Unlock()
		srv.closeDoneChanLocked()
	}()

	return true
}
