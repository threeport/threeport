ARG ARCH=amd64
FROM golang:1.21 as builder
RUN mkdir /build
ADD . /build
WORKDIR /build
RUN CGO_ENABLED=0 GOOS=linux GOARCH=${ARCH} go build -a -o workload-controller cmd/workload-controller/main_gen.go

FROM gcr.io/distroless/static:nonroot
COPY --from=builder /build/workload-controller /
USER 65532:65532

ENTRYPOINT ["/workload-controller"]
