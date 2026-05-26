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

func newHandler(dir string, lookup func(string) ([]string, error)) http.Handler {
	fs := http.FileServer(http.Dir(dir))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Printf("%s %s %s", r.RemoteAddr, r.Method, r.URL.Path)
		if r.URL.Path == "/healthcheck" {
			if _, err := os.Stat(dir); err != nil {
				http.Error(w, fmt.Sprintf("unhealthy: %v", err), http.StatusServiceUnavailable)
				return
			}
			io.WriteString(w, "ok\n")
			return
		}
		if strings.HasPrefix(r.URL.Path, "/node/") {
			r2 := r.Clone(r.Context())
			r2.URL.Path = strings.TrimPrefix(r.URL.Path, "/node")
			fs.ServeHTTP(w, r2)
			return
		}
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
		shortName := strings.SplitN(strings.TrimSuffix(names[0], "."), ".", 2)[0]
		log.Printf("resolved %s -> %s", ip, shortName)
		r2 := r.Clone(r.Context())
		r2.URL.Path = "/" + shortName + r.URL.Path
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
	flag.Parse()

	// journald already timestamps each line; drop Go's prefix to avoid duplicates.
	log.SetFlags(0)
	log.Printf("serving %s on %s", *dir, *addr)
	log.Fatal(http.ListenAndServe(*addr, newHandler(*dir, net.LookupAddr)))
}
