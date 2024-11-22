#!/bin/bash
docker build --rm --tag scjtqs/bot_app:chat-test  -f cn.Dockerfile .
docker push scjtqs/bot_app:chat-test