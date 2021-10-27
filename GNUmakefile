NAME=amazon
BINARY=packer-plugin-${NAME}

COUNT?=1
TEST?=$(shell go list ./...)
TESTARGS?=

.PHONY: dev

build:
	@go build -o ${BINARY}

generate:
	@go install github.com/hashicorp/packer-plugin-sdk/cmd/packer-sdc@latest
	@go generate -v ./...

ci-release-docs:
	@go install github.com/hashicorp/packer-plugin-sdk/cmd/packer-sdc@latest
	@packer-sdc renderdocs -src docs -partials docs-partials/ -dst docs/
	@/bin/sh -c "[ -d docs ] && zip -r docs.zip docs/"

dev: build
	@mkdir -p ~/.packer.d/plugins/
	@mv ${BINARY} ~/.packer.d/plugins/${BINARY}

run-example: dev
	@packer build ./example

test:
	@go test -count $(COUNT) $(TEST) -timeout=3m

testacc: dev
	@PACKER_ACC=1 go test -count $(COUNT) -v $(TEST) $(TESTARGS) -timeout=120m
