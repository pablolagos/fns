package fns_test

import (
	"log"

	"github.com/pablolagos/fns"
)

func ExampleFS() {
	fs := &fns.FS{
		// Path to directory to serve.
		Root: "/var/www/static-site",

		// Generate index pages if client requests directory contents.
		GenerateIndexPages: true,

		// Enable transparent compression to save network traffic.
		Compress: true,
	}

	// Create request handler for serving static files.
	h := fs.NewRequestHandler()

	// Start the server.
	if err := fns.ListenAndServe(":8080", h); err != nil {
		log.Fatalf("error in ListenAndServe: %v", err)
	}
}
