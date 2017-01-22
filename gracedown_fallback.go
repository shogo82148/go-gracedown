// +build !go1.8

package gracedown

import (
	"crypto/tls"
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

	wg                sync.WaitGroup
	mu                sync.Mutex
	originalConnState func(conn net.Conn, newState http.ConnState)
	connStateOnce     sync.Once
	closed            int32 // accessed atomically.
	idlePool          map[net.Conn]struct{}
	listeners         map[net.Listener]struct{}
}

// NewWithServer wraps an existing http.Server.
func NewWithServer(s *http.Server) *Server {
	return &Server{
		Server:      s,
		KillTimeOut: 10 * time.Second,
		idlePool:    map[net.Conn]struct{}{},
		listeners:   map[net.Listener]struct{}{},
	}
}

// ListenAndServe provides a graceful equivalent of net/http.Server.ListenAndServe
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

// ListenAndServeTLS provides a graceful equivalent of net/http.Server.ListenAndServeTLS
func (srv *Server) ListenAndServeTLS(certFile, keyFile string) error {
	// direct lift from net/http/server.go
	addr := srv.Addr
	if addr == "" {
		addr = ":https"
	}

	config := cloneTLSConfig(srv.TLSConfig)
	if !strSliceContains(config.NextProtos, "http/1.1") {
		config.NextProtos = append(config.NextProtos, "http/1.1")
	}

	configHasCert := len(config.Certificates) > 0 || config.GetCertificate != nil
	if !configHasCert || certFile != "" || keyFile != "" {
		var err error
		config.Certificates = make([]tls.Certificate, 1)
		config.Certificates[0], err = tls.LoadX509KeyPair(certFile, keyFile)
		if err != nil {
			return err
		}
	}

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}

	return srv.Serve(tls.NewListener(ln, config))
}

// cloneTLSConfig returns a shallow clone of the exported
// fields of cfg, ignoring the unexported sync.Once, which
// contains a mutex and must not be copied.
//
// The cfg must not be in active use by tls.Server, or else
// there can still be a race with tls.Server updating SessionTicketKey
// and our copying it, and also a race with the server setting
// SessionTicketsDisabled=false on failure to set the random
// ticket key.
//
// If cfg is nil, a new zero tls.Config is returned.
//
// Direct lift from net/http/transport.go
func cloneTLSConfig(cfg *tls.Config) *tls.Config {
	if cfg == nil {
		return &tls.Config{}
	}
	return &tls.Config{
		Rand:                     cfg.Rand,
		Time:                     cfg.Time,
		Certificates:             cfg.Certificates,
		NameToCertificate:        cfg.NameToCertificate,
		GetCertificate:           cfg.GetCertificate,
		RootCAs:                  cfg.RootCAs,
		NextProtos:               cfg.NextProtos,
		ServerName:               cfg.ServerName,
		ClientAuth:               cfg.ClientAuth,
		ClientCAs:                cfg.ClientCAs,
		InsecureSkipVerify:       cfg.InsecureSkipVerify,
		CipherSuites:             cfg.CipherSuites,
		PreferServerCipherSuites: cfg.PreferServerCipherSuites,
		SessionTicketsDisabled:   cfg.SessionTicketsDisabled,
		SessionTicketKey:         cfg.SessionTicketKey,
		ClientSessionCache:       cfg.ClientSessionCache,
		MinVersion:               cfg.MinVersion,
		MaxVersion:               cfg.MaxVersion,
		CurvePreferences:         cfg.CurvePreferences,
	}
}

func strSliceContains(ss []string, s string) bool {
	for _, v := range ss {
		if v == s {
			return true
		}
	}
	return false
}

// Serve provides a graceful equivalent of net/http.Server.Serve
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

// Close shuts down the default server used by ListenAndServe, ListenAndServeTLS and
// Serve. It returns true if it's the first time Close is called.
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
