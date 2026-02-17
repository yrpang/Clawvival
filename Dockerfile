ARG GO_VERSION=1
FROM golang:${GO_VERSION}-bookworm AS builder

WORKDIR /usr/src/app
COPY go.mod go.sum ./
RUN go mod download && go mod verify
COPY . .
RUN go build -v -o /run-app ./cmd/server


FROM debian:bookworm

WORKDIR /app

COPY --from=builder /run-app /usr/local/bin/run-app
COPY --from=builder /usr/src/app/skills ./skills

CMD ["run-app"]
