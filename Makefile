MAKEFLAGS := --jobs=1
VERSION := $(shell git describe --tag)
COMMIT := $(shell git rev-parse --short HEAD)

.PHONY:

help:
	@echo "Typical commands (more see below):"
	@echo "  make build                      - Build web app, documentation and server/client (sloowwww)"
	@echo "  make cli-linux-amd64            - Build server/client binary (amd64, no web app or docs)"
	@echo "  make install-linux-amd64        - Install ntfy binary to /usr/bin/ntfy (amd64)"
	@echo "  make web                        - Build the web app"
	@echo "  make docs                       - Build the documentation"
	@echo "  make check                      - Run all tests, vetting/formatting checks and linters"
	@echo
	@echo "Build everything:"
	@echo "  make build                      - Build web app, documentation and server/client"
	@echo "  make clean                      - Clean build/dist folders"
	@echo
	@echo "Build server & client (using GoReleaser, not release version):"
	@echo "  make cli                        - Build server & client (all architectures)"
	@echo "  make cli-linux-amd64            - Build server & client (Linux, amd64 only)"
	@echo "  make cli-linux-armv6            - Build server & client (Linux, armv6 only)"
	@echo "  make cli-linux-armv7            - Build server & client (Linux, armv7 only)"
	@echo "  make cli-linux-arm64            - Build server & client (Linux, arm64 only)"
	@echo "  make cli-windows-amd64          - Build client (Windows, amd64 only)"
	@echo "  make cli-darwin-all             - Build client (macOS, arm64+amd64 universal binary)"
	@echo
	@echo "Build server & client (without GoReleaser):"
	@echo "  make cli-linux-server           - Build client & server (no GoReleaser, current arch, Linux)"
	@echo "  make cli-darwin-server          - Build client & server (no GoReleaser, current arch, macOS)"
	@echo "  make cli-client                 - Build client only (no GoReleaser, current arch, Linux/macOS/Windows)"
	@echo
	@echo "Build web app:"
	@echo "  make web                        - Build the web app"
	@echo "  make web-deps                   - Install web app dependencies (npm install the universe)"
	@echo "  make web-build                  - Actually build the web app"
	@echo
	@echo "Build documentation:"
	@echo "  make docs                       - Build the documentation"
	@echo "  make docs-deps                  - Install Python dependencies (pip3 install)"
	@echo "  make docs-build                 - Actually build the documentation"
	@echo
	@echo "Test/check:"
	@echo "  make test                       - Run tests"
	@echo "  make race                       - Run tests with -race flag"
	@echo "  make coverage                   - Run tests and show coverage"
	@echo "  make coverage-html              - Run tests and show coverage (as HTML)"
	@echo "  make coverage-upload            - Upload coverage results to codecov.io"
	@echo
	@echo "Lint/format:"
	@echo "  make fmt                        - Run 'go fmt'"
	@echo "  make fmt-check                  - Run 'go fmt', but don't change anything"
	@echo "  make vet                        - Run 'go vet'"
	@echo "  make lint                       - Run 'golint'"
	@echo "  make staticcheck                - Run 'staticcheck'"
	@echo
	@echo "Releasing:"
	@echo "  make release                    - Create a release"
	@echo "  make release-snapshot           - Create a test release"
	@echo
	@echo "Install locally (requires sudo):"
	@echo "  make install-linux-amd64        - Copy amd64 binary from dist/ to /usr/bin/ntfy"
	@echo "  make install-linux-armv6        - Copy armv6 binary from dist/ to /usr/bin/ntfy"
	@echo "  make install-linux-armv7        - Copy armv7 binary from dist/ to /usr/bin/ntfy"
	@echo "  make install-linux-arm64        - Copy arm64 binary from dist/ to /usr/bin/ntfy"
	@echo "  make install-linux-deb-amd64    - Install .deb from dist/ (amd64 only)"
	@echo "  make install-linux-deb-armv6    - Install .deb from dist/ (armv6 only)"
	@echo "  make install-linux-deb-armv7    - Install .deb from dist/ (armv7 only)"
	@echo "  make install-linux-deb-arm64    - Install .deb from dist/ (arm64 only)"


# Building everything

clean: .PHONY
	rm -rf dist build server/docs server/site

build: web docs cli

update: web-deps-update cli-deps-update docs-deps-update
	docker pull alpine

# Ubuntu-specific

build-deps-ubuntu:
	sudo apt update
	sudo apt install -y \
		curl \
		gcc-aarch64-linux-gnu \
		gcc-arm-linux-gnueabi \
		jq
	which pip3 || sudo apt install -y python3-pip

