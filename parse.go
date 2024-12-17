package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/scjtqs2/bot_adapter/coolq"
	"github.com/scjtqs2/bot_adapter/event"
	"github.com/scjtqs2/bot_adapter/pb/entity"
	log "github.com/sirupsen/logrus"
	"github.com/tidwall/gjson"

	"github.com/scjtqs2/bot_app_chat/bot"
)

func parseMsg(data string) {
	msg := gjson.Parse(data)
	switch msg.Get("post_type").String() {
	case "message": // 消息事件
		switch msg.Get("message_type").String() {
		case event.MESSAGE_TYPE_PRIVATE:
			var req event.MessagePrivate
			_ = json.Unmarshal([]byte(msg.Raw), &req)
			ok := false
			if bot.OpenaiEndpoint != "" && bot.OpenaiApiKey != "" {
				ok = chatgpt(req.RawMessage, req.UserID, 0, false, req.SelfID)
			}
			if bot.TulingKey != "" && !ok {
				ok = tuling(req.RawMessage, req.UserID, 0, false, req.SelfID)
			}
			if !ok {
				ok = qingyunke(req.RawMessage, req.UserID, 0, false, req.SelfID)
			}
			log.Debug(ok)
		case event.MESSAGE_TYPE_GROUP:
			var req event.MessageGroup
			_ = json.Unmarshal([]byte(msg.Raw), &req)
			ok := false
			if bot.OpenaiEndpoint != "" && bot.OpenaiApiKey != "" {
				ok = chatgpt(req.RawMessage, req.UserID, 0, false, req.SelfID)
			}
			if bot.TulingKey != "" && !ok {
				ok = tuling(req.RawMessage, req.Sender.UserID, req.GroupID, true, req.SelfID)
			}
			if !ok {
				ok = qingyunke(req.RawMessage, req.Sender.UserID, req.GroupID, true, req.SelfID)
			}
			log.Debug(ok)
		}
	case "notice": // 通知事件
		switch msg.Get("notice_type").String() {
		case event.NOTICE_TYPE_FRIEND_ADD:
			var req event.NoticeFriendAdd
			_ = json.Unmarshal([]byte(msg.Raw), &req)
		case event.NOTICE_TYPE_FRIEND_RECALL:
			var req event.NoticeFriendRecall
			_ = json.Unmarshal([]byte(msg.Raw), &req)
		case event.NOTICE_TYPE_GROUP_BAN:
			var req event.NoticeGroupBan
			_ = json.Unmarshal([]byte(msg.Raw), &req)
		case event.NOTICE_TYPE_GROUP_DECREASE:
			var req event.NoticeGroupDecrease
			_ = json.Unmarshal([]byte(msg.Raw), &req)
		case event.NOTICE_TYPE_GROUP_INCREASE:
			var req event.NoticeGroupIncrease
			_ = json.Unmarshal([]byte(msg.Raw), &req)
		case event.NOTICE_TYPE_GROUP_ADMIN:
			var req event.NoticeGroupAdmin
			_ = json.Unmarshal([]byte(msg.Raw), &req)
		case event.NOTICE_TYPE_GROUP_RECALL:
			var req event.NoticeGroupRecall
			_ = json.Unmarshal([]byte(msg.Raw), &req)
		case event.NOTICE_TYPE_GROUP_UPLOAD:
			var req event.NoticeGroupUpload
			_ = json.Unmarshal([]byte(msg.Raw), &req)
		case event.NOTICE_TYPE_POKE:
			var req event.NoticePoke
			_ = json.Unmarshal([]byte(msg.Raw), &req)
		case event.NOTICE_TYPE_HONOR:
			var req event.NoticeHonor
			_ = json.Unmarshal([]byte(msg.Raw), &req)
		case event.NOTICE_TYPE_LUCKY_KING:
			var req event.NoticeLuckyKing
			_ = json.Unmarshal([]byte(msg.Raw), &req)
		case event.CUSTOM_NOTICE_TYPE_GROUP_CARD:
		case event.CUSTOM_NOTICE_TYPE_OFFLINE_FILE:
		}
	case "request": // 请求事件
		switch msg.Get("request_type").String() {
		case event.REQUEST_TYPE_FRIEND:
			var req event.RequestFriend
			_ = json.Unmarshal([]byte(msg.Raw), &req)
		case event.REQUEST_TYPE_GROUP:
			var req event.RequestGroup
			_ = json.Unmarshal([]byte(msg.Raw), &req)
		}
	case "meta_event": // 元事件
		switch msg.Get("meta_event_type").String() {
		case event.META_EVENT_LIFECYCLE:
			var req event.MetaEventLifecycle
			_ = json.Unmarshal([]byte(msg.Raw), &req)
		case event.META_EVENT_HEARTBEAT:
			var req event.MetaEventHeartbeat
			_ = json.Unmarshal([]byte(msg.Raw), &req)
		}
	}
}

