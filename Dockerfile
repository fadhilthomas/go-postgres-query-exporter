# build stage
FROM golang:1.16.3-alpine3.13 AS build-env
RUN apk --no-cache add build-base
ADD . /src
RUN cd /src && go build -o cmd/main cmd/main.go

# final stage
FROM alpine
WORKDIR /app
COPY --from=build-env /src/cmd/main /app/cmd/
RUN chmod +x /app/cmd/main
CMD ["./cmd/main"]