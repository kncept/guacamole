package debug

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"path"
	"sort"
	"strings"
	"sync"

	"github.com/AdguardTeam/gomitmproxy"
	"github.com/tidwall/pretty"
)

const (
	// listenHost/listenPort is where the debug proxy accepts connections.
	listenHost = "127.0.0.1"
	listenPort = 8081

	// upstream is the single origin every request is forwarded to.
	upstream = "localhost:8080"
)

// RunDebug starts a MITM proxy that listens on localhost:8081 and forwards
// every request to localhost:8080, printing each request/response pair to the
// console as it passes through. It is meant to be started in its own goroutine;
// the proxy keeps serving after this function returns.

var singleStreamLogFile *bufio.Writer
var singleThreaded sync.Mutex

func RunDebug() {
	proxy := gomitmproxy.NewProxy(gomitmproxy.Config{
		ListenAddr: &net.TCPAddr{
			IP:   net.ParseIP(listenHost),
			Port: listenPort,
		},
		OnRequest:  handleRequest,
		OnResponse: handleResponse,
	})

	// ensure a logs directory exists
	os.MkdirAll("logs", 0755)

	f, err := os.OpenFile(path.Join("logs", "unified.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()
	singleStreamLogFile = bufio.NewWriter(f)

	if err := proxy.Start(); err != nil {
		log.Printf("debug: failed to start proxy on %s:%d: %v", listenHost, listenPort, err)
		return
	}

	log.Printf("debug: proxy listening on http://%s:%d, forwarding to http://%s",
		listenHost, listenPort, upstream)
}

// handleRequest logs the incoming request and rewrites it so that it is sent to
// the upstream server. The body is buffered so it can be both logged and
// forwarded. Returning nil keeps the (mutated) original request.
func handleRequest(session *gomitmproxy.Session) (*http.Request, *http.Response) {
	req := session.Request()

	body, err := io.ReadAll(req.Body)
	_ = req.Body.Close()
	if err != nil {
		body = []byte(fmt.Sprintf("<error reading request body: %v>", err))
	}
	// Restore the body so it can still be sent upstream.
	req.Body = io.NopCloser(bytes.NewReader(body))
	req.ContentLength = int64(len(body))

	// Force the request to target the upstream server.
	req.URL.Scheme = "http"
	req.URL.Host = upstream
	req.Host = upstream

	logRequest(session.ID(), req, body)

	return nil, nil
}

// handleResponse logs the upstream response. The body is buffered so it can be
// both logged and forwarded to the client unchanged. Returning nil keeps the
// (mutated) original response.
func handleResponse(session *gomitmproxy.Session) *http.Response {
	res := session.Response()

	raw, err := io.ReadAll(res.Body)
	_ = res.Body.Close()
	if err != nil {
		raw = []byte(fmt.Sprintf("<error reading response body: %v>", err))
	}
	// Restore the original body so the client receives it unchanged.
	res.Body = io.NopCloser(bytes.NewReader(raw))

	// Produce a human-readable body for logging (decompress gzip if present).
	logBody := raw
	if res.Header.Get("Content-Encoding") == "gzip" {
		if decoded, derr := gunzip(raw); derr == nil {
			logBody = decoded
		}
	}

	logResponse(session.ID(), res, logBody)

	return nil
}

func logFileName(id string) string {
	return path.Join("logs", fmt.Sprintf("proxy-%s.log", id))

}

// logRequest prints a single request as a console block.
func logRequest(id string, req *http.Request, body []byte) {
	f, err := os.OpenFile(logFileName(id), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()
	logWriter := bufio.NewWriter(f)
	output := func(w *bufio.Writer) {
		singleThreaded.Lock()
		defer singleThreaded.Unlock()
		fmt.Fprintf(w, "---- REQUEST %s ----\n", id)
		fmt.Fprintf(w, "%s %s%s HTTP/%d.%d\n",
			req.Method, req.URL.Host, req.URL.RequestURI(), req.ProtoMajor, req.ProtoMinor)
		w.WriteString(formatHeaders(req.Header))
		w.Write(pretty.PrettyOptions(body, &pretty.Options{
			Width:    200,
			Prefix:   "",
			Indent:   " ",
			SortKeys: false,
		}))
		w.WriteString("\n")
		w.Flush()
	}
	output(singleStreamLogFile)
	output(logWriter)

}

// logResponse prints a single response as a console block.
func logResponse(id string, res *http.Response, body []byte) {
	f, err := os.OpenFile(logFileName(id), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()
	logWriter := bufio.NewWriter(f)

	output := func(w *bufio.Writer) {
		singleThreaded.Lock()
		defer singleThreaded.Unlock()
		fmt.Fprintf(w, "---- RESPONSE %s ----\n", id)
		fmt.Fprintf(w, "HTTP/%d.%d %s\n", res.ProtoMajor, res.ProtoMinor, res.Status)
		w.WriteString(formatHeaders(res.Header))
		w.Write(pretty.PrettyOptions(body, &pretty.Options{
			Width:    200,
			Prefix:   "",
			Indent:   " ",
			SortKeys: false,
		}))
		w.WriteString("\n")
		w.Flush()
	}
	output(singleStreamLogFile)
	output(logWriter)
}

// formatHeaders renders headers in a stable, sorted order.
func formatHeaders(h http.Header) string {
	keys := make([]string, 0, len(h))
	for k := range h {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var b strings.Builder
	for _, k := range keys {
		for _, v := range h[k] {
			fmt.Fprintf(&b, "%s: %s\n", k, v)
		}
	}
	return b.String()
}

// gunzip decompresses a gzip-encoded byte slice.
func gunzip(data []byte) ([]byte, error) {
	r, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer r.Close()

	return io.ReadAll(r)
}
