package logic

import (
	"context"
	"encoding/json"
	"github.com/pkg/errors"
	"strings"
	"time"

	"github.com/segmentio/kafka-go"
	"github.com/sirupsen/logrus"
	"gochat/config"
)

var ErrAIProducerNotInit = errors.New("ai kafka producer not initialized")
var aiWriter *kafka.Writer

// 在 Logic 模块启动时调用一次（见第 4 步）
func (l *Logic) InitAIKafkaProducer() error {
	brokers := strings.Split(config.Conf.Common.CommonKafka.Brokers, ",")
	aiWriter = &kafka.Writer{
		Addr:                   kafka.TCP(brokers...),
		Topic:                  config.Conf.Common.CommonKafka.AIJobsTopic,
		AllowAutoTopicCreation: true,
		BatchTimeout:           10 * time.Millisecond,
	}
	logrus.Infof("AI CommonKafka producer ready, topic=%s", config.Conf.Common.CommonKafka.AIJobsTopic)
	return nil
}

// 与 AI worker 约定的任务格式（保持与 worker 的 Job 一致）
type AIJob struct {
	Op         string `json:"op"` // "ask" | "summarize" | "translate"
	RoomID     int    `json:"roomId"`
	FromUserID int    `json:"fromUserId"`
	FromName   string `json:"fromUserName"`
	Prompt     string `json:"prompt"` // /ai 的问题；/translate 的文本；/summarize 可留空
	Lang       string `json:"lang"`   // translate 目标语言
}

func (l *Logic) KafkaPublishAIJob(job *AIJob) error {
	if aiWriter == nil {
		return ErrAIProducerNotInit
	}
	b, _ := json.Marshal(job)
	return aiWriter.WriteMessages(context.Background(), kafka.Message{Value: b})
}
