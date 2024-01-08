ARG ARCH=amd64
FROM golang:1.20 as builder
RUN mkdir /build
ADD . /build
WORKDIR /build
RUN CGO_ENABLED=0 GOOS=linux GOARCH=${ARCH} go build -a -o helm-workload-controller cmd/helm-workload-controller/main_gen.go

FROM gcr.io/distroless/static:nonroot
COPY --from=builder /build/helm-workload-controller /
USER 65532:65532

ENTRYPOINT ["/helm-workload-controller"]
