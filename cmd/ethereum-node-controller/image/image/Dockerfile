FROM golang:1.19 as builder
RUN mkdir /build
ADD . /build
WORKDIR /build
RUN CGO_ENABLED=0 GOOS=linux go build -a -o threeport-ethereum-node-controller cmd/ethereum-node-controller/main.go

FROM scratch
COPY --from=builder /build/threeport-ethereum-node-controller /
ENTRYPOINT ["/threeport-ethereum-node-controller"]
