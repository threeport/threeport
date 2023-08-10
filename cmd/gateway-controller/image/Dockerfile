ARG ARCH=amd64
FROM golang:1.20 as builder
RUN mkdir /build
ADD . /build
WORKDIR /build
RUN CGO_ENABLED=0 GOOS=linux GOARCH=${ARCH} go build -a -o threeport-gateway-controller cmd/gateway-controller/main_gen.go

FROM gcr.io/distroless/static:nonroot
COPY --from=builder /build/threeport-gateway-controller /
USER 65532:65532

ENTRYPOINT ["/threeport-gateway-controller"]
