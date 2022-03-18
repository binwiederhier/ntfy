VERSION := $(shell git describe --tag)

.PHONY:

help:
	@echo "Typical commands (more see below):"
	@echo "  make build                   - Build web app, documentation and server/client (sloowwww)"
	@echo "  make server-amd64            - Build server/client binary (amd64, no web app or docs)"
	@echo "  make install-amd64           - Install ntfy binary to /usr/bin/ntfy (amd64)"
	@echo "  make web                     - Build the web app"
	@echo "  make docs                    - Build the documentation"
	@echo "  make check                   - Run all tests, vetting/formatting checks and linters"
	@echo
	@echo "Build everything:"
	@echo "  make build                   - Build web app, documentation and server/client"
	@echo "  make clean                   - Clean build/dist folders"
	@echo
	@echo "Build server & client (not release version):"
	@echo "  make server                  - Build server & client (all architectures)"
	@echo "  make server-amd64            - Build server & client (amd64 only)"
	@echo "  make server-armv7            - Build server & client (armv7 only)"
	@echo "  make server-arm64            - Build server & client (arm64 only)"
	@echo
	@echo "Build web app:"
	@echo "  make web                     - Build the web app"
	@echo "  make web-deps                - Install web app dependencies (npm install the universe)"
	@echo "  make web-build               - Actually build the web app"
	@echo
	@echo "Build documentation:"
	@echo "  make docs                    - Build the documentation"
	@echo "  make docs-deps               - Install Python dependencies (pip3 install)"
	@echo "  make docs-build              - Actually build the documentation"
	@echo
	@echo "Test/check:"
	@echo "  make test                    - Run tests"
	@echo "  make race                    - Run tests with -race flag"
	@echo "  make coverage                - Run tests and show coverage"
	@echo "  make coverage-html           - Run tests and show coverage (as HTML)"
	@echo "  make coverage-upload         - Upload coverage results to codecov.io"
	@echo
	@echo "Lint/format:"
	@echo "  make fmt                     - Run 'go fmt'"
	@echo "  make fmt-check               - Run 'go fmt', but don't change anything"
	@echo "  make vet                     - Run 'go vet'"
	@echo "  make lint                    - Run 'golint'"
	@echo "  make staticcheck             - Run 'staticcheck'"
	@echo
	@echo "Releasing:"
	@echo "  make release                 - Create a release"
	@echo "  make release-snapshot        - Create a test release"
	@echo
	@echo "Install locally (requires sudo):"
	@echo "  make install-amd64           - Copy amd64 binary from dist/ to /usr/bin/ntfy"
	@echo "  make install-armv7           - Copy armv7 binary from dist/ to /usr/bin/ntfy"
	@echo "  make install-arm64           - Copy arm64 binary from dist/ to /usr/bin/ntfy"
	@echo "  make install-deb-amd64       - Install .deb from dist/ (amd64 only)"
	@echo "  make install-deb-armv7       - Install .deb from dist/ (armv7 only)"
	@echo "  make install-deb-arm64       - Install .deb from dist/ (arm64 only)"


# Building everything

clean: .PHONY
	rm -rf dist build server/docs server/site

build: web docs server


# Documentation

docs: docs-deps docs-build

docs-deps: .PHONY
	pip3 install -r requirements.txt

docs-build: .PHONY
	mkdocs build


# Web app

web: web-deps web-build

web-deps:
	cd web && npm install
	# If this fails for .svg files, optimizes them with svgo

web-build:
	cd web \
		&& npm run build \
		&& mv build/index.html build/app.html \
		&& rm -rf ../server/site \
		&& mv build ../server/site \
		&& rm \
			../server/site/config.js \
			../server/site/asset-manifest.json


# Main server/client build

server: server-deps
	goreleaser build --snapshot --rm-dist --debug

server-amd64: server-deps-static-sites
	goreleaser build --snapshot --rm-dist --debug --id ntfy_amd64

server-armv7: server-deps-static-sites server-deps-gcc-armv7
	goreleaser build --snapshot --rm-dist --debug --id ntfy_armv7

server-arm64: server-deps-static-sites server-deps-gcc-arm64
	goreleaser build --snapshot --rm-dist --debug --id ntfy_arm64

server-deps: server-deps-static-sites server-deps-all server-deps-gcc

server-deps-gcc: server-deps-gcc-armv7 server-deps-gcc-arm64

server-deps-static-sites:
	mkdir -p server/docs server/site
	touch server/docs/index.html server/site/app.html

server-deps-all:
	which upx || { echo "ERROR: upx not installed. On Ubuntu, run: apt install upx"; exit 1; }

server-deps-gcc-armv7:
	which arm-linux-gnueabi-gcc || { echo "ERROR: ARMv7 cross compiler not installed. On Ubuntu, run: apt install gcc-arm-linux-gnueabi"; exit 1; }

server-deps-gcc-arm64:
	which aarch64-linux-gnu-gcc || { echo "ERROR: ARM64 cross compiler not installed. On Ubuntu, run: apt install gcc-aarch64-linux-gnu"; exit 1; }


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

release: release-deps
	goreleaser release --rm-dist --debug

release-snapshot: release-deps
	goreleaser release --snapshot --skip-publish --rm-dist --debug

release-deps: clean server-deps release-check-tags docs web check

release-check-tags:
	$(eval LATEST_TAG := $(shell git describe --abbrev=0 --tags | cut -c2-))
	if ! grep -q $(LATEST_TAG) docs/install.md; then\
	 	echo "ERROR: Must update docs/install.md with latest tag first.";\
	 	exit 1;\
	fi
	if ! grep -q $(LATEST_TAG) docs/releases.md; then\
		echo "ERROR: Must update docs/releases.mdwith latest tag first.";\
		exit 1;\
	fi


# Installing targets

install-amd64: remove-binary
	sudo cp -a dist/ntfy_amd64_linux_amd64/ntfy /usr/bin/ntfy

install-armv7: remove-binary
	sudo cp -a dist/ntfy_armv7_linux_armv7/ntfy /usr/bin/ntfy

install-arm64: remove-binary
	sudo cp -a dist/ntfy_arm64_linux_arm64/ntfy /usr/bin/ntfy

remove-binary:
	sudo rm -f /usr/bin/ntfy

install-amd64-deb: purge-package
	sudo dpkg -i dist/ntfy_*_linux_amd64.deb

install-armv7-deb: purge-package
	sudo dpkg -i dist/ntfy_*_linux_armv7.deb

install-arm64-deb: purge-package
	sudo dpkg -i dist/ntfy_*_linux_arm64.deb

purge-package:
	sudo systemctl stop ntfy || true
	sudo apt-get purge ntfy || true
