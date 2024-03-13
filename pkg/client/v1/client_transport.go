package v1

import "net/http"

// Custom transport is a struct that holds custom round trippers and any associated info
type CustomTransport struct {
	CustomRoundTripper http.RoundTripper
	IsTlsEnabled       bool
}

func (ct CustomTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return ct.CustomRoundTripper.RoundTrip(req)
}

// internalRoundTripper is a holder function to make the process of
// creating middleware a bit easier without requiring to
// implement the RoundTripper interface.
type internalRoundTripper func(*http.Request) (*http.Response, error)

func (rt internalRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return rt(req)
}

// Middleware is our middleware creation functionality.
type Middleware func(http.RoundTripper) http.RoundTripper

// Chain is a handy function to wrap a base RoundTripper (optional)
// with the middlewares.
func Chain(rt http.RoundTripper, middlewares ...Middleware) http.RoundTripper {
	if rt == nil {
		rt = http.DefaultTransport
	}

	for _, m := range middlewares {
		rt = m(rt)
	}

	return rt
}

// cloneRequest returns a clone of the provided *http.Request. The clone is a
// shallow copy of the struct and its Header map.
func cloneRequest(r *http.Request) *http.Request {
	// shallow copy of the struct
	r2 := new(http.Request)
	*r2 = *r
	// deep copy of the Header
	r2.Header = make(http.Header, len(r.Header))
	for k, s := range r.Header {
		r2.Header[k] = append([]string(nil), s...)
	}
	return r2
}

// AddHeader adds a header to the request.
func AddHeader(key, value string) Middleware {
	return func(rt http.RoundTripper) http.RoundTripper {
		return internalRoundTripper(func(req *http.Request) (*http.Response, error) {
			req = cloneRequest(req)
			header := req.Header
			if header == nil {
				header = make(http.Header)
			}

			header.Set(key, value)

			return rt.RoundTrip(req)
		})
	}
}
