package main

import (
	"fmt"
	log "github.com/sirupsen/logrus"
	"github.com/tidwall/gjson"
	"net/url"
)

// 青云免费机器人API
var qingyunke_key = "free"
var qingyunke_api = "http://api.qingyunke.com/api.php"

func qingyunkeText(message string, user_id int64, group_id int64) (string, error) {
	URL := fmt.Sprintf("%s?key=%s&appid=0&msg=%s", qingyunke_api, qingyunke_key, url.QueryEscape(message))
	r, err := post(URL, nil)
	if err != nil {
		return "", err
	}
	result := gjson.ParseBytes(r)
	if result.Get("result").Int() != 0 {
		log.Errorf("青云机器人返回异常,resp:%s", result.String())
		return "", fmt.Errorf("青云机器人返回异常,resp:%s", result.String())
	}
	return result.Get("content").String(), nil
}

func qingyunkeImage(message string, user_id int64, group_id int64) (string, error) {
	URL := fmt.Sprintf("%s?key=%s&appid=0&msg=%s", qingyunke_api, qingyunke_key, url.QueryEscape(message))
	r, err := post(URL, nil)
	if err != nil {
		return "", err
	}
	result := gjson.ParseBytes(r)
	if result.Get("result").Int() != 0 {
		log.Errorf("青云机器人返回异常,resp:%s", result.String())
		return "", fmt.Errorf("青云机器人返回异常,resp:%s", result.String())
	}
	return result.Get("content").String(), nil
}
