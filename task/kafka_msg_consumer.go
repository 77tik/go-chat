// task/kafka_consumer.go
package task

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/segmentio/kafka-go"
	"gochat/config"
)

func topicForServer(serverId string) string {
	return fmt.Sprintf("%s-%s", config.QueueName, serverId) // 例: gochat-queue-srv1
}

func (t *Task) InitKafkaConsumer(serverId string) error {
	r := kafka.NewReader(kafka.ReaderConfig{
		Brokers:        strings.Split(config.Conf.Common.CommonKafka.Brokers, ","), // "ip1:9092,ip2:9092"
		GroupID:        "gochat-task-" + serverId,                                  // 同 ServerId 的一组消费者分摊分区
		Topic:          topicForServer(serverId),
		MinBytes:       10e3,        // 10KB
		MaxBytes:       10e6,        // 10MB
		CommitInterval: time.Second, // 自动周期提交；要手动提交可设为0
	})
	// 可在 t.Shutdown() 时 r.Close()

	go func() {
		defer r.Close()
		ctx := context.Background()
		for {
			m, err := r.ReadMessage(ctx)
			if err != nil {
				log.Printf("kafka read error: %v", err)
				time.Sleep(time.Second)
				continue
			}
			// 你原先的 Push 接口吃 string，这里直接复用
			t.Push(string(m.Value))
		}
	}()
	return nil
}
