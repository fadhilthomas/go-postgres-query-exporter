# build stage
FROM golang:1.16.3-alpine3.13 AS build-env
RUN apk --no-cache add build-base
ADD . /src
RUN cd /src && go build -o main main.go

# final stage
FROM alpine
WORKDIR /app
COPY --from=build-env /src/main /app/
RUN chmod +x /app/main
CMD ["./main"]