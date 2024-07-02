package main

import (
	"log"

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
		Handler: func(ctx *fns.RequestCtx) {
			ctx.Success("text/plain", []byte("Hello, world!"))
		},
	}

	// Enable HTTP/2
	fns.EnableHTTP2(s, fns.DefaultH2Config())

	// Listen and serve. It will block the execution.
	err = fns.ListenAndServeTLSEmbed(":8443", cert, key, s.Handler)
	if err != nil {
		log.Fatalf("Error in ListenAndServe: %v", err)
	}

}
