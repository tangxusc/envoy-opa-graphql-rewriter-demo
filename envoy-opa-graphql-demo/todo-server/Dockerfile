FROM golang:1.26-alpine AS builder

ENV GOPROXY=https://goproxy.cn,direct
ENV GONOSUMDB=*
ENV GONOSUMCHECK=*
ENV GOTOOLCHAIN=auto

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -o /todo-server ./cmd/server

FROM alpine:3.20
RUN apk add --no-cache ca-certificates
COPY --from=builder /todo-server /todo-server
EXPOSE 8081
CMD ["/todo-server"]
