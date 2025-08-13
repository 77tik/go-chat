package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	"github.com/segmentio/kafka-go"
	"gochat/api/rpc"
	"gochat/config"
	proto2 "gochat/internal/proto"
	"gochat/internal/tools"
	"strings"
	"time"
)

type FormAISummarize struct {
	AuthToken string `json:"authToken" binding:"required"`
	RoomId    int    `json:"roomId" binding:"required"`
	Limit     int    `json:"limit"` // 默认80-200较合适
}

func AISummarizeRoom(c *gin.Context) {
	var form FormAISummarize
	if err := c.ShouldBindBodyWith(&form, binding.JSON); err != nil {
		tools.FailWithMsg(c, err.Error())
		return
	}
	if form.Limit <= 0 || form.Limit > 500 {
		form.Limit = 120
	}

	// 拿用户信息（中间件已验token，这里如果需要用户名可再取一次）
	code, uid, uname := rpc.RpcLogicObj.CheckAuth(&proto2.CheckAuthRequest{AuthToken: form.AuthToken})
	if code == tools.CodeFail || uid <= 0 {
		tools.FailWithMsg(c, "auth fail")
		return
	}

	// 拉历史
	lr := &proto2.ListMessagesRequest{RoomId: form.RoomId, Limit: form.Limit}
	code, list, msg := rpc.RpcLogicObj.ListRoomMessages(lr)
	if code == tools.CodeFail {
		tools.FailWithMsg(c, "list history fail: "+msg)
		return
	}

	// 拼 prompt
	var b strings.Builder
	// 简短的系统引导，告诉模型要点式总结（可按需调整/多语言）
	b.WriteString("以下是聊天室最近的对话，请用中文生成要点式总结：\n")
	for _, m := range list {
		// 取时分秒展示
		ts := m.CreateTime
		if len(ts) >= 8 {
			ts = ts[len(ts)-8:]
		}
		fmt.Fprintf(&b, "[%s] %s: %s\n", ts, m.FromUserName, m.Content)
	}
	prompt := b.String()

	// 投递到 ai.jobs
	job := map[string]any{
		"op":           "summarize",
		"roomId":       form.RoomId,
		"fromUserId":   uid,
		"fromUserName": uname,
		"prompt":       prompt,
		"requestTime":  time.Now().Format("2006-01-02 15:04:05"),
	}
	payload, _ := json.Marshal(job)

	w := &kafka.Writer{
		Addr:     kafka.TCP(strings.Split(config.Conf.Common.CommonKafka.Brokers, ",")...),
		Topic:    config.Conf.Common.CommonKafka.AIJobsTopic,
		Balancer: &kafka.LeastBytes{},
	}
	defer w.Close()

	if err := w.WriteMessages(context.Background(), kafka.Message{
		Key:   []byte(fmt.Sprintf("room-%d", form.RoomId)),
		Value: payload,
	}); err != nil {
		tools.FailWithMsg(c, "enqueue ai job fail: "+err.Error())
		return
	}

	tools.SuccessWithMsg(c, "ok", "已提交AI总结任务")
}
