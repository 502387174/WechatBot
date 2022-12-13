package handlers

import (
	"errors"
	"fmt"
	"github.com/869413421/wechatbot/config"
	"github.com/869413421/wechatbot/gpt"
	"github.com/869413421/wechatbot/pkg/logger"
	"github.com/869413421/wechatbot/service"
	"github.com/coocood/freecache"
	"github.com/eatmoreapple/openwechat"
	"strconv"
	"strings"
)

var _ MessageHandlerInterface = (*UserMessageHandler)(nil)
var Bot *openwechat.Bot

// 缓存大小，100M
var userRequestCache = freecache.NewCache(100 * 1024 * 1024)

// UserMessageHandler 私聊消息处理
type UserMessageHandler struct {
	// 接收到消息
	msg *openwechat.Message
	// 发送的用户
	sender *openwechat.User
	// 实现的用户业务
	service service.UserServiceInterface
}

// NewUserMessageHandler 创建私聊处理器
func NewUserMessageHandler(message *openwechat.Message) (MessageHandlerInterface, error) {
	sender, err := message.Sender()
	if err != nil {
		return nil, err
	}
	userService := service.NewUserService(c, sender)
	handler := &UserMessageHandler{
		msg:     message,
		sender:  sender,
		service: userService,
	}

	return handler, nil
}

// handle 处理消息
func (h *UserMessageHandler) handle() error {
	if h.msg.IsText() {
		return h.ReplyText()
	}
	return nil
}

func SetBot(b *openwechat.Bot) {
	Bot = b
}

// 所有好友
func getFriends() (f openwechat.Friends) {
	// 获取登陆的用户
	bot := Bot
	self, _ := bot.GetCurrentUser()
	f, _ = self.Friends()
	return f
}

// ReplyText 发送文本消息到群
func (h *UserMessageHandler) ReplyText() error {
	logger.Info(fmt.Sprintf("Received User %v Text Msg : %v", h.sender.NickName, h.msg.Content))
	// 1.获取上下文，如果字符串为空不处理
	requestText := h.getRequestText()
	if requestText == "" {
		logger.Info("user message is null")
		return nil
	}
	if strings.Contains(requestText, "清除用户缓存") && h.sender.NickName == "锐" {
		split := strings.Split(requestText, ":")
		key := []byte(split[1])
		friends := getFriends()
		for _, friend := range friends {
			if friend.NickName == split[1] {
				logger.Info("清除用户缓存:", split[1])
				userRequestCache.Del(key)
			}
		}
		_, err := h.msg.ReplyText("清除用户" + split[1] + "缓存成功")
		return err
	}
	userName := h.sender.UserName
	key := []byte(userName)
	got, _ := userRequestCache.Get(key)
	num := 0
	//存在次数
	atoi, err := strconv.Atoi(string(got))
	num = atoi
	if num >= 3 {
		_, err = h.msg.ReplyText("感谢您的体验，每个用户体验10次，一小时后重置")
		if err != nil {
			return errors.New(fmt.Sprintf("已经超过体验次数: %v ", num))
		}
		return err
	}
	fmt.Printf("%v用户已经体验次数: %v ", h.sender.NickName, num)
	// 2.向GPT发起请求，如果回复文本等于空,不回复
	reply, err := gpt.Completions(h.getRequestText())
	if err != nil {
		// 2.1 将GPT请求失败信息输出给用户，省得整天来问又不知道日志在哪里。
		errMsg := fmt.Sprintf("gpt request error: %v", err)
		_, err = h.msg.ReplyText(errMsg)
		if err != nil {
			return errors.New(fmt.Sprintf("response user error: %v ", err))
		}
		return err
	}
	// 2.设置上下文，回复用户
	h.service.SetUserSessionContext(h.msg.Content, reply)
	_, err = h.msg.ReplyText(buildUserReply(reply))
	if err != nil {
		return errors.New(fmt.Sprintf("response user error: %v ", err))
	}
	//过期时间
	expire := 60 * 60 // expire in 60*60 seconds
	// 设置KEY
	userRequestCache.Set(key, []byte(strconv.Itoa(num+1)), expire)
	// 3.返回错误
	return err
}

// getRequestText 获取请求接口的文本，要做一些清晰
func (h *UserMessageHandler) getRequestText() string {
	// 1.去除空格以及换行
	text := strings.TrimSpace(h.msg.Content)
	text = strings.Trim(h.msg.Content, "\n")

	// 2.获取上下文，拼接在一起，如果字符长度超出4000，截取为4000。（GPT按字符长度算）
	requestText := h.service.GetUserSessionContext() + text
	if len(requestText) >= 4000 {
		requestText = requestText[:4000]
	}

	// 3.返回请求文本
	return requestText
}

// buildUserReply 构建用户回复
func buildUserReply(reply string) string {
	// 1.去除空格问号以及换行号，如果为空，返回一个默认值提醒用户
	textSplit := strings.Split(reply, "\n\n")
	if len(textSplit) > 1 {
		trimText := textSplit[0]
		reply = strings.Trim(reply, trimText)
	}
	reply = strings.TrimSpace(reply)

	reply = strings.TrimSpace(reply)
	if reply == "" {
		return "请求得不到任何有意义的回复，请具体提出问题。"
	}

	// 2.如果用户有配置前缀，加上前缀
	reply = config.LoadConfig().ReplyPrefix + "\n" + reply
	reply = strings.Trim(reply, "\n")

	// 3.返回拼接好的字符串
	return reply
}
