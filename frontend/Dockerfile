FROM node:22-alpine AS build
RUN corepack enable && corepack prepare pnpm@latest --activate
WORKDIR /app

ENV CI=true

COPY package.json pnpm-lock.yaml ./
RUN pnpm config set minimum-release-age 0
RUN pnpm install --frozen-lockfile

COPY . .

ARG VITE_RADIO_URL
ENV VITE_RADIO_URL=$VITE_RADIO_URL

ARG VITE_RADIO_NAME
ENV VITE_RADIO_NAME=$VITE_RADIO_NAME

RUN pnpm run build

RUN pnpm prune --prod

ENV NODE_ENV=production

FROM node:22-alpine
WORKDIR /app

COPY --from=build /app/build ./build
COPY --from=build /app/package.json ./
COPY --from=build /app/pnpm-lock.yaml ./

RUN corepack enable && corepack prepare pnpm@latest --activate  
RUN pnpm config set minimum-release-age 0
RUN pnpm install --prod --frozen-lockfile

USER node

EXPOSE 3000

CMD ["node", "build"]