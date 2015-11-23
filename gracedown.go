package gracedown

import (
	"net"
	"net/http"
	"sync"
	"sync/atomic"
)

type Server struct {
	Server *http.Server

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
		Server:    s,
		idlePool:  map[net.Conn]struct{}{},
		listeners: map[net.Listener]struct{}{},
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

	// close all idle connections
	srv.mu.Lock()
	for conn := range srv.idlePool {
		conn.Close()
	}
	srv.mu.Unlock()

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
		originalConnState(conn, newState)
	}
}
