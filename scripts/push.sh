#!/usr/bin/env bash

set -e

VERSION=$(git rev-parse --short HEAD)
docker push "negz/actual-kubelets:${VERSION}"
