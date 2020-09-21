#!/usr/bin/env bash

set -e
echo "" >coverage.txt

for d in $(go list ./...); do
    go test -race -coverprofile=c $d
    if [ -f c ]; then
        cat c >>coverage.txt
        rm c
    fi
done
