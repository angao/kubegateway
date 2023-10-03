# This repo's root import path (under GOPATH).
ROOT := github.com/kubewharf/kubegateway

# Module name.
NAME := kube-gateway

# Container image prefix and suffix added to targets.
# The final built images are:
#   $[REGISTRY]/$[IMAGE_PREFIX]$[TARGET]$[IMAGE_SUFFIX]:$[VERSION]
# $[REGISTRY] is an item from $[REGISTRIES], $[TARGET] is an item from $[TARGETS].
IMAGE_PREFIX ?= $(strip )
IMAGE_SUFFIX ?= $(strip )

# Container registries.
REGISTRY ?= cr-cn-guilin-boe.ivolces.com/vke

#
# These variables should not need tweaking.
#

# It's necessary to set this because some environments don't link sh -> bash.
export SHELL := /bin/bash

# It's necessary to set the errexit flags for the bash shell.
export SHELLOPTS := errexit

# Project main package location.
CMD_DIR := ./cmd/kube-gateway

# Project output directory.
OUTPUT_DIR := ./bin

# Build directory.
BUILD_DIR := ./build

IMAGE_NAME := kube-gateway

# Current version of the project.
VERSION      ?= $(shell git describe --tags --always --dirty)
BRANCH       ?= $(shell git branch | grep \* | cut -d ' ' -f2)
GITCOMMIT    ?= $(shell git rev-parse HEAD)
GITTREESTATE ?= $(if $(shell git status --porcelain),dirty,clean)
BUILDDATE    ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
appVersion   ?= $(VERSION)

# Track code version with Docker Label.
DOCKER_LABELS ?= git-describe="$(shell date -u +v%Y%m%d)-$(shell git describe --tags --always --dirty)"

# Default golang flags used in build and test
# -count: run each test and benchmark 1 times. Set this flag to disable test cache
export GOFLAGS ?= -count=1

# Golang standard bin directory.
GOPATH ?= $(shell go env GOPATH)
BIN_DIR := $(GOPATH)/bin
GOLANGCI_LINT := $(BIN_DIR)/golangci-lint

.PHONY: test
test:
	@go test -v -race -coverpkg=./... -coverprofile=coverage.out.tmp -gcflags="all=-l" ./...
	@go tool cover -func coverage.out | tail -n 1 | awk '{ print "Total coverage: " $$3 }'

.PHONY: build

build: build-local

build-local:
	@go build -v -o $(OUTPUT_DIR)/$(NAME)                                        \
	  -ldflags "-s -w -X $(ROOT)/pkg/version.version=$(VERSION)            \
	    -X $(ROOT)/pkg/version.branch=$(BRANCH)                            \
	    -X $(ROOT)/pkg/version.gitCommit=$(GITCOMMIT)                      \
	    -X $(ROOT)/pkg/version.gitTreeState=$(GITTREESTATE)                \
	    -X $(ROOT)/pkg/version.buildDate=$(BUILDDATE)"                     \
	  $(CMD_DIR);

build-linux:
	/bin/bash -c 'GOOS=linux GOARCH=amd64 GOPATH=/go GOFLAGS="$(GOFLAGS)"        \
	  go build -v -o $(OUTPUT_DIR)/$(NAME)                                       \
	    -ldflags "-s -w -X $(ROOT)/pkg/version.version=$(VERSION)          \
	      -X $(ROOT)/pkg/version.branch=$(BRANCH)                          \
	      -X $(ROOT)/pkg/version.gitCommit=$(GITCOMMIT)                    \
	      -X $(ROOT)/pkg/version.gitTreeState=$(GITTREESTATE)              \
	      -X $(ROOT)/pkg/version.buildDate=$(BUILDDATE)"                   \
		$(CMD_DIR)'

.PHONY: container
container:
	@docker build -t $(REGISTRY)/$(IMAGE_NAME):$(VERSION)                  \
	  --label $(DOCKER_LABELS)                                             \
	  -f $(BUILD_DIR)/Dockerfile .;

.PHONY: push
push: container
	@docker push $(REGISTRY)/$(IMAGE_NAME):$(VERSION);

.PHONY: clean
clean:
	@-rm -vrf ${OUTPUT_DIR} _output coverage.out coverage.out.tmp

.PHONY: lint
lint: $(GOLANGCI_LINT)
	@$(GOLANGCI_LINT) run

$(GOLANGCI_LINT):
	curl -sfL https://install.goreleaser.com/github.com/golangci/golangci-lint.sh | sh -s -- -b $(BIN_DIR) 1.46.2