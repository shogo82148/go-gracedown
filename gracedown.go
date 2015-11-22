package gracedown

import (
	"net"
	"net/http"
)

type Server struct {
	Server *http.Server
}

func NewWithServer(s *http.Server) *Server {
	return &Server{
		Server: s,
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
	return srv.Server.Serve(l)
}
