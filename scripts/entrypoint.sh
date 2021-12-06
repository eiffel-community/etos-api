#!/bin/bash

set -e

# Setup requirements
export GOBIN=$(pwd)/bin
export PATH=$GOBIN:$PATH
make gen
sleep 1
CompileDaemon --build="go build -o bin/api ./cmd/api" --exclude-dir=".git" --exclude-dir="**/**/test" --command=./bin/api -verbose
