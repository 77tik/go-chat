package task

import (
	"context"
	"encoding/json"
	"gochat/internal/tools"
	"strings"
	"time"

	"github.com/segmentio/kafka-go"
	"github.com/sirupsen/logrus"
	"gochat/config"
)

// 与 AI Worker 约定的结果格式（保持一致）
type aiResult struct {
	RoomID      int    `json:"roomId"`
	Text        string `json:"text"`
	Op          string `json:"op"`                    // ask/summarize/translate（仅日志用）
	Model       string `json:"model,omitempty"`       // 模型名（可选）
	Err         string `json:"err,omitempty"`         // 出错时非空
	ClientMsgId int64  `json:"clientMsgId,omitempty"` // 可选；不传我会兜底生成
}

// 前端/WS 期望的消息体（跟你群发里 body 的结构一致）
type wsInnerMsg struct {
	Code         int    `json:"code"`
	Msg          string `json:"msg"`
	FromUserId   int    `json:"fromUserId"`
	FromUserName string `json:"fromUserName"`
	ToUserId     int    `json:"toUserId"`
	ToUserName   string `json:"toUserName"`
	RoomId       int    `json:"roomId"`
	Op           int    `json:"op"`                    // config.OpRoomSend = 3
	CreateTime   string `json:"createTime"`            // "YYYY-MM-DD HH:MM:SS"
	ClientMsgId  int64  `json:"clientMsgId,omitempty"` // 可选；不传我会兜底生成
}

// 在 Task 启动时调用一次
func (t *Task) InitAIResultsConsumer() error {
	brokers := strings.Split(config.Conf.Common.CommonKafka.Brokers, ",")
	topic := config.Conf.Common.CommonKafka.AIResultsTopic
	if topic == "" {
		topic = "ai.results"
	}

	r := kafka.NewReader(kafka.ReaderConfig{
		Brokers:  brokers,
		Topic:    topic,
		GroupID:  "gochat-task-ai", // AI 结果消费者组
		MinBytes: 1,
		MaxBytes: 10 << 20,
	})

	go func() {
		defer r.Close()
		logrus.Infof("[AI] results consumer started, topic=%s, brokers=%v", topic, brokers)

		ctx := context.Background()
		for {
			m, err := r.ReadMessage(ctx)
			if err != nil {
				logrus.Warnf("[AI] read result err: %v", err)
				time.Sleep(time.Second)
				continue
			}

			var res aiResult
			if err := json.Unmarshal(m.Value, &res); err != nil {
				logrus.Warnf("[AI] bad json: %s", string(m.Value))
				continue
			}

			// 组装展示文本
			text := res.Text
			if res.Err != "" {
				text = "（AI处理失败）" + res.Err
			} else if res.Model != "" {
				// 可选：尾注模型名；不想展示可删
				text = text + "\n—— " + res.Model
			}

			// 1) 先构造“房间消息”的 payload（与你普通群聊一致）
			if res.ClientMsgId == 0 {
				res.ClientMsgId = tools.GetSnowflakeIdForInt64()
			}
			// 组装 WS 的 body（就是 broadcastRoomToConnect 里要的 []byte）
			body, _ := json.Marshal(wsInnerMsg{
				Code:         0,
				Msg:          text,
				FromUserId:   0,
				FromUserName: "🤖 AI",
				ToUserId:     0,
				ToUserName:   "",
				RoomId:       res.RoomID,
				Op:           config.OpRoomSend, // 3
				CreateTime:   tools.GetNowDateTime(),
				ClientMsgId:  res.ClientMsgId,
			})

			// 2) 先入库（幂等：主键/雪花ID冲突会 DoNothing）
			if t.History != nil {
				if err := t.History.SaveRoomMsgByBytes(body); err != nil {
					logrus.Warnf("[AI] save history fail: %v", err)
				}
			} else {
				logrus.Warn("[AI] history store not initialized")
			}
			// 复用现有广播（内部会遍历所有 connect 客户端 RPC 调 PushRoomMsg）
			t.broadcastRoomToConnect(res.RoomID, body)

			logrus.Infof("[AI] pushed room=%d op=%s bytes=%d", res.RoomID, res.Op, len(body))
		}
	}()

	return nil
}
