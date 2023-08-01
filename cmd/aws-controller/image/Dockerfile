FROM golang:1.19 as builder
RUN mkdir /build
ADD . /build
WORKDIR /build
RUN CGO_ENABLED=0 GOOS=linux go build -a -o threeport-aws-controller cmd/aws-controller/main_gen.go

FROM scratch
COPY --from=builder /build/threeport-aws-controller /
ENTRYPOINT ["/threeport-aws-controller"]

