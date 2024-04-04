FROM golang:1.21-alpine as builder
ARG ARCH=amd64
ARG TERRAFORM_VERSION=1.7.3
WORKDIR /
# download terraform
RUN apk update && apk add --no-cache ca-certificates wget unzip
RUN wget https://releases.hashicorp.com/terraform/${TERRAFORM_VERSION}/terraform_${TERRAFORM_VERSION}_linux_amd64.zip \
    && unzip terraform_${TERRAFORM_VERSION}_linux_amd64.zip
# build controller
RUN mkdir /build
ADD . /build
WORKDIR /build
RUN CGO_ENABLED=0 GOOS=linux GOARCH=${ARCH} go build -a -o terraform-controller cmd/terraform-controller/main_gen.go

FROM gcr.io/distroless/static:nonroot
COPY --from=builder /build/terraform-controller /
COPY --from=builder /terraform /usr/local/bin/
USER 65532:65532

ENTRYPOINT ["/terraform-controller"]
