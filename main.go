package main

import (
	"github.com/kataras/iris/v12"
	"github.com/scjtqs2/bot_adapter/client"
	"github.com/scjtqs2/bot_adapter/sha256"
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
	app_id             = os.Getenv("APP_ID")
	app_secret         = os.Getenv("APP_SECRET")
	app_encrypt_key    = os.Getenv("APP_ENCRYPT_KEY")
	bot_adapter_addr   = os.Getenv("ADAPTER_ADDR")
	bot_adapter_client *client.AdapterService
)

func setup() {
	var err error
	bot_adapter_client, err = client.NewAdapterServiceClient(bot_adapter_addr, app_id, app_secret)
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

// MSG 消息Map
type MSG map[string]interface{}

func msginput(ctx iris.Context) {
	raw, _ := ctx.GetBody()
	enc := gjson.ParseBytes(raw).Get("encrypt").String()
	// 解密推送数据
	msg, err := sha256.Decrypt(enc, app_encrypt_key)
	if err != nil {
		log.Errorf("解密失败：enc:%s err:%s", enc, err.Error())
	}
	go parseMsg(msg)
	_, _ = ctx.JSON(MSG{
		"code": 200,
		"msg":  "received",
	})
}
