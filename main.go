package main

import (
	"github.com/kataras/iris/v12"
	"github.com/scjtqs2/bot_adapter/client"
	"github.com/scjtqs2/bot_adapter/sha256"
	"github.com/scjtqs2/bot_app_chat/bot"
	log "github.com/sirupsen/logrus"
	"github.com/tidwall/gjson"
	"os"
	"os/signal"
)

func main() {
	setup()
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt)
	<-quit
	os.Exit(1)
}

var (
	appID            = os.Getenv("APP_ID")
	appSecret        = os.Getenv("APP_SECRET")
	appEncryptKey    = os.Getenv("APP_ENCRYPT_KEY")
	botAdapterAddr   = os.Getenv("ADAPTER_ADDR")
	botAdapterClient *client.AdapterService
)

func setup() {
	var err error
	botAdapterClient, err = client.NewAdapterServiceClient(botAdapterAddr, appID, appSecret)
	if err != nil {
		log.Fatalf("faild to init grpc client err:%v", err)
	}
	app := iris.New()
	app.Post("/", msginput)
	go func() {
		port := "8080"
		if os.Getenv("HTTP_PORT") != "" {
			port = os.Getenv("HTTP_PORT")
		}
		err = app.Run(iris.Addr(":" + port))
		if err != nil {
			log.Fatalf("error init http listen port %s err:%v", port, err)
		}
	}()
}


func msginput(ctx iris.Context) {
	raw, _ := ctx.GetBody()
	enc := gjson.ParseBytes(raw).Get("encrypt").String()
	// 解密推送数据
	msg, err := sha256.Decrypt(enc, appEncryptKey)
	if err != nil {
		log.Errorf("解密失败：enc:%s err:%s", enc, err.Error())
	}
	go parseMsg(msg)
	_, _ = ctx.JSON(bot.MSG{
		"code": 200,
		"msg":  "received",
	})
}
