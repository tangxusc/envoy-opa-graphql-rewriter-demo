GO ?= /Users/tangxu/sdk/go1.16rc1/bin/go
GOFMT ?= /Users/tangxu/sdk/go1.16rc1/bin/gofmt
PROXY ?= http://127.0.0.1:7890
GOPROXY_URL ?= https://goproxy.cn,direct
GOCACHE_DIR ?= $(CURDIR)/.gocache
GOPATH_DIR ?= $(CURDIR)/.gopath
GOMODCACHE_DIR ?= $(GOPATH_DIR)/pkg/mod

GO_ENV = HTTP_PROXY=$(PROXY) HTTPS_PROXY=$(PROXY) http_proxy=$(PROXY) https_proxy=$(PROXY) GOPROXY=$(GOPROXY_URL) GOSUMDB=off GOCACHE=$(GOCACHE_DIR) GOPATH=$(GOPATH_DIR) GOMODCACHE=$(GOMODCACHE_DIR)

.PHONY: init generate fmt test run clean

init:
	$(GO_ENV) $(GO) mod tidy

generate:
	$(GO_ENV) $(GO) run github.com/99designs/gqlgen generate

fmt:
	$(GOFMT) -w $(shell find . -type f -name '*.go' -not -path './vendor/*')

test:
	$(GO_ENV) $(GO) test ./...

run:
	$(GO_ENV) $(GO) run ./cmd/server

clean:
	rm -rf graph/generated graph/model
