package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	log "github.com/sirupsen/logrus"
	"github.com/tidwall/gjson"
	"io"
	"net/http"
	"os"
	"time"
)

// 图灵收费机器人 API
var TulingKey string
var tulingApi = "http://openapi.tuling123.com/openapi/api/v2"

func init()  {
	TulingKey = os.Getenv("TULING_KEY")
}

var tulingErrcode = []int64{4000, 4001, 4002, 4003, 4004, 4005, 4006, 4007, 4100, 4200, 4300, 4400, 4500, 4600, 4602, 5000, 6000, 77002, 8008}

func post(postURL string, data MSG) ([]byte, error) {
	var res *http.Response
	body, _ := json.Marshal(data)
	client := http.Client{Timeout: time.Second * 2}
	header := make(http.Header)
	header.Set("Content-Type", "application/json")
	req, err := http.NewRequest("POST", postURL, bytes.NewReader(body))
	if err != nil {
		log.Warnf("发送数据到 %s 时创建请求失败: %v", postURL, err)
		return nil, err
	}
	req.Header = header
	res, err = client.Do(req)
	if res != nil {
		//goland:noinspection GoDeferInLoop
		defer res.Body.Close()
	}
	if err != nil {
		return nil, err
	}
	r, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}
	return r, err
}

// tulingText 字符串匹配
func tulingText(message string, user_id int64, group_id int64) (string, error) {
	postData := MSG{
		"userInfo": MSG{
			"apiKey":  TulingKey,
			"userId":  user_id,
			"groupId": group_id,
		},
		"perception": MSG{
			"inputText": MSG{
				"text": message,
			},
		},
	}
	r, err := post(tulingApi, postData)
	if err != nil {
		return "", err
	}
	result := gjson.ParseBytes(r)
	if inTulingErrcode(result.Get("intent.code").Int()) {
		log.Errorf("图灵机器人返回异常,resp: %s", result.String())
		return "", fmt.Errorf("图灵机器人返回异常,resp: %s", result.String())
	}
	str := ""
	for _, v := range result.Get("results").Array() {
		str += v.Get("values").Get(v.Get("resultType").String()).String()
	}
	return str, err
}

func tulingImage(url string, user_id int64, group_id int64) (string, error) {
	postData := MSG{
		"userInfo": MSG{
			"apiKey":  TulingKey,
			"userId":  user_id,
			"groupId": group_id,
		},
		"perception": MSG{
			"inputImage": MSG{
				"url": url,
			},
		},
	}
	r, err := post(tulingApi, postData)
	if err != nil {
		return "", err
	}
	result := gjson.ParseBytes(r)
	if inTulingErrcode(result.Get("intent.code").Int()) {
		log.Errorf("图灵机器人返回异常,resp: %s", result.String())
		return "", fmt.Errorf("图灵机器人返回异常,resp: %s", result.String())
	}
	str := ""
	for _, v := range result.Get("results").Array() {
		str += v.Get("values").Get(v.Get("resultType").String()).String()
	}
	return str, err
}

func inTulingErrcode(code int64) bool {
	for _, v := range tulingErrcode {
		if code == v {
			return true
		}
	}
	return false
}
