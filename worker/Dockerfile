FROM golang:1.14.3-alpine3.11 as builder
RUN apk update && apk add --no-cache git
WORKDIR $GOPATH/src/transcoder/
COPY . .
RUN go build -o builder.bin build.go && chmod 777 builder.bin
RUN ./builder.bin build worker -p linux-amd64
RUN cp ./build/transcoderw-linux-amd64 /bin/transcoderw && chmod +x /bin/transcoderw

FROM tentacule/pgstosrt
COPY --from=builder /bin/transcoderw /app/transcoderw

ENTRYPOINT ["/app/transcoderw"]