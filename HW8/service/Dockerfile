FROM golang:1.21-alpine AS build
RUN apk add --no-cache git
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/server ./main.go

FROM alpine:3.19
RUN adduser -D appuser
USER appuser
COPY --from=build /out/server /server
EXPOSE 8080
ENTRYPOINT ["/server"]
