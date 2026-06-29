VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
BINARY := ward
BUILD_FLAGS := -ldflags="-s -w -X 'ward/internal/cli.Version=$(VERSION)'" -trimpath

build:
	CGO_ENABLED=0 go build $(BUILD_FLAGS) -o $(BINARY) ./cmd/ward

test:
	go test -race ./...

install: build
	install -m 755 $(BINARY) /usr/local/bin/$(BINARY)

build-linux-amd64:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build $(BUILD_FLAGS) -o dist/$(BINARY)-linux-amd64 ./cmd/ward

build-linux-arm64:
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build $(BUILD_FLAGS) -o dist/$(BINARY)-linux-arm64 ./cmd/ward

build-darwin-arm64:
	CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build $(BUILD_FLAGS) -o dist/$(BINARY)-darwin-arm64 ./cmd/ward

build-windows-amd64:
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build $(BUILD_FLAGS) -o dist/$(BINARY)-windows-amd64.exe ./cmd/ward

build-all: build-linux-amd64 build-linux-arm64 build-darwin-arm64 build-windows-amd64

clean:
	rm -rf dist/ $(BINARY)
