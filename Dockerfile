FROM golang:1.22-alpine AS build

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/mvno-dashboard ./cmd/api

FROM alpine:3.20

WORKDIR /app

COPY --from=build /out/mvno-dashboard /app/mvno-dashboard
COPY web /app/web

ENV GIN_MODE=release

EXPOSE 8080

CMD ["/app/mvno-dashboard"]
