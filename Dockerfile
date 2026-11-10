FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
COPY modules/athub/go.mod modules/athub/go.sum ./modules/athub/
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o /out/oauth ./cmd/server

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/oauth /oauth
EXPOSE 8001 8003
USER nonroot:nonroot
ENTRYPOINT ["/oauth"]
