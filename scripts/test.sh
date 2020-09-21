#!/usr/bin/env bash

set -e

GOCACHE=/tmp/go-cache

docker run -t --rm \
    -e GOCACHE=/cache \
    -v ${GOCACHE}:/cache \
    -v $(pwd):/ak -w /ak \
    golang:1.13.15 \
    go test -covermode=set -v ./...
