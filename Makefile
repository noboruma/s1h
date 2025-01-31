.PHONY: all
s1h: ./cmd/s1h/main.go ./internal/ssh/ssh.go
	go build -o s1h ./cmd/s1h/main.go
