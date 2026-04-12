FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -trimpath -o build/notifier .

FROM gcr.io/distroless/static-debian12
COPY --from=build /src/build/notifier /usr/local/bin/notifier
USER nonroot:nonroot
ENTRYPOINT ["/usr/local/bin/notifier"]
CMD ["daemon"]
