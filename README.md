# 🚧 Under Development 🚧

**This software is currently in development and is not yet production-ready.**

Please be aware that features may change, and the application may contain bugs or incomplete functionality. Use at your own risk, and feel free to contribute or provide feedback!

---

# FNS (FastNetServer)

Welcome to FNS, a high-performance HTTP server designed to meet the demands of modern web applications. Built on the robust foundations of fasthttp, FNS offers enhanced compatibility with HTTP/2.0 and provides unbuffered responses for superior performance.

![FNS Logo](https://example.com/fns-logo.png) <!-- Replace with actual logo URL -->

## 🚀 Features

- **Blazing Fast Performance**: Experience lightning-fast request processing.
- **HTTP/2.0 Compatibility**: Full support for the latest HTTP protocol.
- **Unbuffered Responses**: Real-time data delivery without buffering.
- **Ease of Use**: Simple API designed for developers.
- **Scalability**: Efficient resource management for handling large volumes of traffic.

## 📦 Installation

Get started with FNS in just a few steps.

### Prerequisites

Ensure you have Go installed on your machine. [Download Go](https://golang.org/dl/)

### Installing FNS

```bash
go get -u github.com/pablolagos/fns
```

## 🛠 Usage
Setting up a basic FNS server is straightforward. Here's a quick example:

```go
package main

import (
    "github.com/pablolagos/fns"
    "log"
)

func main() {
    fns.Get("/hello", func(ctx *fns.RequestCtx) {
        ctx.WriteString("Hello, World!")
    })

    log.Println("Starting server on :8080")
    if err := fns.ListenAndServe(":8080", nil); err != nil {
        log.Fatalf("Error starting server: %s", err)
    }
}
```

## ⚡ Allocation-free header iteration

`RequestHeader.VisitAll` takes a closure, which the compiler must move to the
heap because `VisitAll` is not inlined — costing allocations on hot paths that
scan every header. For those paths, `RequestHeader` exposes an indexed view that
iterates without a closure:

```go
for i, n := 0, h.KVLen(); i < n; i++ {
    key, value := h.KVAt(i)
    // ...
}
```

`KVLen`/`KVAt` return the exact same entries as `VisitAll`, in the same order —
including the synthetic special headers (`Host`, `Content-Length`,
`Content-Type`, `User-Agent`, `Trailer`, the combined `Cookie`, and a trailing
`Connection: close`). The view is built lazily once per request and reused, and
is invalidated on `Reset`, so repeated scans are allocation-free after the first.
The returned slices alias header storage (including reused special-header
buffers); copy them if you need to retain them past the next reset.

It is named `KVAt` (not `KV`) on purpose: `RequestHeader` already has a `Len`
method, so a `KV` accessor would make it accidentally satisfy generic
`Len()`+`KV(i)` iterator interfaces with `Len`'s incompatible header-count
semantics.

## 📖 Documentation
Detailed documentation is available on our wiki. Here are some quick links to get you started:

- Getting Started
- HTTP/2.0 Setup
- Examples
## 🤝 Contributing
We welcome contributions from the community! Please read our Contributing Guidelines to get started.

## 📝 License
FNS is licensed under the MIT License. See the LICENSE file for more details.

### 🌟 Acknowledgements
FNS is based on the incredible work done by FastHTTP created by Aliaksandr Valialkin, VertaMedia, Kirill Danshin, Erik Dubbelboer and all FastHTTP Authors.

Made with ❤️ for a faster web.
