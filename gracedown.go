package gracedown

import (
	"net"
	"net/http"
	"sync"
)

type Server struct {
	Server *http.Server

	wg        sync.WaitGroup
	chanClose chan bool
}

func NewWithServer(s *http.Server) *Server {
	return &Server{
		Server:    s,
		chanClose: make(chan bool),
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
	go func() {
		srv.chanClose <- true
		close(srv.chanClose)
		s.Server.SetKeepAlivesEnabled(false)
		l.Close()
	}()

	originalConnState := srv.Server.ConnState
	srv.ConnState = func(conn net.Conn, newState http.ConnState) {
		switch newState {
		case http.StateNew:
			srv.wg.Add(1)
		case http.StateActive:
		case http.StateIdle:
		case http.StateHijacked:
		case http.StateClosed:
			srv.wg.Done()
		}
		if originalConnState != nil {
			originalConnState(conn, newState)
		}
	}

	err := srv.Server.Serve(l)
	srv.wg.Wait()
	return err
}

func (srv *Server) Close() bool {
	ret := <-srv.chanClose
	return ret
}
