FROM golang:1.21-alpine AS build
WORKDIR /src
COPY go.mod ./
COPY *.go ./
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/go-api .

FROM alpine:3.19
RUN apk add --no-cache ca-certificates wget
COPY --from=build /out/go-api /usr/local/bin/go-api
EXPOSE 8080
USER nobody
ENTRYPOINT ["/usr/local/bin/go-api"]
