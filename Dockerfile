FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o /out/atnexus ./cmd/server
RUN mkdir -p /out/image-temp

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/atnexus /atnexus
# Owned by the runtime user so a named volume mounted here is writable.
COPY --from=build --chown=nonroot:nonroot /out/image-temp /var/lib/atnexus/image-temp
EXPOSE 8001 8002 8004 8005
USER nonroot:nonroot
ENTRYPOINT ["/atnexus"]
