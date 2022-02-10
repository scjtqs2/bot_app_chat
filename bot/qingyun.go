package bot

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/scjtqs2/bot_adapter/coolq"
	log "github.com/sirupsen/logrus"
	"github.com/tidwall/gjson"
)

// qingyunkeKey 青云免费机器人API
var qingyunkeKey = "free"
var qingyunkeAPI = "http://api.qingyunke.com/api.php"

// QingyunkeText 文字
func QingyunkeText(message string, userID int64, groupID int64) (string, error) {
	msg := coolq.CleanCQCode(message)
	if msg == "" {
		return "", errors.New("empty message")
	}
	URL := fmt.Sprintf("%s?key=%s&appid=0&msg=%s", qingyunkeAPI, qingyunkeKey, url.QueryEscape(msg))
	r, err := get(URL)
	if err != nil {
		log.Errorf("qingyunke post err:%v", err)
		return "", err
	}
	result := gjson.ParseBytes(r)
	if result.Get("result").Int() != 0 {
		log.Errorf("青云机器人返回异常,resp:%s", result.String())
		return "", errors.New("青云机器人返回异常,resp:" + result.String())
	}
	return parseQingyunkeContext(result.Get("content").String()), nil
}

func get(getURL string) ([]byte, error) {
	var res *http.Response
	client := http.Client{Timeout: time.Second * 4}
	header := make(http.Header)
	req, err := http.NewRequest("GET", getURL, nil)
	if err != nil {
		log.Warnf("发送数据到 %s 时创建请求失败: %v", getURL, err)
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

func parseQingyunkeContext(context string) string {
	// {face:81}
	patten1 := `\{face:(\d+)\}`
	r1, _ := regexp.Compile(patten1)
	context = r1.ReplaceAllString(context, "[CQ:face,id=$1]")
	return strings.ReplaceAll(context, "{br}", "\n")
}

// QingyunkeImage 图片
func QingyunkeImage(message string, userID int64, groupID int64) (string, error) {
	URL := fmt.Sprintf("%s?key=%s&appid=0&msg=%s", qingyunkeAPI, qingyunkeKey, url.QueryEscape(message))
	r, err := post(URL, nil)
	if err != nil {
		return "", err
	}
	result := gjson.ParseBytes(r)
	if result.Get("result").Int() != 0 {
		log.Errorf("青云机器人返回异常,resp:%s", result.String())
		return "", errors.New("青云机器人返回异常,resp:" + result.String())
	}
	return result.Get("content").String(), nil
}
