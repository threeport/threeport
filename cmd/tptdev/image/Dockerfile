# this development image is used for
# live-reload and debug (delve) builds,
# not for the tptdev cli tool itself.

# base target
FROM golang:1.20-alpine as base
RUN mkdir /threeport
RUN go install github.com/go-delve/delve/cmd/dlv@latest && \
    mv /go/bin/dlv /usr/local/bin
WORKDIR /threeport

# live-reload development target
FROM base as live-reload
RUN apk update && apk add git
RUN go install github.com/cosmtrek/air@latest && \
    mv /go/bin/air /usr/local/bin

# static development target
FROM base as dev
ARG BINARY
COPY bin/${BINARY} /${BINARY}

# terraform controller target
FROM ubuntu as terraform
COPY bin/${BINARY} /${BINARY}
RUN apt update && apt install -y wget unzip
RUN wget https://releases.hashicorp.com/terraform/1.6.6/terraform_1.6.6_linux_amd64.zip
RUN unzip terraform_1.6.6_linux_amd64.zip
RUN mv terraform /usr/local/bin/