// tuling 图灵机器人聊天
func tuling(message string, userID int64, groupID int64, isGroup bool, bootID int64) bool {
	if !isGroup {
		// 私聊
		text, err := bot.TulingText(message, userID, groupID)
		if err != nil || text == "" {
			log.Errorf("tuling msg error:%v", err)
			return false
		}
		_, _ = botAdapterClient.SendPrivateMsg(context.TODO(), &entity.SendPrivateMsgReq{
			UserId:  userID,
			Message: []byte(text),
		})
		return true
	}
	var msg string

	if strings.HasPrefix(message, "#") {
		msg = strings.Replace(message, "#", "", 1)
	}
	if ok, _ := coolq.IsAtMe(message, bootID); ok {
		msg = strings.ReplaceAll(message, coolq.EnAtCode(fmt.Sprintf("%d", bootID)), "")
	}
	if msg != "" {
		text, err := bot.TulingText(msg, userID, groupID)
		if err != nil || text == "" {
			log.Errorf("tuling msg error:%v", err)
			return false
		}
		_, _ = botAdapterClient.SendGroupMsg(context.TODO(), &entity.SendGroupMsgReq{
			GroupId: groupID,
			Message: []byte(fmt.Sprintf("%s%s", coolq.EnAtCode(fmt.Sprintf("%d", userID)), text)),
		})
		return true
	}
	return false
}

// qingyunke 青云客机器人聊天
func qingyunke(message string, userID int64, groupID int64, isGroup bool, bootID int64) bool {
	if !isGroup {
		// 私聊
		text, err := bot.QingyunkeText(message, userID, groupID)
		if err != nil || text == "" {
			log.Errorf("qingyunke msg error:%v", err)
			return false
		}
		_, _ = botAdapterClient.SendPrivateMsg(context.TODO(), &entity.SendPrivateMsgReq{
			UserId:  userID,
			Message: []byte(text),
		})
		return true
	}
	var msg string

	if strings.HasPrefix(message, "#") {
		msg = strings.Replace(message, "#", "", 1)
	}
	if ok, _ := coolq.IsAtMe(message, bootID); ok {
		msg = strings.ReplaceAll(message, coolq.EnAtCode(fmt.Sprintf("%d", bootID)), "")
	}
	if msg != "" {
		text, err := bot.QingyunkeText(msg, userID, groupID)
		if err != nil || text == "" {
			log.Errorf("qingyunke msg error:%v", err)
			return false
		}
		_, _ = botAdapterClient.SendGroupMsg(context.TODO(), &entity.SendGroupMsgReq{
			GroupId: groupID,
			Message: []byte(fmt.Sprintf("%s%s", coolq.EnAtCode(fmt.Sprintf("%d", userID)), text)),
		})
		return true
	}
	return false
}

// chatgpt chatgpt聊天
func chatgpt(message string, userID int64, groupID int64, isGroup bool, bootID int64) bool {
	if !isGroup {
		// 私聊
		text, err := bot.ChatGptText(message, userID, groupID, botAdapterClient)
		if err != nil || text == "" {
			log.Errorf("chatgpt msg error:%v", err)
			return false
		}
		_, _ = botAdapterClient.SendPrivateMsg(context.TODO(), &entity.SendPrivateMsgReq{
			UserId:  userID,
			Message: []byte(text),
		})
		return true
	}
	var msg string

	if strings.HasPrefix(message, "#") {
		msg = strings.Replace(message, "#", "", 1)
	}
	if ok, _ := coolq.IsAtMe(message, bootID); ok {
		msg = strings.ReplaceAll(message, coolq.EnAtCode(fmt.Sprintf("%d", bootID)), "")
	}
	if msg != "" {
		text, err := bot.ChatGptText(msg, userID, groupID, botAdapterClient)
		if err != nil || text == "" {
			log.Errorf("chatgpt msg error:%v", err)
			return false
		}
		_, _ = botAdapterClient.SendGroupMsg(context.TODO(), &entity.SendGroupMsgReq{
			GroupId: groupID,
			Message: []byte(fmt.Sprintf("%s%s", coolq.EnAtCode(fmt.Sprintf("%d", userID)), text)),
		})
		return true
	}
	return false
}
