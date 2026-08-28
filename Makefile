.PHONY: install build test

# Install the mdreview binary (to GOBIN, default ~/go/bin) and the
# mdreview skill for Claude Code at the user level, so every project
# can invoke /mdreview.
install:
	go install .
	$$(go env GOPATH)/bin/mdreview init
	@echo "make sure $$(go env GOPATH)/bin is on your PATH"

build:
	go build -o mdreview .

test:
	go vet ./...
	go test ./...