# Documentation

docs: docs-deps docs-build

docs-build: .PHONY
	@if ! /bin/echo -e "import sys\nif sys.version_info < (3,8):\n exit(1)" | python3; then \
	  if which python3.8; then \
	  	echo "python3.8 $(shell which mkdocs) build"; \
	    python3.8 $(shell which mkdocs) build; \
	  else \
	    echo "ERROR: Python version too low. mkdocs-material needs >= 3.8"; \
	    exit 1; \
	  fi; \
	else \
	  echo "mkdocs build"; \
	  mkdocs build; \
	fi

docs-deps: .PHONY
	pip3 install -r requirements.txt

docs-deps-update: .PHONY
	pip3 install -r requirements.txt --upgrade


# Web app

web: web-deps web-build

web-build:
	cd web \
		&& npm run build \
		&& mv build/index.html build/app.html \
		&& rm -rf ../server/site \
		&& mv build ../server/site \
		&& rm \
			../server/site/config.js \
			../server/site/asset-manifest.json

web-deps:
	cd web && npm install
	# If this fails for .svg files, optimize them with svgo

web-deps-update:
	cd web && npm update


# Main server/client build

cli: cli-deps
	goreleaser build --snapshot --rm-dist

cli-linux-amd64: cli-deps-static-sites
	goreleaser build --snapshot --rm-dist --id ntfy_linux_amd64

cli-linux-armv6: cli-deps-static-sites cli-deps-gcc-armv6-armv7
	goreleaser build --snapshot --rm-dist --id ntfy_linux_armv6

cli-linux-armv7: cli-deps-static-sites cli-deps-gcc-armv6-armv7
	goreleaser build --snapshot --rm-dist --id ntfy_linux_armv7

cli-linux-arm64: cli-deps-static-sites cli-deps-gcc-arm64
	goreleaser build --snapshot --rm-dist --id ntfy_linux_arm64

cli-windows-amd64: cli-deps-static-sites
	goreleaser build --snapshot --rm-dist --id ntfy_windows_amd64

cli-darwin-all: cli-deps-static-sites
	goreleaser build --snapshot --rm-dist --id ntfy_darwin_all

cli-linux-server: cli-deps-static-sites
	# This is a target to build the CLI (including the server) manually.
	# Use this for development, if you really don't want to install GoReleaser ...
	mkdir -p dist/ntfy_linux_server server/docs
	CGO_ENABLED=1 go build \
		-o dist/ntfy_linux_server/ntfy \
		-tags sqlite_omit_load_extension,osusergo,netgo \
		-ldflags \
		"-linkmode=external -extldflags=-static -s -w -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.date=$(shell date +%s)"

cli-darwin-server: cli-deps-static-sites
	# This is a target to build the CLI (including the server) manually.
	# Use this for macOS/iOS development, so you have a local server to test with.
	mkdir -p dist/ntfy_darwin_server server/docs
	CGO_ENABLED=1 go build \
		-o dist/ntfy_darwin_server/ntfy \
		-tags sqlite_omit_load_extension,osusergo,netgo \
		-ldflags \
		"-linkmode=external -s -w -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.date=$(shell date +%s)"

cli-client: cli-deps-static-sites
	# This is a target to build the CLI (excluding the server) manually. This should work on Linux/macOS/Windows.
	# Use this for development, if you really don't want to install GoReleaser ...
	mkdir -p dist/ntfy_client server/docs
	CGO_ENABLED=0 go build \
		-o dist/ntfy_client/ntfy \
		-tags noserver \
		-ldflags \
		"-X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.date=$(shell date +%s)"

cli-deps: cli-deps-static-sites cli-deps-all cli-deps-gcc

cli-deps-gcc: cli-deps-gcc-armv6-armv7 cli-deps-gcc-arm64

cli-deps-static-sites:
	mkdir -p server/docs server/site
	touch server/docs/index.html server/site/app.html

cli-deps-all:
	go install github.com/goreleaser/goreleaser@latest

cli-deps-gcc-armv6-armv7:
	which arm-linux-gnueabi-gcc || { echo "ERROR: ARMv6/ARMv7 cross compiler not installed. On Ubuntu, run: apt install gcc-arm-linux-gnueabi"; exit 1; }

cli-deps-gcc-arm64:
	which aarch64-linux-gnu-gcc || { echo "ERROR: ARM64 cross compiler not installed. On Ubuntu, run: apt install gcc-aarch64-linux-gnu"; exit 1; }

