// Package bot 机器人
package bot

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/scjtqs2/bot_adapter/coolq"
	log "github.com/sirupsen/logrus"
	"github.com/tidwall/gjson"
)

// TulingKey 图灵收费机器人 API
var TulingKey string
var tulingAPI = "http://openapi.tuling123.com/openapi/api/v2"

func init() {
	TulingKey = os.Getenv("TULING_KEY")
	log.Warnf("TulingKey:%s", TulingKey)
}

var tulingErrcode = []int64{4000, 4001, 4002, 4003, 4004, 4005, 4006, 4007, 4100, 4200, 4300, 4400, 4500, 4600, 4602, 5000, 6000, 77002, 8008}

func post(postURL string, data MSG) ([]byte, error) {
	var res *http.Response
	body, _ := json.Marshal(data)
	client := http.Client{Timeout: time.Second * 8}
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

// TulingText 字符串匹配
func TulingText(message string, userID int64, groupID int64) (string, error) {
	msg := coolq.CleanCQCode(message)
	if msg == "" {
		return "", errors.New("empty message")
	}
	postData := MSG{
		"userInfo": MSG{
			"apiKey":  TulingKey,
			"userId":  userID,
			"groupId": groupID,
		},
		"perception": MSG{
			"inputText": MSG{
				"text": msg,
			},
		},
	}
	r, err := post(tulingAPI, postData)
	if err != nil {
		return "", err
	}
	result := gjson.ParseBytes(r)
	if inTulingErrcode(result.Get("intent.code").Int()) {
		log.Errorf("图灵机器人返回异常,resp: %s", result.String())
		return "", errors.New("图灵机器人返回异常,resp:" + result.String())
	}
	str := ""
	for _, v := range result.Get("results").Array() {
		str += v.Get("values").Get(v.Get("resultType").String()).String()
	}
	return str, err
}

// TulingImage 图片
func TulingImage(url string, userID int64, groupID int64) (string, error) {
	postData := MSG{
		"userInfo": MSG{
			"apiKey":  TulingKey,
			"userId":  userID,
			"groupId": groupID,
		},
		"perception": MSG{
			"inputImage": MSG{
				"url": url,
			},
		},
	}
	r, err := post(tulingAPI, postData)
	if err != nil {
		return "", err
	}
	result := gjson.ParseBytes(r)
	if inTulingErrcode(result.Get("intent.code").Int()) {
		log.Errorf("图灵机器人返回异常,resp: %s", result.String())
		return "", errors.New("图灵机器人返回异常,resp:" + result.String())
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
