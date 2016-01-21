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

## GODOC

See [godoc](https://godoc.org/github.com/shogo82148/go-gracedown) for more information.

## SEE ALSO

- Manners https://github.com/braintree/manners
- httpdown https://github.com/facebookgo/httpdown
