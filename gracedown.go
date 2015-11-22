package gracedown

import (
	"net"
	"net/http"
	"sync"
	"sync/atomic"
)

type Server struct {
	Server *http.Server

	wg       sync.WaitGroup
	mu       sync.Mutex
	closed   int32 // accessed atomically.
	idlePool map[net.Conn]struct{}
}

func NewWithServer(s *http.Server) *Server {
	return &Server{
		Server:    s,
		chanClose: make(chan bool),
		idlePool:  map[net.Conn]struct{}{},
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

func (srv *Server) Serve(l net.Listener) error {
	originalConnState := srv.Server.ConnState
	srv.Server.ConnState = func(conn net.Conn, newState http.ConnState) {
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
		if originalConnState != nil {
			originalConnState(conn, newState)
		}
	}

	err := srv.Server.Serve(l)

	// close all idle connections
	srv.mu.Lock()
	for conn := range srv.idlePool {
		conn.Close()
	}
	srv.mu.Unlock()

	// wait all connections have done
	srv.wg.Wait()

	if atomic.LoadInt32(&srv.closed) != nil {
		// ignore closed network error when srv.Close() is called
		return nil
	}
	return err
}

func (srv *Server) Close() bool {
	if atomic.CompareAndSwapInt32(&srv.closed, 0, 1) {
		srv.Server.SetKeepAlivesEnabled(false)
		l.Close()
		return true
	}
	return false
}
