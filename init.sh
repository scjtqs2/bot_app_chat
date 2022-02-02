#!/bin/sh
if [ -f /data/install.lock ];then
    touch /data/install.lock
else
     cp -f /usr/bin/bot_app /data/bot_app
fi
if [ "$UPDATE" = "1" ];then
  cp -f /usr/bin/bot_app /data/bot_app
fi
touch /data/install.lock
chmod +x /data/bot_app
/data/bot_app