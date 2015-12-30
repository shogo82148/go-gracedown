package gracedown

import (
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"testing"
	"time"
)

func newLocalListener() net.Listener {
	// this code from net/http/httptest
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		if l, err = net.Listen("tcp6", "[::1]:0"); err != nil {
			panic(fmt.Sprintf("httptest: failed to listen on a port: %v", err))
		}
	}
	return l
}

func TestShutdown_NoKeepAlive(t *testing.T) {
	const expectedBody = "test response body"

	// prepare test server
	chHandlerRequest := make(chan *http.Request)
	chHandlerResponse := make(chan []byte)
	handler := http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		chHandlerRequest <- req
		data := <-chHandlerResponse
		w.Write(data)
	})
	ts := NewWithServer(&http.Server{
		Handler: handler,
	})

	// start server
	l := newLocalListener()
	chServe := make(chan error)
	go func() {
		chServe <- ts.Serve(l)
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
			DisableKeepAlives:   true, // keep-alives are DISABLE!!
			MaxIdleConnsPerHost: 1,
		},
	}

	// first request will be success
	chRequest := make(chan []byte)
	go func() {
		t.Log("request 1st GET")
		resp, err := client.Get(url)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
			chRequest <- nil
			return
		}
		data, _ := ioutil.ReadAll(resp.Body)
		t.Logf("response is '%s'", string(data))
		resp.Body.Close()
		chRequest <- data
	}()

	// close server
	<-chHandlerRequest // wait for recieving the request...
	t.Log("call sever.Close()")
	if ts.Close() != true {
		t.Errorf("first call to Close returned false")
	}
	if ts.Close() != false {
		t.Fatal("second call to Close returned true")
	}

	// second request will be failure, because the server starts shutting down process
	_, err := client.Get(url)
	t.Logf("request 2nd GET: %v", err)
	if err == nil {
		t.Errorf("want some error, but not")
	}

	select {
	case <-chServe:
		t.Error("Serve() returned too early")
	default:
	}

	// test the response
	select {
	case <-chRequest:
		t.Error("the response returned too early")
	default:
	}
	chHandlerResponse <- []byte(expectedBody)
	select {
	case data := <-chRequest:
		if string(data) != expectedBody {
			t.Errorf("want %s, got %s", expectedBody, string(data))
		}
	case <-time.After(5 * time.Second):
		t.Errorf("timeout")
	}

	select {
	case err := <-chServe:
		if err != nil {
			t.Errorf("unexpeted error: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Errorf("timeout")
	}
}

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
	}
	resp.Body.Close()

	// start shutting down process
	ts.Close()

	// 2nd request will be success, because this request uses the Keep-Alive connection
	resp, err = client.Get(url)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	resp.Body.Close()

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
