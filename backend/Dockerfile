FROM golang:1.26.5-alpine AS build
WORKDIR /app
RUN apk add --no-cache git
COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN go build -o radio ./cmd/local-radio

FROM alpine:3.20
RUN apk add --no-cache ffmpeg ca-certificates

WORKDIR /app

COPY --from=build /app/radio .

ENV S3_ENDPOINT_URL="rustfs:9000"
ENV PORT=8080

EXPOSE 8080
CMD ["./radio"]