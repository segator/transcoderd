FROM golang:1.14.3-alpine3.11 as builder
RUN apk update && apk add --no-cache git
WORKDIR $GOPATH/src/transcoder/
COPY . .
RUN apk add gcc libgtk-3-dev libappindicator3-dev -y
RUN go run  build.go build worker -p linux-amd64 -m console
RUN cp ./build/transcoderw-linux-amd64 /bin/transcoderw && chmod +x /bin/transcoderw

FROM tentacule/pgstosrt
COPY --from=builder /bin/transcoderw /app/transcoderw

ENTRYPOINT ["/app/transcoderw"]