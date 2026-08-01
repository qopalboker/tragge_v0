# ---- Build ----
FROM node:20-alpine AS builder
RUN corepack enable
WORKDIR /app

# Cache dependencies (changes rarely)
COPY pnpm-lock.yaml pnpm-workspace.yaml package.json ./
COPY apps/user-frontend/package.json apps/user-frontend/
COPY packages/contracts/ts/package.json packages/contracts/ts/
COPY packages/frontend-shared/package.json packages/frontend-shared/
RUN pnpm install --frozen-lockfile 2>/dev/null || \
    (cd apps/user-frontend && sed -i 's|"workspace:\*"|"file:../../packages/contracts/ts"|' package.json && npm install --legacy-peer-deps)

# Build — shared package source must be present before user-frontend builds
COPY packages/contracts/ packages/contracts/
COPY packages/frontend-shared/ packages/frontend-shared/
COPY apps/user-frontend/ apps/user-frontend/
RUN cd apps/user-frontend && npm run build

# ---- Runtime ----
FROM nginx:alpine
COPY --from=builder /app/apps/user-frontend/dist /usr/share/nginx/html
COPY apps/user-frontend/nginx.conf /etc/nginx/nginx.conf

# Run as non-root (match other containers' security posture)
RUN chown -R nginx:nginx /usr/share/nginx/html /var/cache/nginx /var/log/nginx /var/run
USER nginx

EXPOSE 8080
CMD ["nginx", "-g", "daemon off;"]
