FROM golang:1.19 as builder
RUN mkdir /build
ADD . /build
WORKDIR /build
RUN CGO_ENABLED=0 GOOS=linux go build -a -o threeport-kubernetes-runtime-controller cmd/kubernetes-runtime-controller/main_gen.go

FROM scratch
COPY --from=builder /build/threeport-kubernetes-runtime-controller /
ENTRYPOINT ["/threeport-kubernetes-runtime-controller"]

