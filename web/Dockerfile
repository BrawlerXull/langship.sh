# --- build stage ---
FROM node:22-alpine AS build
WORKDIR /app
COPY package.json package-lock.json* ./
RUN test -f package-lock.json && npm ci --no-fund --no-audit \
    || npm install --no-fund --no-audit
COPY . .
RUN npm run build

# --- runtime: nginx serving the static export, proxying /api to the flow API ---
FROM nginx:1.27-alpine
COPY --from=build /app/out /usr/share/nginx/html
COPY nginx.conf /etc/nginx/conf.d/default.conf
EXPOSE 3000
