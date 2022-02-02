#!/bin/bash
docker buildx create --use --name mybuilder
docker buildx build --tag scjtqs/bot_app:chat --platform linux/amd64,linux/arm64,linux/386,linux/arm/v7 --push  .
docker buildx rm mybuilder