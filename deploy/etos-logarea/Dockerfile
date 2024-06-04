FROM golang:1.21-alpine AS build
WORKDIR /tmp/logarea
COPY . .
RUN apk add --no-cache make=4.4.1-r2 git=2.43.0-r0 && make build

FROM alpine:3.17.3
ARG TZ
ENV TZ=$TZ

LABEL org.opencontainers.image.source=https://github.com/eiffel-community/etos-api
LABEL org.opencontainers.image.authors=etos-maintainers@googlegroups.com
LABEL org.opencontainers.image.licenses=Apache-2.0

RUN apk add --no-cache tzdata=2024a-r0
ENTRYPOINT ["/app/etos-logarea"]

COPY --from=build /tmp/logarea/bin/etos-logarea /app/etos-logarea
