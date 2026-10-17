# syntax=docker/dockerfile:1
FROM node:24-alpine AS build
WORKDIR /app
COPY package.json package-lock.json ./
RUN npm ci
COPY . .

RUN npm run build

FROM node:24-alpine
WORKDIR /app
ENV NODE_ENV=production NEXT_TELEMETRY_DISABLED=1 HOSTNAME=0.0.0.0 PORT=3100
RUN adduser -D -u 10001 admin
COPY --from=build --chown=admin /app/.next/standalone ./
COPY --from=build --chown=admin /app/.next/static ./.next/static
COPY --from=build --chown=admin /app/public ./public
USER admin
EXPOSE 3100
CMD ["node", "server.js"]