cli-deps-update:
	go get -u
	go install honnef.co/go/tools/cmd/staticcheck@latest
	go install golang.org/x/lint/golint@latest
	go install github.com/goreleaser/goreleaser@latest

cli-build-results:
	cat dist/config.yaml
	[ -f dist/artifacts.json ] && cat dist/artifacts.json | jq . || true
	[ -f dist/metadata.json ] && cat dist/metadata.json | jq . || true
	[ -f dist/checksums.txt ] && cat dist/checksums.txt || true
	find dist -maxdepth 2 -type f \
		\( -name '*.deb' -or -name '*.rpm' -or -name '*.zip' -or -name '*.tar.gz' -or -name 'ntfy' \) \
		-and -not -path 'dist/goreleaserdocker*' \
		-exec sha256sum {} \;

# Test/check targets

check: test fmt-check vet lint staticcheck

test: .PHONY
	go test -v $(shell go list ./... | grep -vE 'ntfy/(test|examples|tools)')

race: .PHONY
	go test -race $(shell go list ./... | grep -vE 'ntfy/(test|examples|tools)')

coverage:
	mkdir -p build/coverage
	go test -race -coverprofile=build/coverage/coverage.txt -covermode=atomic $(shell go list ./... | grep -vE 'ntfy/(test|examples|tools)')
	go tool cover -func build/coverage/coverage.txt

coverage-html:
	mkdir -p build/coverage
	go test -race -coverprofile=build/coverage/coverage.txt -covermode=atomic $(shell go list ./... | grep -vE 'ntfy/(test|examples|tools)')
	go tool cover -html build/coverage/coverage.txt

coverage-upload:
	cd build/coverage && (curl -s https://codecov.io/bash | bash)


# Lint/formatting targets

fmt:
	gofmt -s -w .

fmt-check:
	test -z $(shell gofmt -l .)

vet:
	go vet ./...

lint:
	which golint || go install golang.org/x/lint/golint@latest
	go list ./... | grep -v /vendor/ | xargs -L1 golint -set_exit_status

staticcheck: .PHONY
	rm -rf build/staticcheck
	which staticcheck || go install honnef.co/go/tools/cmd/staticcheck@latest
	mkdir -p build/staticcheck
	ln -s "go" build/staticcheck/go
	PATH="$(PWD)/build/staticcheck:$(PATH)" staticcheck ./...
	rm -rf build/staticcheck


# Releasing targets

release: clean update cli-deps release-checks docs web check
	goreleaser release --rm-dist

release-snapshot: clean update cli-deps docs web check
	goreleaser release --snapshot --skip-publish --rm-dist

release-checks:
	$(eval LATEST_TAG := $(shell git describe --abbrev=0 --tags | cut -c2-))
	if ! grep -q $(LATEST_TAG) docs/install.md; then\
	 	echo "ERROR: Must update docs/install.md with latest tag first.";\
	 	exit 1;\
	fi
	if ! grep -q $(LATEST_TAG) docs/releases.md; then\
		echo "ERROR: Must update docs/releases.md with latest tag first.";\
		exit 1;\
	fi
	if [ -n "$(shell git status -s)" ]; then\
	  echo "ERROR: Git repository is in an unclean state.";\
	  exit 1;\
	fi


# Installing targets

install-linux-amd64: remove-binary
	sudo cp -a dist/ntfy_linux_amd64_linux_amd64_v1/ntfy /usr/bin/ntfy

install-linux-armv6: remove-binary
	sudo cp -a dist/ntfy_linux_armv6_linux_arm_6/ntfy /usr/bin/ntfy

install-linux-armv7: remove-binary
	sudo cp -a dist/ntfy_linux_armv7_linux_arm_7/ntfy /usr/bin/ntfy

install-linux-arm64: remove-binary
	sudo cp -a dist/ntfy_linux_arm64_linux_arm64/ntfy /usr/bin/ntfy

remove-binary:
	sudo rm -f /usr/bin/ntfy

install-linux-amd64-deb: purge-package
	sudo dpkg -i dist/ntfy_*_linux_amd64.deb

install-linux-armv6-deb: purge-package
	sudo dpkg -i dist/ntfy_*_linux_armv6.deb

install-linux-armv7-deb: purge-package
	sudo dpkg -i dist/ntfy_*_linux_armv7.deb

install-linux-arm64-deb: purge-package
	sudo dpkg -i dist/ntfy_*_linux_arm64.deb

purge-package:
	sudo systemctl stop ntfy || true
	sudo apt-get purge ntfy || true
