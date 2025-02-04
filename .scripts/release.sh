#!/bin/bash

platforms=(
    "darwin arm64"
    "darwin amd64"
    "linux arm64"
    "linux amd64"
    "windows arm64"
    "windows amd64"
)

for platform in "${platforms[@]}"; do
    set -- $platform
    GOOS=$1
    GOARCH=$2
    dir="${GOOS}-${GOARCH}"
    mkdir -p "$dir"
done

for platform in "${platforms[@]}"; do
    set -- $platform
    GOOS=$1
    GOOS=$1
    GOARCH=$2
    dir="${GOOS}-${GOARCH}"
    GOOS=$GOOS GOARCH=$GOARCH go build -ldflags="-X main.Version=${VERSION} -s -w" -o "$dir/s1h" ./cmd/s1h/main.go
    GOOS=$GOOS GOARCH=$GOARCH go build -ldflags="-X main.Version=${VERSION} -s -w" -o "$dir/s1hpass" ./cmd/s1hpass/main.go
done

for platform in "${platforms[@]}"; do
    set -- $platform
    dir="${1}-${2}"
    tar zcvf "$dir.tar.gz" "$dir"
done
