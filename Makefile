BIN := groknode
GOPATH_BIN := $(shell go env GOPATH)/bin

.PHONY: build vet test race lint vuln fmt ci clean

build:
	CGO_ENABLED=0 go build -o $(BIN) ./cmd/groknode

vet:
	go vet ./...

test:
	go test ./...

race:
	go test -race ./...

lint:
	$(GOPATH_BIN)/golangci-lint run --timeout=5m

vuln:
	$(GOPATH_BIN)/govulncheck ./...

fmt:
	gofmt -w .

# Full gate set, mirroring the barge project conventions.
ci: build vet lint vuln race
	@echo "All CI gates passed."

clean:
	rm -f $(BIN)
