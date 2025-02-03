.PHONY: all
all: s1h s1hpass

s1h: ./cmd/s1h/main.go ./internal/ssh/ssh.go
	go build -o s1h ./cmd/s1h/main.go

s1hpass: ./cmd/s1hpass/main.go ./internal/ssh/ssh.go
	go build -o s1hpass ./cmd/s1hpass/main.go

release:
	-mkdir darwin-arm64
	-mkdir darwin-amd64
	-mkdir linux-arm64
	-mkdir linux-amd64
	-mkdir windows-arm64
	-mkdir windows-amd64
	GOOS=darwin GOARCH=arm64 go build -o darwin-arm64/s1h ./cmd/s1h/main.go
	GOOS=darwin GOARCH=amd64 go build -o darwin-amd64/s1h ./cmd/s1h/main.go
	GOOS=linux GOARCH=arm64 go build -o linux-arm64/s1h ./cmd/s1h/main.go
	GOOS=linux GOARCH=amd64 go build -o linux-amd64/s1h ./cmd/s1h/main.go
	GOOS=windows GOARCH=arm64 go build -o windows-arm64/s1h ./cmd/s1h/main.go
	GOOS=windows GOARCH=amd64 go build -o windows-amd64/s1h ./cmd/s1h/main.go
	GOOS=darwin GOARCH=arm64 go build -o darwin-arm64/s1hpass ./cmd/s1hpass/main.go
	GOOS=darwin GOARCH=amd64 go build -o darwin-amd64/s1hpass ./cmd/s1hpass/main.go
	GOOS=linux GOARCH=arm64 go build -o linux-arm64/s1hpass ./cmd/s1hpass/main.go
	GOOS=linux GOARCH=amd64 go build -o linux-amd64/s1hpass ./cmd/s1hpass/main.go
	GOOS=windows GOARCH=arm64 go build -o windows-arm64/s1hpass ./cmd/s1hpass/main.go
	GOOS=windows GOARCH=amd64 go build -o windows-amd64/s1hpass ./cmd/s1hpass/main.go
	tar zcvf darwin-arm64.tar.gz darwin-arm64
	tar zcvf darwin-amd64.tar.gz darwin-amd64
	tar zcvf linux-arm64.tar.gz linux-arm64
	tar zcvf linux-amd64.tar.gz linux-amd64
	tar zcvf windows-arm64.tar.gz windows-arm64
	tar zcvf windows-amd64.tar.gz windows-amd64

clean:
	-rm s1h
	-rm s1hpass
	-rm darwin-arm64.tar.gz
	-rm darwin-amd64.tar.gz
	-rm linux-arm64.tar.gz
	-rm linux-amd64.tar.gz
	-rm windows-arm64.tar.gz
	-rm windows-amd64.tar.gz
