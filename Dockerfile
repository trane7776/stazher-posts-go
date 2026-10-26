FROM golang:1.26-alpine AS build
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -o /out/server ./cmd/server

FROM alpine:3.22
RUN adduser -D -u 10001 app
WORKDIR /app
COPY --from=build /out/server /app/server
USER app

EXPOSE 18080
ENTRYPOINT ["/app/server"]
