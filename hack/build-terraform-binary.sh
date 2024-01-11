#! /bin/bash

GOOS=linux GOARCH=amd64 go build -o bin/terraform-controller cmd/terraform-controller/main_gen.go

