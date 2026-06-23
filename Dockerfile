FROM golang:1.22-alpine AS builder
WORKDIR /app

COPY go.mod ./
COPY cmd ./cmd
COPY internal ./internal

RUN go build -o /bin/frontpocket ./cmd/frontpocket

FROM alpine:3.20
RUN adduser -D -g '' frontpocket
USER frontpocket
WORKDIR /home/frontpocket

COPY --from=builder /bin/frontpocket /usr/local/bin/frontpocket

EXPOSE 8088
ENTRYPOINT ["frontpocket"]
