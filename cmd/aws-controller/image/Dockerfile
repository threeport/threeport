ARG ARCH=amd64
FROM golang:1.20 as builder
RUN mkdir /build
ADD . /build
WORKDIR /build
RUN CGO_ENABLED=0 GOOS=linux GOARCH=${ARCH} go build -a -o threeport-aws-controller cmd/aws-controller/main_gen.go

FROM scratch
COPY --from=builder /build/threeport-aws-controller /
ENTRYPOINT ["/threeport-aws-controller"]

