FROM golang:1.24-alpine AS builder

RUN apk add --no-cache gcc musl-dev sqlite-dev

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=1 go build -o /checker .
RUN CGO_ENABLED=0 go build -o /demo-server ./demo-server

FROM alpine:3.20

RUN apk add --no-cache ca-certificates sqlite-libs

WORKDIR /app
COPY --from=builder /checker .
COPY --from=builder /demo-server .
COPY configs ./configs
COPY wordlists ./wordlists

RUN mkdir -p results

EXPOSE 8080

CMD ["./checker", "-addr", ":8080"]
