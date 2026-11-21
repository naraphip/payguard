FROM golang:1.26.0-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -o /out/payguard .
RUN CGO_ENABLED=0 go build -trimpath -o /out/payguard-import-bin-master ./cmd/import-bin-master

FROM alpine:3.23
RUN adduser -D payguard
COPY --from=build /out/payguard /usr/local/bin/payguard
COPY --from=build /out/payguard-import-bin-master /usr/local/bin/payguard-import-bin-master
USER payguard
EXPOSE 8081
ENTRYPOINT ["/usr/local/bin/payguard"]
