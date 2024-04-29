GOPATH=$(shell go env GOPATH)

.PHONY: lint
lint:
	# install it into ./bin/
	@echo "Installing golangci-lint..."
	@curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $(GOPATH)/bin v1.57.2

	${GOPATH}/bin/golangci-lint run --enable=nolintlint,gochecknoinits,bodyclose,gofumpt,gocritic