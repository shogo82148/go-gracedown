package gracedown

import (
	"crypto/tls"
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

type Server struct {
	*http.Server

	KillTimeOut time.Duration

	wg                sync.WaitGroup
	mu                sync.Mutex
	originalConnState func(conn net.Conn, newState http.ConnState)
	connStateOnce     sync.Once
	closed            int32 // accessed atomically.
	idlePool          map[net.Conn]struct{}
	listeners         map[net.Listener]struct{}
}

func NewWithServer(s *http.Server) *Server {
	return &Server{
		Server:      s,
		KillTimeOut: 10 * time.Second,
		idlePool:    map[net.Conn]struct{}{},
		listeners:   map[net.Listener]struct{}{},
	}
}

func (srv *Server) ListenAndServe() error {
	addr := srv.Server.Addr
	if addr == "" {
		addr = ":http"
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	return srv.Serve(ln)
}

// ListenAndServeTLS provides a graceful equivalent of net/http.Serve.ListenAndServeTLS
func (srv *Server) ListenAndServeTLS(certFile, keyFile string) error {
	// direct lift from net/http/server.go
	addr := srv.Addr
	if addr == "" {
		addr = ":https"
	}
	config := &tls.Config{}
	if srv.TLSConfig != nil {
		*config = *srv.TLSConfig
	}
	if config.NextProtos == nil {
		config.NextProtos = []string{"http/1.1"}
	}

	var err error
	config.Certificates = make([]tls.Certificate, 1)
	config.Certificates[0], err = tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return err
	}

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}

	return srv.Serve(tls.NewListener(ln, config))
}

func (srv *Server) Serve(l net.Listener) error {
	// remember net.Listener
	srv.mu.Lock()
	srv.listeners[l] = struct{}{}
	srv.mu.Unlock()
	defer func() {
		srv.mu.Lock()
		delete(srv.listeners, l)
		srv.mu.Unlock()
	}()

	// replace ConnState
	srv.connStateOnce.Do(func() {
		srv.originalConnState = srv.Server.ConnState
		srv.Server.ConnState = srv.connState
	})

	err := srv.Server.Serve(l)

	go func() {
		// wait for closing keep-alive connection by sending `Connection: Close` header.
		time.Sleep(srv.KillTimeOut)

		// time out, close all idle connections
		srv.mu.Lock()
		for conn := range srv.idlePool {
			conn.Close()
		}
		srv.mu.Unlock()
	}()

	// wait all connections have done
	srv.wg.Wait()

	if atomic.LoadInt32(&srv.closed) != 0 {
		// ignore closed network error when srv.Close() is called
		return nil
	}
	return err
}

func (srv *Server) Close() bool {
	if atomic.CompareAndSwapInt32(&srv.closed, 0, 1) {
		srv.Server.SetKeepAlivesEnabled(false)
		srv.mu.Lock()
		listeners := srv.listeners
		srv.listeners = map[net.Listener]struct{}{}
		srv.mu.Unlock()
		for l := range listeners {
			l.Close()
		}
		return true
	}
	return false
}

func (srv *Server) connState(conn net.Conn, newState http.ConnState) {
	srv.mu.Lock()
	switch newState {
	case http.StateNew:
		srv.wg.Add(1)
	case http.StateActive:
		delete(srv.idlePool, conn)
	case http.StateIdle:
		srv.idlePool[conn] = struct{}{}
	case http.StateClosed, http.StateHijacked:
		delete(srv.idlePool, conn)
		srv.wg.Done()
	}
	srv.mu.Unlock()
	if srv.originalConnState != nil {
		srv.originalConnState(conn, newState)
	}
}
