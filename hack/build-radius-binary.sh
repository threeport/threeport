#! /bin/bash

GOOS=linux GOARCH=amd64 go build -o bin/radius-workload-controller cmd/radius-workload-controller/main_gen.go

