ARG ARCH=amd64
FROM golang:1.22 as builder
RUN mkdir /build
ADD . /build
WORKDIR /build
RUN CGO_ENABLED=0 GOOS=linux GOARCH=${ARCH} go build -a -o control-plane-controller cmd/control-plane-controller/main_gen.go

FROM gcr.io/distroless/static:nonroot
COPY --from=builder /build/control-plane-controller /
USER 65532:65532

ENTRYPOINT ["/control-plane-controller"]
