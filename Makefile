PKGS = $(shell go list ./...)
GOFILES = $(shell find . -name '*.go' -and -not -path "./vendor/*")
UNFMT = $(shell gofmt -l ${GOFILES})
GITSHA = $(shell git rev-parse --short HEAD)
GITTAG = $(shell git describe --tags --abbrev=0)

LD_FLAGS = -s -w -extldflags "-static" -X tile38-cluster-manager/version.SHA=${GITSHA} -X tile38-cluster-manager/version.Tag=${GITTAG}
GOOS ?= $(shell go env GOOS)
GOARCH ?= $(shell go env GOARCH)

test: fmtcheck
	@echo "==> Running tests"
	@go test -v -count=1 -timeout=300s ${PKGS}

.PHONY: test

fmt:
	@echo "==> Fixing code with gofmt"
	@gofmt -s -w ${GOFILES}

.PHONY: fmt

fmtcheck:
	@echo "==> Checking code for gofmt compliance"
	@[ -z "${UNFMT}" ] || ( echo "Following files are not gofmt compliant.\n\n${UNFMT}\n\nRun 'make fmt' for reformat code"; exit 1 )

.PHONY: fmtcheck

build:
	@echo "Building Cluster Manager (${GOOS}/${GOARCH}) from tag ${GITTAG} at SHA ${GITSHA}"
	@CGO_ENABLED=0 GOOS=${GOOS} GOARCH=${GOARCH} go build -a -o="pkg/tile38-cluster-manager" -ldflags "${LD_FLAGS}"
.PHONY: build

testacc: fmtcheck
	@TEST_ACC=true go test -v -run '^TestAcc*' -count=1 -failfast -timeout=600s ${PKGS}

.PHONY: testacc

gen-flags:
	@go run -tags doc doc.go app.go > site/flags.txt
.PHONY: gen-flags

doc: gen-flags
	@docker run --rm -it -v $(PWD):/docs --entrypoint /bin/sh squidfunk/mkdocs-material /docs/mkdocs.sh build
.PHONY: doc

doc-server: gen-flags
	@docker run --rm -it -v $(PWD):/docs --entrypoint /bin/sh squidfunk/mkdocs-material /docs/mkdocs.sh serve --dev-addr 0.0.0.0:8000
.PHONY: doc-server