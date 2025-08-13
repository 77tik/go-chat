package task

import (
	"gochat/db"
	"gochat/internal/chatstore"
	"time"
)

type ChatMessage struct {
	ID           int64     `gorm:"primaryKey;column:id"`
	RoomID       int       `gorm:"column:room_id"`
	FromUserID   int       `gorm:"column:from_user_id"`
	FromUserName string    `gorm:"column:from_user_name"`
	Content      string    `gorm:"column:content"`
	Op           int       `gorm:"column:op"`
	CreatedAt    time.Time `gorm:"column:created_at"` // GORM 会存 TEXT(ISO8601)
}

func (t *Task) InitHistoryStore() error {
	t.History = chatstore.New(db.GetDb("gochat"))
	return t.History.AutoMigrate()
}
