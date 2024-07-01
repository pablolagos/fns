package fasthttpproxy

import (
	"bufio"
	"encoding/base64"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/powerwaf-cdn/fasthttp"
)

// FasthttpHTTPDialer returns a fasthttp.DialFunc that dials using
// the provided HTTP proxy.
//
// Example usage:
//
//	c := &fasthttp.Client{
//		Dial: fasthttpproxy.FasthttpHTTPDialer("username:password@localhost:9050"),
//	}
func FasthttpHTTPDialer(proxy string) fns.DialFunc {
	return FasthttpHTTPDialerTimeout(proxy, 0)
}

// FasthttpHTTPDialerTimeout returns a fasthttp.DialFunc that dials using
// the provided HTTP proxy using the given timeout.
//
// Example usage:
//
//	c := &fasthttp.Client{
//		Dial: fasthttpproxy.FasthttpHTTPDialerTimeout("username:password@localhost:9050", time.Second * 2),
//	}
func FasthttpHTTPDialerTimeout(proxy string, timeout time.Duration) fns.DialFunc {
	var auth string
	if strings.Contains(proxy, "@") {
		index := strings.LastIndex(proxy, "@")
		auth = base64.StdEncoding.EncodeToString([]byte(proxy[:index]))
		proxy = proxy[index+1:]
	}

	return func(addr string) (net.Conn, error) {
		var conn net.Conn
		var err error

		if strings.HasPrefix(proxy, "[") {
			// ipv6
			if timeout == 0 {
				conn, err = fns.DialDualStack(proxy)
			} else {
				conn, err = fns.DialDualStackTimeout(proxy, timeout)
			}
		} else {
			// ipv4
			if timeout == 0 {
				conn, err = fns.Dial(proxy)
			} else {
				conn, err = fns.DialTimeout(proxy, timeout)
			}
		}

		if err != nil {
			return nil, err
		}

		req := "CONNECT " + addr + " HTTP/1.1\r\nHost: " + addr + "\r\n"
		if auth != "" {
			req += "Proxy-Authorization: Basic " + auth + "\r\n"
		}
		req += "\r\n"

		if _, err := conn.Write([]byte(req)); err != nil {
			return nil, err
		}

		res := fns.AcquireResponse()
		defer fns.ReleaseResponse(res)

		res.SkipBody = true

		if err := res.Read(bufio.NewReader(conn)); err != nil {
			conn.Close()
			return nil, err
		}
		if res.Header.StatusCode() != 200 {
			conn.Close()
			return nil, fmt.Errorf("could not connect to proxy: %s status code: %d", proxy, res.Header.StatusCode())
		}
		return conn, nil
	}
}
