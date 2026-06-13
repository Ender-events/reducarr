FROM golang:1.26-alpine AS build

RUN apk add --no-cache git ca-certificates

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN go generate ./...
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /reducarr ./cmd/reducarr

FROM scratch

# Copy CA certificates for HTTPS requests
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

COPY --from=build /reducarr /reducarr

WORKDIR /data

# Expose WebUI port
EXPOSE 8080

ENTRYPOINT ["/reducarr"]
CMD ["serve"]
