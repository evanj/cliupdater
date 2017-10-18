#!/bin/sh

set -euf -o pipefail
set -x

go test -v ./...
go vet ./...
golint ./...

echo SUCCESS
