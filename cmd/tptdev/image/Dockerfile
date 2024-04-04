# this development image is used for
# live-reload and debug (delve) builds,
# not for the tptdev cli tool itself.

# base target
FROM golang:1.21-alpine as base
RUN mkdir /threeport
RUN go install github.com/go-delve/delve/cmd/dlv@latest && \
    mv /go/bin/dlv /usr/local/bin
WORKDIR /threeport

# live-reload development target
FROM base as live-reload
RUN apk update && apk add --no-cache git
RUN go install github.com/cosmtrek/air@latest && \
    mv /go/bin/air /usr/local/bin

# static development target
FROM base as dev
ARG BINARY
COPY bin/${BINARY} /${BINARY}

# live-reload development target for terraform controller
FROM base as live-reload-terraform
RUN apk update && apk add --no-cache git ca-certificates wget unzip
RUN wget https://releases.hashicorp.com/terraform/${TERRAFORM_VERSION}/terraform_${TERRAFORM_VERSION}_linux_amd64.zip \
    && unzip terraform_${TERRAFORM_VERSION}_linux_amd64.zip -d /usr/local/bin/ \
    && rm terraform_${TERRAFORM_VERSION}_linux_amd64.zip
RUN go install github.com/cosmtrek/air@latest && \
    mv /go/bin/air /usr/local/bin

# static development target for terraform controller
FROM base as dev-terraform
ARG BINARY
ARG TERRAFORM_VERSION=1.7.3
COPY bin/${BINARY} /${BINARY}
RUN apk add --no-cache ca-certificates wget unzip
RUN wget https://releases.hashicorp.com/terraform/${TERRAFORM_VERSION}/terraform_${TERRAFORM_VERSION}_linux_amd64.zip \
    && unzip terraform_${TERRAFORM_VERSION}_linux_amd64.zip -d /usr/local/bin/ \
    && rm terraform_${TERRAFORM_VERSION}_linux_amd64.zip

