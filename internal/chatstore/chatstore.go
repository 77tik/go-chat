package chatstore

import (
	"context"
	"encoding/json"
	"gochat/internal/tools"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// =============== 模型 ===============

type ChatMessage struct {
	ID           int64     `gorm:"primaryKey;column:id"`
	RoomID       int       `gorm:"column:room_id"`
	FromUserID   int       `gorm:"column:from_user_id"`
	FromUserName string    `gorm:"column:from_user_name"`
	Content      string    `gorm:"column:content"`
	Op           int       `gorm:"column:op"`
	CreatedAt    time.Time `gorm:"column:created_at"` // 存 UTC（建议）
}

// =============== Store ===============

type Store struct {
	DB *gorm.DB
}

func New(db *gorm.DB) *Store {
	return &Store{DB: db}
}

func (s *Store) AutoMigrate() error {
	if err := s.DB.AutoMigrate(&ChatMessage{}); err != nil {
		return err
	}
	return s.DB.Exec(`CREATE INDEX IF NOT EXISTS idx_chat_message_room_time ON chat_message(room_id, created_at)`).Error
}

// =============== 入库（房间消息） ===============

type RoomMsgPayload struct {
	Msg          string `json:"msg"`
	FromUserId   int    `json:"fromUserId"`
	FromUserName string `json:"fromUserName"`
	RoomId       int    `json:"roomId"`
	Op           int    `json:"op"`
	CreateTime   string `json:"createTime"`  // "YYYY-MM-DD HH:MM:SS"（本地）
	ClientMsgId  int64  `json:"clientMsgId"` // 建议由上游生成
}

// SaveRoomMsgRaw: 直接吃 Kafka/队列里的 JSON（和你现有结构对齐）
func (s *Store) SaveRoomMsgRaw(raw []byte) error {
	var p RoomMsgPayload
	if err := json.Unmarshal(raw, &p); err != nil {
		return err
	}
	return s.SaveRoomMsg(p)
}

// SaveRoomMsg: 显式入库（便于单元测试）
func (s *Store) SaveRoomMsg(p RoomMsgPayload) error {
	// 解析时间：本地 -> UTC
	ts := time.Now().UTC()
	if len(p.CreateTime) >= 19 {
		if t, err := time.ParseInLocation("2006-01-02 15:04:05", p.CreateTime, time.Local); err == nil {
			ts = t.UTC()
		}
	}
	id := p.ClientMsgId
	if id == 0 {
		// 不依赖外部工具，兜底用纳秒；生产上仍建议上游传 ClientMsgId
		id = time.Now().UnixNano()
	}
	rec := ChatMessage{
		ID:           id,
		RoomID:       p.RoomId,
		FromUserID:   p.FromUserId,
		FromUserName: p.FromUserName,
		Content:      p.Msg,
		Op:           p.Op,
		CreatedAt:    ts,
	}
	// 幂等：主键冲突忽略
	return s.DB.Clauses(clause.OnConflict{DoNothing: true}).Create(&rec).Error
}

func (s *Store) SaveRoomMsgByBytes(raw []byte) error {
	var p RoomMsgPayload
	if err := json.Unmarshal(raw, &p); err != nil {
		return err
	}
	// 解析时间（你的 createTime 是本地时区；转成 UTC 存）
	ts := time.Now().UTC()
	if len(p.CreateTime) >= 19 {
		if t, err := time.ParseInLocation("2006-01-02 15:04:05", p.CreateTime, time.Local); err == nil {
			ts = t.UTC()
		}
	}
	id := p.ClientMsgId
	if id == 0 {
		id = tools.GetSnowflakeIdForInt64() // 没带就兜底生成，但推荐从 Logic 带过来
	}
	rec := ChatMessage{
		ID:           id,
		RoomID:       p.RoomId,
		FromUserID:   p.FromUserId,
		FromUserName: p.FromUserName,
		Content:      p.Msg,
		Op:           p.Op,
		CreatedAt:    ts,
	}
	// 幂等：主键冲突就忽略
	return s.DB.Clauses(clause.OnConflict{DoNothing: true}).Create(&rec).Error
}

// =============== 查询（给 Logic / API 用） ===============

func (s *Store) ListRoomMessages(ctx context.Context, roomID, limit int) ([]ChatMessage, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	var rows []ChatMessage
	err := s.DB.WithContext(ctx).
		Where("room_id = ?", roomID).
		Order("created_at DESC").
		Limit(limit).
		Find(&rows).Error
	if err != nil {
		return nil, err
	}
	// 可在这里反转为正序，视前端需要
	for i, j := 0, len(rows)-1; i < j; i, j = i+1, j-1 {
		rows[i], rows[j] = rows[j], rows[i]
	}
	return rows, nil
}
