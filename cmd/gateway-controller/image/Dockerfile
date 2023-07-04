FROM golang:1.19 as builder
RUN mkdir /build
ADD . /build
WORKDIR /build
RUN CGO_ENABLED=0 GOOS=linux go build -a -o threeport-gateway-controller cmd/gateway-controller/main_gen.go

FROM scratch
COPY --from=builder /build/threeport-gateway-controller /
ENTRYPOINT ["/threeport-gateway-controller"]

