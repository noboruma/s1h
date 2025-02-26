export VERSION=0.1.0
export GOFLAGS=

.PHONY: all
all: s1h

s1h: ./cmd/s1h/main.go ./internal/ssh/ssh.go
	@COMMIT_HASH=$$(git rev-parse --short HEAD); \
	go build -ldflags="-X main.Version=${VERSION}~$$COMMIT_HASH -s -w" -o s1h ./cmd/s1h/main.go

clean:
	-rm s1h
	-rm -rf dist

release:
	go install github.com/goreleaser/goreleaser/v2@latest
	goreleaser release --clean
