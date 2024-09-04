FROM golang:1.22-alpine as build

RUN apk update && apk add git

WORKDIR /build

COPY go.mod go.sum ./

RUN go mod download

COPY . .

RUN CGO_ENABLE=0 go build -o server ./cmd/evolution-go

FROM alpine:3.19.1 as final

RUN apk update && apk add --no-cache tzdata ffmpeg

WORKDIR /app

COPY --from=build /build/server .

ENV TZ=America/Sao_Paulo

ENTRYPOINT ["/app/server"]
