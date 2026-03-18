FROM golang:1.26-alpine AS builder

ENV GOPROXY=https://goproxy.cn,direct
ENV GONOSUMDB=*
ENV GONOSUMCHECK=*
ENV GOTOOLCHAIN=auto

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -o /graphql-server ./cmd/server

FROM alpine:3.20
RUN apk add --no-cache ca-certificates
COPY --from=builder /graphql-server /graphql-server
EXPOSE 8080
CMD ["/graphql-server"]
