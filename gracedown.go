package gracedown

import (
	"net"
	"net/http"
)

type Server struct {
	Server *http.Server

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
		l.Close()
	}()
	return srv.Server.Serve(l)
}

func (srv *Server) Close() bool {
	ret := <-srv.chanClose
	return ret
}
