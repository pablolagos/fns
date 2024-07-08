package main

import (
	"log"
	"time"

	"github.com/pablolagos/fns"
)

func main() {

	// Create self signed certificate
	cert, key, err := GenerateSelfSignedCert("FNS Test Server", "localhost")
	if err != nil {
		log.Fatalf("error creating self-signed cert: %v", err)
	}

	// Create a new server
	s := &fns.Server{
		ReadTimeout: 10 * time.Second,
		Name:        "fns test server",
		Handler: func(ctx *fns.RequestCtx) {
			ctx.Success("text/plain", []byte("Hello, world!"))
		},
	}

	// Enable HTTP/2
	fns.EnableHTTP2(s, fns.ServerConfig{Debug: true})

	// Serve the server
	log.Println("Serving server on :8443")
	if err := s.ListenAndServeTLSEmbed(":8443", cert, key); err != nil {
		log.Fatalf("error serving server: %v", err)
	}
}
