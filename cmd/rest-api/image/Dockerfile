# if building manually first run:
# CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -a -o threeport-rest-api server.go
# and then build Docker image

FROM scratch
COPY threeport-rest-api /
ENTRYPOINT ["/threeport-rest-api"]
