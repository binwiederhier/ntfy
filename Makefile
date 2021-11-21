GO=$(shell which go)
VERSION := $(shell git describe --tag)

.PHONY:

help:
	@echo "Typical commands:"
	@echo "  make check                       - Run all tests, vetting/formatting checks and linters"
	@echo "  make fmt build-snapshot install  - Build latest and install to local system"
	@echo
	@echo "Test/check:"
	@echo "  make test                        - Run tests"
	@echo "  make race                        - Run tests with -race flag"
	@echo "  make coverage                    - Run tests and show coverage"
	@echo "  make coverage-html               - Run tests and show coverage (as HTML)"
	@echo "  make coverage-upload             - Upload coverage results to codecov.io"
	@echo
	@echo "Lint/format:"
	@echo "  make fmt                         - Run 'go fmt'"
	@echo "  make fmt-check                   - Run 'go fmt', but don't change anything"
	@echo "  make vet                         - Run 'go vet'"
	@echo "  make lint                        - Run 'golint'"
	@echo "  make staticcheck                 - Run 'staticcheck'"
	@echo
	@echo "Build:"
	@echo "  make build                       - Build"
	@echo "  make build-snapshot              - Build snapshot"
	@echo "  make build-simple                - Build (using go build, without goreleaser)"
	@echo "  make clean                       - Clean build folder"
	@echo
	@echo "Releasing (requires goreleaser):"
	@echo "  make release                     - Create a release"
	@echo "  make release-snapshot            - Create a test release"
	@echo
	@echo "Install locally (requires sudo):"
	@echo "  make install                     - Copy binary from dist/ to /usr/bin"
	@echo "  make install-deb                 - Install .deb from dist/"
	@echo "  make install-lint                - Install golint"


# Test/check targets

check: test fmt-check vet lint staticcheck

test: .PHONY
	$(GO) test ./...

race: .PHONY
	$(GO) test -race ./...

coverage:
	mkdir -p build/coverage
	$(GO) test -race -coverprofile=build/coverage/coverage.txt -covermode=atomic ./...
	$(GO) tool cover -func build/coverage/coverage.txt

coverage-html:
	mkdir -p build/coverage
	$(GO) test -race -coverprofile=build/coverage/coverage.txt -covermode=atomic ./...
	$(GO) tool cover -html build/coverage/coverage.txt

coverage-upload:
	cd build/coverage && (curl -s https://codecov.io/bash | bash)


# Lint/formatting targets

fmt:
	$(GO) fmt ./...

fmt-check:
	test -z $(shell gofmt -l .)

vet:
	$(GO) vet ./...

lint:
	which golint || $(GO) get -u golang.org/x/lint/golint
	$(GO) list ./... | grep -v /vendor/ | xargs -L1 golint -set_exit_status

staticcheck: .PHONY
	rm -rf build/staticcheck
	which staticcheck || go get honnef.co/go/tools/cmd/staticcheck
	mkdir -p build/staticcheck
	ln -s "$(GO)" build/staticcheck/go
	PATH="$(PWD)/build/staticcheck:$(PATH)" staticcheck ./...
	rm -rf build/staticcheck


# Building targets

build-deps: .PHONY
	which arm-linux-gnueabi-gcc || { echo "ERROR: ARMv6/v7 cross compiler not installed. On Ubuntu, run: apt install gcc-arm-linux-gnueabi"; exit 1; }
	which aarch64-linux-gnu-gcc || { echo "ERROR: ARM64 cross compiler not installed. On Ubuntu, run: apt install gcc-aarch64-linux-gnu"; exit 1; }

build: build-deps
	goreleaser build --rm-dist --debug

build-snapshot: build-deps
	goreleaser build --snapshot --rm-dist --debug

build-simple: clean
	mkdir -p dist/ntfy_linux_amd64
	export CGO_ENABLED=1
	$(GO) build \
		-o dist/ntfy_linux_amd64/ntfy \
		-tags sqlite_omit_load_extension,osusergo,netgo \
		-ldflags \
		"-linkmode=external -extldflags=-static -s -w -X main.version=$(VERSION) -X main.commit=$(shell git rev-parse --short HEAD) -X main.date=$(shell date +%s)"

clean: .PHONY
	rm -rf dist build


# Releasing targets

release: build-deps
	goreleaser release --rm-dist --debug

release-snapshot: build-deps
	goreleaser release --snapshot --skip-publish --rm-dist --debug


# Installing targets

install:
	sudo rm -f /usr/bin/ntfy
	sudo cp -a dist/ntfy_linux_amd64/ntfy /usr/bin/ntfy

install-deb:
	sudo systemctl stop ntfy || true
	sudo apt-get purge ntfy || true
	sudo dpkg -i dist/*.deb
