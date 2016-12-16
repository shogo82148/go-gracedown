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
	<-chHandlerRequest // wait for receiving the request...
	t.Log("call sever.Close()")
	if ts.Close() != true {
		t.Errorf("first call to Close returned false")
	}
	if ts.Close() != false {
		t.Fatal("second call to Close returned true")
	}
	time.Sleep(1 * time.Second) // make sure closing the test server has started

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
