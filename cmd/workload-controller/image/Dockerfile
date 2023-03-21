FROM golang:1.19 as builder
RUN mkdir /build
ADD . /build
WORKDIR /build
RUN CGO_ENABLED=0 GOOS=linux go build -a -o threeport-workload-controller cmd/workload-controller/main.go

FROM scratch
COPY --from=builder /build/threeport-workload-controller /
ENTRYPOINT ["/threeport-workload-controller"]
