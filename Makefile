.PHONY: all
all: s1h s1hpass

s1h: ./cmd/s1h/main.go ./internal/ssh/ssh.go
	go build -o s1h ./cmd/s1h/main.go

s1hpass: ./cmd/s1hpass/main.go ./internal/ssh/ssh.go
	go build -o s1hpass ./cmd/s1hpass/main.go
