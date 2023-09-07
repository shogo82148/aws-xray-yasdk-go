// Package xrayhttp traces the HTTP requests.
//
// # HTTP Server
//
// [Handler] wraps the provided [net/http.Handler].
// The wrapped [http.Handler] creates a sub-segment and collects information of the request.
//
//	namer := xrayhttp.FixedTracingNamer("myApp")
//	h := xrayhttp.Handler(namer, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
//	  w.Write([]byte("Hello, World!"))
//	}))
//	http.ListenAndServe(":8080", h)
//
// # HTTP Client
//
// [Client] wraps the provided [net/http.Client].
// The wrapped [net/http.Client] sets HTTP-specific xray fields, and adds the trace header to the outbound request.
//
//	client := xrayhttp.Client(nil)
//	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://example.com", nil)
//	if seg.AddError(err) {
//	  panic(err)
//	}
//	resp, err := client.Do(req)
//	if seg.AddError(err) {
//	  panic(err)
//	}
//	defer resp.Body.Close()
package xrayhttp
