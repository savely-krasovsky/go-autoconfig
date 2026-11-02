# syntax=docker/dockerfile:1

FROM golang:1.27.3@sha256:2920bb124f1a2e12a1d39f0cab756ae0068441ee1da7655dec7f5e374944095b AS build

ARG VERSION=1.0.0

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -buildvcs=false \
    -ldflags="-s -w -X github.com/Shagrat2/go-autoconfig/handlers.version=${VERSION}" \
    -o /out/autoconfig .

FROM gcr.io/distroless/static-debian12:nonroot@sha256:afa5c872c891853ca7fcf1f12c3edb23f7eeef36189728842dd51042ff57f7ab

LABEL org.opencontainers.image.title="go-autoconfig" \
      org.opencontainers.image.description="IMAP/SMTP autodiscover for Thunderbird, Apple Mail and Outlook" \
      org.opencontainers.image.licenses="MIT"

COPY --from=build /out/autoconfig /autoconfig

USER nonroot:nonroot
EXPOSE 8080
ENTRYPOINT ["/autoconfig", "-config", "/etc/autoconfig/config.json"]
