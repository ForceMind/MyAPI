package service

import "net/http"

const maxWaffoPancakeResponseBytes int64 = 1 << 20

// The SDK reads complete response bodies. Bound them at its HTTP boundary
// without altering its protocol parsing or signature implementation.
type waffoPancakeBoundedTransport struct{ base http.RoundTripper }

func (t waffoPancakeBoundedTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	response, err := t.base.RoundTrip(r)
	if response != nil && response.Body != nil {
		response.Body = http.MaxBytesReader(nil, response.Body, maxWaffoPancakeResponseBytes)
	}
	return response, err
}
