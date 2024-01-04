ARG ARCH=amd64
FROM golang:1.20 as builder
RUN mkdir /build
ADD . /build
WORKDIR /build
RUN CGO_ENABLED=0 GOOS=linux GOARCH=${ARCH} go build -a -o radius-workload-controller cmd/radius-workload-controller/main_gen.go

FROM gcr.io/distroless/static:nonroot
COPY --from=builder /build/radius-workload-controller /
COPY rad /bin/rad
USER 65532:65532

ENTRYPOINT ["/radius-workload-controller"]
