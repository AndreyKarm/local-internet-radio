FROM node:22-alpine AS build
RUN corepack enable && corepack prepare pnpm@latest --activate
WORKDIR /app

ENV CI=true

COPY package.json pnpm-lock.yaml ./

RUN pnpm config set minimum-release-age 0
RUN pnpm install --frozen-lockfile

COPY . .

ARG PUBLIC_RADIO_URL
ENV PUBLIC_RADIO_URL=$PUBLIC_RADIO_URL

RUN pnpm run build

FROM node:22-alpine
WORKDIR /app

COPY --from=build /app/build ./build
COPY --from=build /app/package.json ./
COPY --from=build /app/pnpm-lock.yaml ./

RUN corepack enable && corepack prepare pnpm@latest --activate  
RUN pnpm config set minimum-release-age 0
RUN pnpm install --prod --frozen-lockfile

EXPOSE 3000
CMD ["node", "build"]