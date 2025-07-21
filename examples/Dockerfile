FROM golang:1.24.5 AS builder

WORKDIR /app

# This will cache the dependencies
COPY go.mod /app/go.mod
COPY go.sum /app/go.sum

RUN go mod download

COPY . /app

RUN CGO_ENABLED=0 go build -o /app/application ./cmd/main.go

# ----------------------------
FROM scratch

COPY --from=builder /app/application /app/application

EXPOSE 8080
CMD ["/app/application"]