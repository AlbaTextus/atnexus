# syntax=docker/dockerfile:1
FROM node:22-alpine

WORKDIR /app
RUN corepack enable

COPY package.json pnpm-lock.yaml pnpm-workspace.yaml ./
COPY web/package.json ./web/package.json
RUN pnpm install --frozen-lockfile --ignore-scripts

COPY web ./web
ARG OAUTH_BACKEND_URL=http://oauth:8080
ARG NEXT_PUBLIC_WIKI_URL=http://localhost:3200
ENV OAUTH_BACKEND_URL=$OAUTH_BACKEND_URL
ENV NEXT_PUBLIC_WIKI_URL=$NEXT_PUBLIC_WIKI_URL
ARG NEXT_PUBLIC_CREDIT_URL=http://localhost:3300
ENV NEXT_PUBLIC_CREDIT_URL=$NEXT_PUBLIC_CREDIT_URL
RUN pnpm web:build

EXPOSE 3288
CMD ["pnpm", "web:start"]
