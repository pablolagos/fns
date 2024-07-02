package main

import (
	"fmt"
	"os"

	"github.com/pablolagos/fns"
)

func main() {
	// Get URI from a pool
	url := fns.AcquireURI()
	url.Parse(nil, []byte("http://localhost:8080/"))
	url.SetUsername("Aladdin")
	url.SetPassword("Open Sesame")

	hc := &fns.HostClient{
		Addr: "localhost:8080", // The host address and port must be set explicitly
	}

	req := fns.AcquireRequest()
	req.SetURI(url)     // copy url into request
	fns.ReleaseURI(url) // now you may release the URI

	req.Header.SetMethod(fns.MethodGet)
	resp := fns.AcquireResponse()
	err := hc.Do(req, resp)
	fns.ReleaseRequest(req)
	if err == nil {
		fmt.Printf("Response: %s\n", resp.Body())
	} else {
		fmt.Fprintf(os.Stderr, "Connection error: %v\n", err)
	}
	fns.ReleaseResponse(resp)
}
