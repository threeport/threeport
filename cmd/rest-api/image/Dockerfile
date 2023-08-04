ARG ARCH=amd64
FROM golang:1.19 as builder
RUN mkdir /build
ADD . /build
WORKDIR /build
RUN CGO_ENABLED=0 GOOS=linux GOARCH=${ARCH} go build -a -o threeport-rest-api cmd/rest-api/main.go

FROM scratch
COPY --from=builder /build/threeport-rest-api /
ENTRYPOINT ["/threeport-rest-api"]

