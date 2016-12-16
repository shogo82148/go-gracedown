go-gracedown
=====

Package go-gracedown provides a library that makes it easy to build a HTTP server that can be shutdown gracefully (that is, without dropping any connections).

## SYNOPSIS

``` go
import (
  "os"
  "os/signal"

  "github.com/shogo82148/go-gracedown"
)

func main() {
  go func() {
    for {
      s := <-signal_chan
        if s == syscall.SIGTERM {
          gracedown.Close() // trigger graceful shutdown
        }
    }
  }()

  handler := MyHTTPHandler()
  gracedown.ListenAndServe(":7000", handler)
}
```

## built-in graceful shutdown support (from Go version 1.8 onward)

From Go version 1.8 onward, the HTTP Server has support for graceful shutdown.
([HTTP Server Graceful Shutdown](https://beta.golang.org/doc/go1.8#http_shutdown))
The go-gracedown package is just a wrapper of the net/http package to maintain interface compatibility.
So you should use the ["net/http".Server.Shutdown](https://golang.org/pkg/net/http/#Server.Shutdown) method
and ["net/http".Server.Close](https://golang.org/pkg/net/http/#Server.Close) method directly.


### Changes

- Go version 1.7 or less: The grace.Server.Close method keeps currently-open connections.
- From Go version 1.8 onward: The grace.Server.Close method drops currently-open connections.


## GODOC

See [godoc](https://godoc.org/github.com/shogo82148/go-gracedown) for more information.

## SEE ALSO

- [braintree/manners](https://github.com/braintree/manners)
- [facebookgo](https://github.com/facebookgo/httpdown)
- [HTTP Server Graceful Shutdown](https://beta.golang.org/doc/go1.8#http_shutdown)