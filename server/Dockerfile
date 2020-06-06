FROM golang:1.14.3-alpine3.11 as builder
RUN apk update && apk add --no-cache git
WORKDIR $GOPATH/src/transcoder/
COPY . .
RUN go build -o builder.bin build.go && chmod 777 builder.bin
RUN ./builder.bin build server -p linux-amd64
RUN cp ./build/transcoderd-linux-amd64 /bin/transcoderd && chmod +x /bin/transcoderd

FROM scratch
COPY --from=builder /bin/transcoderd /bin/transcoderd

ENTRYPOINT ["/bin/transcoderd"]