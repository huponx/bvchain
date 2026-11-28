GO      ?= go
BIN     ?= bin
IMAGE_TAG  ?= dev
IMAGE_REPO ?= ghcr.io/huponx

.PHONY: help tidy upgrade outdated test vet build bvchain bvreport bvctl run-bvchain run-bvreport docker docker-bvchain docker-bvreport clean

help:
	@printf '%s\n' \
		'tidy            go mod tidy (GOWORK=off: resolves gocommon from the module proxy)' \
		'upgrade  upgrade the dependencies (PATCH=1: patch releases only), tidy, build and test' \
		'outdated  list the dependencies that have a newer version' \
		'test            go test ./...' \
		'vet             go vet ./...' \
		'build           build bvchain, bvreport, bvctl into $(BIN)/' \
		'run-bvchain     go run ./cmd/bvchain (needs the environment of .env.example)' \
		'run-bvreport    go run ./cmd/bvreport' \
		'bvctl-run       go run ./cmd/bvctl ARGS='...', see README (bvctl)' \
		'docker          build both images (IMAGE_TAG=dev)' \
		'clean           remove $(BIN)/'

tidy:
	GOWORK=off $(GO) mod tidy

# Upgrade the dependencies to their latest minor/patch releases (PATCH=1: patch releases only), then tidy and
# check that everything still builds and passes. GOWORK=off: resolves against the module proxy, not the sibling
# folders of go.work. Review `git diff go.mod go.sum` before committing.
PATCH ?=
upgrade:
	GOWORK=off $(GO) get -u$(if $(PATCH),=patch) ./...
	GOWORK=off $(GO) mod tidy
	GOWORK=off $(GO) build ./...
	GOWORK=off $(GO) test ./...

# Lists the modules that have a newer version (nothing is changed).
outdated:
	@GOWORK=off $(GO) list -u -m -f '{{if and .Update (not .Indirect)}}{{.Path}} {{.Version}} -> {{.Update.Version}}{{end}}' all

test:
	$(GO) test ./...

vet:
	$(GO) vet ./...

build: bvchain bvreport bvctl

bvchain:
	@mkdir -p $(BIN)
	$(GO) build -o $(BIN)/bvchain ./cmd/bvchain

bvreport:
	@mkdir -p $(BIN)
	$(GO) build -o $(BIN)/bvreport ./cmd/bvreport

bvctl:
	@mkdir -p $(BIN)
	$(GO) build -o $(BIN)/bvctl ./cmd/bvctl

run-bvchain:
	$(GO) run ./cmd/bvchain

run-bvreport:
	$(GO) run ./cmd/bvreport

# make bvctl-run ARGS='notify checkin -shift 1'
#      bvctl-run ARGS='report -month 10/2025 -dry-run -verbose'
ARGS ?= help
.PHONY: bvctl-run
bvctl-run:
	$(GO) run ./cmd/bvctl $(ARGS)

clean:
	rm -rf $(BIN)

docker: docker-bvchain docker-bvreport

docker-bvchain:
	docker build -f build/bvchain/Dockerfile -t $(IMAGE_REPO)/bvchain:$(IMAGE_TAG) .

docker-bvreport:
	docker build -f build/bvreport/Dockerfile -t $(IMAGE_REPO)/bvreport:$(IMAGE_TAG) .
