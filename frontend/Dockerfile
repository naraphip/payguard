FROM node:24-bookworm-slim AS build
WORKDIR /app
COPY package.json package-lock.json ./
RUN npm ci
COPY . .
ARG PAYGUARD_API_URL=http://backend:8081
ARG PAYGUARD_BOOKING_API_URL=${PAYGUARD_API_URL}
ENV PAYGUARD_API_URL=${PAYGUARD_API_URL}
ENV NEXT_TELEMETRY_DISABLED=1
RUN npm run build
RUN npm prune --omit=dev

FROM node:24-bookworm-slim
WORKDIR /app
ENV NODE_ENV=production
ENV NEXT_TELEMETRY_DISABLED=1
COPY --from=build --chown=node:node /app/ ./
USER node
EXPOSE 3000
CMD ["node", "node_modules/next/dist/bin/next", "start", "--hostname", "0.0.0.0"]
