#!/bin/sh

for OS in "freebsd" "linux" "darwin" "windows"; do
	for ARCH in "386" "amd64"; do
		GOOS=$OS  CGO_ENABLED=0 GOARCH=$ARCH go build -o korra
		ARCHIVE=korra-$OS-$ARCH.tar.gz
		tar -czf $ARCHIVE korra
		echo $ARCHIVE
	done
done
