FROM node:24-alpine AS frontend-builder
WORKDIR /src
COPY package.json package-lock.json ./
COPY frontend/apps/admin/package.json frontend/apps/admin/package.json
COPY frontend/apps/viewer/package.json frontend/apps/viewer/package.json
RUN npm ci
COPY frontend ./frontend
RUN npm run build:ui

FROM golang:1.26-alpine AS backend-builder
WORKDIR /src/backend
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend ./
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/bangumi-pipeline ./cmd/server

FROM alpine:3.23
RUN addgroup -S bangumipipeline && adduser -S -G bangumipipeline bangumipipeline \
    && mkdir -p /app/data && chown -R bangumipipeline:bangumipipeline /app
WORKDIR /app
COPY --from=backend-builder /out/bangumi-pipeline /app/bangumi-pipeline
COPY --from=frontend-builder /src/frontend/apps/admin/dist /app/frontend/apps/admin/dist
COPY --from=frontend-builder /src/frontend/apps/viewer/dist /app/frontend/apps/viewer/dist
ENV BP_ADMIN_ADDR=:8080 \
    BP_VIEWER_ADDR=:8090 \
    BP_ADMIN_WEB_DIR=/app/frontend/apps/admin/dist \
    BP_VIEWER_WEB_DIR=/app/frontend/apps/viewer/dist \
    BP_COVER_DIR=/app/data/images/bangumi \
    BP_BANGUMI_API_URL=https://api.bgm.tv
EXPOSE 8080 8090
USER bangumipipeline
ENTRYPOINT ["/app/bangumi-pipeline"]
