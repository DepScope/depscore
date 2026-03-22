FROM golang:1.26-alpine AS build
WORKDIR /src
COPY . .
RUN CGO_ENABLED=0 go build -o /depscope ./cmd/depscope

FROM alpine:3.19
RUN apk add --no-cache git ca-certificates
COPY --from=build /depscope /usr/local/bin/depscope
EXPOSE 8080
ENTRYPOINT ["depscope"]
CMD ["server", "--port", "8080"]
