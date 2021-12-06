FROM golang:1.17.3-alpine AS build
WORKDIR /tmp/etos-api
COPY . .
RUN go build -o ./bin/api ./cmd/api

FROM alpine:3.13.5
ENTRYPOINT ["/app/api"]
COPY --from=build /tmp/etos-api/bin/api /app/api