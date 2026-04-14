# syntax=docker/dockerfile:1

FROM node:22-alpine AS frontend-builder
WORKDIR /src/frontend

# frontend/postinstall copies altcha to ../static/public/static
COPY frontend/package.json frontend/package-lock.json ./
RUN mkdir -p ../static/public/static && npm ci

COPY frontend/ ./
RUN npm run build


FROM python:3.12-alpine AS docs-builder
WORKDIR /src/docs/docs
COPY docs/docs/requirements.txt ./
RUN pip install --no-cache-dir -r requirements.txt
COPY docs/docs/ ./
RUN mkdocs build


FROM golang:1.26-alpine AS go-builder
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY cmd/ ./cmd/
COPY internal/ ./internal/
COPY models/ ./models/
COPY i18n/ ./i18n/
COPY static/ ./static/
COPY permissions.json config.toml.sample ./

RUN go build -trimpath -ldflags="-s -w" -o /out/listpocket ./cmd


FROM alpine:3.22
RUN apk add --no-cache ca-certificates tzdata \
    && addgroup -S listpocket \
    && adduser -S -G listpocket listpocket \
    && mkdir -p /app/pb_data /frontend

WORKDIR /app

COPY --from=go-builder /out/listpocket /app/listpocket
COPY --from=go-builder /src/permissions.json /app/permissions.json
COPY --from=go-builder /src/config.toml.sample /app/config.toml.sample
COPY --from=go-builder /src/static /app/static
COPY --from=go-builder /src/i18n /app/i18n
COPY --from=frontend-builder /src/frontend/dist /app/frontend/dist
COPY --from=docs-builder /src/docs/docs/_out /app/docs/docs/_out
COPY docs/swagger /app/docs/swagger

RUN ln -s /app/frontend/dist /frontend/dist \
    && cp /app/config.toml.sample /app/config.toml \
    && chmod +x /app/listpocket \
    && chown -R listpocket:listpocket /app /frontend /app/docs

ENV LISTPOCKET_APP__ADDRESS=0.0.0.0:9000
EXPOSE 9000

VOLUME ["/app/pb_data"]

USER listpocket
CMD ["/app/listpocket", "serve", "--config", "/app/config.toml"]
