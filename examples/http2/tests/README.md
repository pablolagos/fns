## Tools for testing:

nghttp -nv https://localhost:8443

curl -v --http2 https://localhost:8443

./h2spec generic/1 --port 8443 --tls --insecure --host localhost
