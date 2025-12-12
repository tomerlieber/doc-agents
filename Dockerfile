FROM golang:1.24 AS build

ARG SERVICE
WORKDIR /app

COPY go.mod ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/service ./cmd/${SERVICE}

FROM gcr.io/distroless/base-debian12
WORKDIR /
COPY --from=build /out/service /service
EXPOSE 8080
ENTRYPOINT ["/service"]

