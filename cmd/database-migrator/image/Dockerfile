ARG ARCH=amd64
FROM golang:1.21 as builder
RUN mkdir /build
ADD . /build
WORKDIR /build
RUN CGO_ENABLED=0 GOOS=linux GOARCH=${ARCH} go build -a -o database-migrator cmd/database-migrator/main.go

FROM gcr.io/distroless/static:nonroot
COPY --from=builder /build/database-migrator /
USER 65532:65532

ENTRYPOINT ["/database-migrator"]
