# this development image is used for
# live-reload and debug (delve) builds,
# not for the tptdev cli tool itself.

# base target
FROM golang:1.20-alpine as base
RUN apk update && apk add git
RUN mkdir /threeport
RUN go install github.com/go-delve/delve/cmd/dlv@latest && \
    mv /go/bin/dlv /usr/local/bin
WORKDIR /threeport

# live-reload development target
FROM base as live-reload
RUN go install github.com/cosmtrek/air@latest && \
    mv /go/bin/air /usr/local/bin

# static development target
FROM base as dev
ARG BINARY
COPY bin/threeport-${BINARY} /threeport-${BINARY}
