FROM node:22-alpine

WORKDIR /app
RUN corepack enable

COPY package.json pnpm-lock.yaml pnpm-workspace.yaml ./
COPY web/package.json ./web/package.json
RUN pnpm install --frozen-lockfile --ignore-scripts

COPY web ./web
ARG OAUTH_BACKEND_URL=http://oauth:8080
ENV OAUTH_BACKEND_URL=$OAUTH_BACKEND_URL
RUN pnpm web:build

EXPOSE 3000
CMD ["pnpm", "web:start"]
