FROM golang:1.19 as builder
RUN mkdir /build
ADD . /build
WORKDIR /build
RUN CGO_ENABLED=0 GOOS=linux go build -a -o threeport-agent cmd/agent/main.go

FROM gcr.io/distroless/static:nonroot
WORKDIR /
COPY --from=builder /build/threeport-agent .
USER 65532:65532

ENTRYPOINT ["/threeport-agent"]
