// logic/kafka.go
package logic

import (
	"context"
	"encoding/json"
	"fmt"
	"gochat/proto"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/segmentio/kafka-go"
	"gochat/config"
)

var (
	kafkaWriters sync.Map // topic -> *kafka.Writer  复用连接，避免每次创建
)

// 推荐：按 serverId 切分 Topic，避免所有 Task 都消费到所有消息。
func topicForServer(serverId string) string {
	// 原来用 config.QueueName，当成前缀即可
	return fmt.Sprintf("%s-%s", config.QueueName, serverId) // 例: gochat-queue-srv1
}

func getWriter(topic string) *kafka.Writer {
	if w, ok := kafkaWriters.Load(topic); ok {
		return w.(*kafka.Writer)
	}
	brokers := strings.Split(config.Conf.Common.Kafka.Brokers, ",")
	w := &kafka.Writer{
		Addr:                   kafka.TCP(brokers...),
		Topic:                  topic,
		Balancer:               &kafka.Hash{},    // 用 Key 保证同 key 有序（如同一 userId/roomId）
		RequiredAcks:           kafka.RequireAll, // 落到所有 ISR，强一致
		AllowAutoTopicCreation: true,             // 开发环境可开，生产最好关并预建 Topic
		BatchSize:              100,              // 视吞吐调整
		BatchBytes:             1 << 20,          // 1MB
		BatchTimeout:           10 * time.Millisecond,
	}
	actual, _ := kafkaWriters.LoadOrStore(topic, w)
	return actual.(*kafka.Writer)
}

func closeAllWriters() {
	kafkaWriters.Range(func(_, v any) bool {
		_ = v.(*kafka.Writer).Close()
		return true
	})
}

// 单聊
func (logic *Logic) KafkaPublishChannel(serverId string, toUserId int, msg []byte) error {
	// 仍复用你原来的消息结构
	redisMsg := proto.RedisMsg{
		Op:       config.OpSingleSend,
		ServerId: serverId,
		UserId:   toUserId,
		Msg:      msg,
	}
	payload, err := json.Marshal(redisMsg)
	if err != nil {
		return err
	}

	topic := topicForServer(serverId)
	w := getWriter(topic)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// 用 userId 做 Key：保证同一用户消息在分区内有序
	return w.WriteMessages(ctx, kafka.Message{
		Key:   []byte(fmt.Sprintf("user:%d", toUserId)),
		Value: payload,
		Headers: []kafka.Header{
			{Key: "op", Value: []byte(strconv.Itoa(config.OpSingleSend))},
		},
		Time: time.Now(),
	})
}

// 群聊
func (logic *Logic) KafkaPublishRoomInfo(roomId int, count int, roomUserInfo map[string]string, msg []byte) error {
	redisMsg := &proto.RedisMsg{
		Op:           config.OpRoomSend,
		RoomId:       roomId,
		Count:        count,
		Msg:          msg,
		RoomUserInfo: roomUserInfo,
	}
	payload, err := json.Marshal(redisMsg)
	if err != nil {
		return err
	}

	// 这里建议也带上 serverId（如果你能拿到），并按 serverId 定投到对应 Topic。
	// 如果拿不到 serverId，就投到一个公共 Topic（见下“方案B”），但那样 Task 侧要过滤。
	topic := topicForServer( /* serverId */ config.Conf.Logic.LogicBase.ServerId)
	w := getWriter(topic)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	return w.WriteMessages(ctx, kafka.Message{
		Key:   []byte(fmt.Sprintf("room:%d", roomId)), // 同一房间内消息尽量有序
		Value: payload,
		Headers: []kafka.Header{
			{Key: "op", Value: []byte(strconv.Itoa(config.OpRoomSend))},
		},
		Time: time.Now(),
	})
}

func (logic *Logic) KafkaPushRoomCount(roomId int, count int) error {
	redisMsg := &proto.RedisMsg{
		Op:     config.OpRoomCountSend,
		RoomId: roomId,
		Count:  count,
	}
	payload, err := json.Marshal(redisMsg)
	if err != nil {
		return err
	}

	topic := topicForServer(config.Conf.Logic.LogicBase.ServerId)
	w := getWriter(topic)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	return w.WriteMessages(ctx, kafka.Message{
		Key:   []byte(fmt.Sprintf("room-count:%d", roomId)),
		Value: payload,
		Headers: []kafka.Header{
			{Key: "op", Value: []byte(strconv.Itoa(config.OpRoomCountSend))},
		},
		Time: time.Now(),
	})
}

func (logic *Logic) KafkaPushRoomInfo(roomId int, count int, roomUserInfo map[string]string) error {
	redisMsg := &proto.RedisMsg{
		Op:           config.OpRoomInfoSend,
		RoomId:       roomId,
		Count:        count,
		RoomUserInfo: roomUserInfo,
	}
	payload, err := json.Marshal(redisMsg)
	if err != nil {
		return err
	}

	topic := topicForServer(config.Conf.Logic.LogicBase.ServerId)
	w := getWriter(topic)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	return w.WriteMessages(ctx, kafka.Message{
		Key:   []byte(fmt.Sprintf("room-info:%d", roomId)),
		Value: payload,
		Headers: []kafka.Header{
			{Key: "op", Value: []byte(strconv.Itoa(config.OpRoomInfoSend))},
		},
		Time: time.Now(),
	})
}
