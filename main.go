// Package main is the tynet-cloud-init HTTP server. It hands out per-node
// cloud-init NoCloud seed data (meta-data, user-data, network-config,
// vendor-data) keyed by the requester's FQDN, which the server derives
// from reverse DNS on the source IP.
package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// newHandler builds the HTTP handler that serves NoCloud seed data from
// per-node directories under dir.
//
// lookup performs reverse DNS on a client IP (typically net.LookupAddr).
// It's injected so tests can stub it without depending on real PTR records.
//
// Routes:
//
//   - GET /healthcheck        -> 200 if dir is statable, else 503.
//   - GET /node/<fqdn>/<file> -> serves <dir>/<fqdn>/<file> directly,
//     bypassing the reverse-DNS lookup. Used by tynet-cloud-init-probe so
//     operators can probe any node regardless of their own source IP.
//   - anything else           -> reverse-DNS the request's source IP, take
//     the PTR result as the FQDN (trailing dot stripped), and serve
//     <dir>/<fqdn>/<path>. PTR error or no matching directory -> 404.
//
// "healthcheck" and "node" are reserved hostnames — a node with either name
// would be shadowed by the special routes.
func newHandler(dir string, lookup func(string) ([]string, error)) http.Handler {
	// http.Dir protects against ../ traversal, so it's safe to rewrite
	// r.URL.Path below and hand the cloned request to fs.
	fs := http.FileServer(http.Dir(dir))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Printf("%s %s %s", r.RemoteAddr, r.Method, r.URL.Path)

		// case: healthcheck
		if r.URL.Path == "/healthcheck" {
			if _, err := os.Stat(dir); err != nil {
				http.Error(w, fmt.Sprintf("unhealthy: %v", err), http.StatusServiceUnavailable)
				return
			}
			io.WriteString(w, "ok\n")
			return
		}

		// case: node case (bypass ip lookup)
		if strings.HasPrefix(r.URL.Path, "/node/") {
			r2 := r.Clone(r.Context())
			r2.URL.Path = strings.TrimPrefix(r.URL.Path, "/node")
			fs.ServeHTTP(w, r2)
			return
		}

		// case: reverse ip lookup (default)
		ip, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			http.Error(w, "bad remote addr", http.StatusBadRequest)
			return
		}
		names, err := lookup(ip)
		if err != nil || len(names) == 0 {
			log.Printf("reverse lookup failed for %s: %v", ip, err)
			http.NotFound(w, r)
			return
		}
		// PTR results are dot-terminated FQDNs ("pi2.tynet.us."); strip the
		// trailing dot to get the directory key.
		fqdn := strings.TrimSuffix(names[0], ".")
		log.Printf("resolved %s -> %s", ip, fqdn)
		r2 := r.Clone(r.Context())
		r2.URL.Path = "/" + fqdn + r.URL.Path
		fs.ServeHTTP(w, r2)
	})
}

func defaultDir() string {
	exe, err := os.Executable()
	if err != nil {
		return "cloud-init"
	}
	return filepath.Join(filepath.Dir(exe), "cloud-init")
}

func main() {
	dir := flag.String("dir", defaultDir(), "directory containing per-node seed data")
	addr := flag.String("addr", ":8000", "address to listen on")
	tlsCert := flag.String("tls-cert", "", "path to TLS cert (PEM); HTTPS enabled when set with -tls-key")
	tlsKey := flag.String("tls-key", "", "path to TLS key (PEM)")
	flag.Parse()

	// journald already timestamps each line; drop Go's prefix to avoid duplicates.
	log.SetFlags(0)
	handler := newHandler(*dir, net.LookupAddr)
	if *tlsCert != "" && *tlsKey != "" {
		log.Printf("serving %s on %s (https)", *dir, *addr)
		log.Fatal(http.ListenAndServeTLS(*addr, *tlsCert, *tlsKey, handler))
	}
	log.Printf("serving %s on %s", *dir, *addr)
	log.Fatal(http.ListenAndServe(*addr, handler))
}
