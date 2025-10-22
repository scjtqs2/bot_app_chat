FROM golang:alpine AS builder

RUN apk add --no-cache git \
  && go env -w GO111MODULE=auto \
  && go env -w CGO_ENABLED=0

WORKDIR /build

COPY ./ .

RUN set -ex \
    && BUILD=`date +%FT%T%z` \
    && COMMIT_SHA1=`git rev-parse HEAD` \
    && go build -ldflags "-s -w -extldflags '-static' -X main.Version=${COMMIT_SHA1}|${BUILD}" -o bot_app


FROM alpine AS production


RUN apk add --no-cache tzdata \
    && cp /usr/share/zoneinfo/Asia/Shanghai /etc/localtime \
    && echo "Asia/Shanghai" > /etc/timezone

ENV UPDATE="1"
ENV HTTP_PORT="8080"
# 推送解密的密码
ENV APP_ENCRYPT_KEY=""
# APPID
ENV APP_ID=""
# APPSECRET
ENV APP_SECRET=""
ENV ADAPTER_ADDR="bot-adapter:8001"
ENV TULING_KEY=""

COPY ./init.sh /
COPY --from=builder /build/bot_app /usr/bin/bot_app
RUN chmod +x /usr/bin/bot_app && chmod +x /init.sh

WORKDIR /data

ENTRYPOINT [ "/init.sh" ]