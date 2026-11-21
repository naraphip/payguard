FROM golang:1.26.0-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -o /out/payguard .

FROM alpine:3.23
RUN adduser -D payguard
COPY --from=build /out/payguard /usr/local/bin/payguard
USER payguard
EXPOSE 8081
ENTRYPOINT ["/usr/local/bin/payguard"]
