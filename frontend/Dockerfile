# Códice web app: built once, served by nginx, which also forwards API calls to the
# backend so the browser only ever talks to one origin.
FROM node:22-alpine AS build
WORKDIR /app
COPY package.json package-lock.json ./
RUN npm ci
COPY . .
# Empty on purpose: the app calls the API on its own origin (nginx forwards it).
ENV VITE_API_URL=
RUN npm run build

FROM nginxinc/nginx-unprivileged:1-alpine
COPY nginx.conf /etc/nginx/conf.d/default.conf
COPY --from=build /app/dist /usr/share/nginx/html
EXPOSE 8080
HEALTHCHECK --interval=15s --timeout=4s --start-period=10s --retries=5 \
  CMD wget -qO- http://127.0.0.1:8080/ >/dev/null || exit 1
