go-gracedown
=====

Package go-gracedown provides a library that makes it easy to build a HTTP server that can be shutdown gracefully (that is, without dropping any connections).

## SYNOPSIS

``` go
import "github.com/shogo82148/go-gracedown"

func main() {
  handler := MyHTTPHandler()
  gracedown.ListenAndServe(":7000", handler)
}
```

## SEE ALSO

- Manners https://github.com/braintree/manners
- httpdown https://github.com/facebookgo/httpdown
