// +build !go1.8

package gracedown

import (
	"net"
	"net/http"
	"testing"
	"time"
)

func TestShutdown_KeepAlive(t *testing.T) {
	// prepare test server
	handler := http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
	})
	ts := NewWithServer(&http.Server{
		Handler: handler,
	})

	// start server
	l := newLocalListener()
	go func() {
		ts.Serve(l)
	}()
	url := "http://" + l.Addr().String()

	client := &http.Client{
		Transport: &http.Transport{
			Proxy: http.ProxyFromEnvironment,
			Dial: (&net.Dialer{
				Timeout:   5 * time.Second,
				KeepAlive: 5 * time.Second,
			}).Dial,
			TLSHandshakeTimeout: 10 * time.Second,
			DisableKeepAlives:   false, // keep-alives are ENABLE!!
			MaxIdleConnsPerHost: 1,
		},
	}

	// 1st request will be success
	resp, err := client.Get(url)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	} else {
		resp.Body.Close()
	}

	// start shutting down process
	ts.Close()
	time.Sleep(1 * time.Second) // make sure closing the test server has started

	// 2nd request will be success, because this request uses the Keep-Alive connection
	resp, err = client.Get(url)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	} else {
		resp.Body.Close()
	}

	// 3rd request will be failure, because the Keep-Alive connection is closed
	resp, err = client.Get(url)
	if err == nil {
		t.Error("want error, but not")
	}
}

func TestShutdown_KillKeepAlive(t *testing.T) {
	// prepare test server
	handler := http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
	})
	ts := NewWithServer(&http.Server{
		Handler: handler,
	})
	ts.KillTimeOut = time.Second // force close after a second

	// start server
	done := make(chan error, 1)
	l := newLocalListener()
	go func() {
		done <- ts.Serve(l)
	}()
	url := "http://" + l.Addr().String()

	client := &http.Client{
		Transport: &http.Transport{
			Proxy: http.ProxyFromEnvironment,
			Dial: (&net.Dialer{
				Timeout:   5 * time.Second,
				KeepAlive: 5 * time.Second,
			}).Dial,
			TLSHandshakeTimeout: 10 * time.Second,
			DisableKeepAlives:   false, // keep-alives are ENABLE!!
			MaxIdleConnsPerHost: 1,
		},
	}

	// 1st request will be success
	resp, err := client.Get(url)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	resp.Body.Close()

	// start shutting down process
	start := time.Now()
	ts.Close()
	time.Sleep(1 * time.Second) // make sure closing the test server has started

	select {
	case err := <-done:
		end := time.Now()
		dt := end.Sub(start)
		t.Logf("kill timeout: %v", dt)
		if dt < ts.KillTimeOut {
			t.Errorf("too fast kill timeout")
		}
		if err != nil {
			t.Errorf("unexpected err: %v", err)
		}
	case <-time.After(ts.KillTimeOut + 5*time.Second):
		t.Errorf("timeout")
	}

	// 2nd request will be failure, because the server has already shut down
	resp, err = client.Get(url)
	if err == nil {
		t.Error("want error, but not")
	}
}
