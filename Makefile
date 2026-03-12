COMMIT_SHA ?= $(shell git rev-parse HEAD)
REPONAME ?= ghcr.io/hanzoai
IMAGE_NAME ?= otel-collector
MIGRATOR_IMAGE_NAME ?= o11y-schema-migrator
CONFIG_FILE ?= ./config/default-config.yaml
DOCKER_TAG ?= latest

GOOS ?= $(shell go env GOOS)
GOARCH ?= $(shell go env GOARCH)
GOPATH ?= $(shell go env GOPATH)
GOTEST=go test -v $(RACE)
GOFMT=gofmt
FMT_LOG=.fmt.log
IMPORT_LOG=.import.log

DATASTORE_HOST ?= 127.0.0.1
DATASTORE_PORT ?= 9000

LD_FLAGS ?=


.PHONY: install-tools
install-tools:
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@v2.6.0

.DEFAULT_GOAL := test-and-lint

.PHONY: test-and-lint
test-and-lint: test fmt lint

.PHONY: test
test:
	go test -count=1 -v -race -cover ./...

.PHONY: build
build:
	go build -o .build/${GOOS}-${GOARCH}/hanzo-otel-collector ./cmd/o11yotelcollector
	go build -o .build/${GOOS}-${GOARCH}/o11y-schema-migrator ./cmd/o11yschemamigrator

.PHONY: amd64
amd64:
	make GOARCH=amd64 build

.PHONY: arm64
arm64:
	make GOARCH=arm64 build

.PHONY: build-all
build-all: amd64 arm64

.PHONY: run
run:
	go run cmd/o11yotelcollector/main.go --config ${CONFIG_FILE}

.PHONY: fmt
fmt:
	@echo Running go fmt on query service ...
	@$(GOFMT) -e -s -l -w .

.PHONY: build-and-push-collector
build-and-push-collector:
	@echo "------------------"
	@echo  "--> Build and push otel collector docker image"
	@echo "------------------"
	docker buildx build --platform linux/amd64,linux/arm64 --progress plain \
		--no-cache --push -f cmd/o11yotelcollector/Dockerfile \
		--tag $(REPONAME)/$(IMAGE_NAME):$(DOCKER_TAG) .

.PHONY: build-collector
build-collector:
	@echo "------------------"
	@echo  "--> Build otel collector docker image"
	@echo "------------------"
	docker build --build-arg TARGETPLATFORM="linux/amd64" \
		--no-cache -f cmd/o11yotelcollector/Dockerfile --progress plain \
		--tag $(REPONAME)/$(IMAGE_NAME):$(DOCKER_TAG) .

.PHONY: build-schema-migrator
build-schema-migrator:
	@echo "------------------"
	@echo  "--> Build schema migrator docker image"
	@echo "------------------"
	docker build --build-arg TARGETPLATFORM="linux/amd64" \
		--no-cache -f cmd/o11yschemamigrator/Dockerfile --progress plain \
		--tag $(REPONAME)/$(MIGRATOR_IMAGE_NAME):$(DOCKER_TAG) .

.PHONY: build-and-push-schema-migrator
build-and-push-schema-migrator:
	@echo "------------------"
	@echo  "--> Build and push schema migrator docker image"
	@echo "------------------"
	docker buildx build --platform linux/amd64,linux/arm64 --progress plain \
		--no-cache --push -f cmd/o11yschemamigrator/Dockerfile \
		--tag $(REPONAME)/$(MIGRATOR_IMAGE_NAME):$(DOCKER_TAG) .

.PHONY: lint
lint:
	@echo "Running linters..."
	@$(GOPATH)/bin/golangci-lint -v --config .golangci.yml run && echo "Done."

.PHONY: install-ci
install-ci: install-tools

.PHONY: test-ci
test-ci: lint

.PHONY: migrator
migrator:
	@echo "------------------"
	@echo "--> Running schema migrator for $(DATASTORE_HOST):$(DATASTORE_PORT)"
	@echo "------------------"
	go run cmd/o11yschemamigrator/main.go sync --dsn "clickhouse://$(DATASTORE_HOST):$(DATASTORE_PORT)" --dev
