package logic

import (
	"context"
	"github.com/pkg/errors"
	"gochat/config"
	"gochat/db"
	"gochat/internal/chatstore"
	"gochat/internal/proto"
	"time"
)

func (rpc *RpcLogic) ListRoomMessages(ctx context.Context, req *proto.ListMessagesRequest, resp *proto.ListMessagesResponse) error {
	resp.Code = config.FailReplyCode
	if req.RoomId <= 0 {
		return errors.New("roomId required")
	}
	if req.Limit <= 0 || req.Limit > 500 {
		req.Limit = 100
	}

	store := chatstore.New(db.GetDb("gochat"))
	rows, err := store.ListRoomMessages(ctx, req.RoomId, req.Limit)
	if err != nil {
		return err
	}
	out := make([]proto.MessageDTO, 0, len(rows))
	for _, r := range rows {
		out = append(out, proto.MessageDTO{
			Id:           r.ID,
			RoomId:       r.RoomID,
			FromUserId:   r.FromUserID,
			FromUserName: r.FromUserName,
			Content:      r.Content,
			CreateTime:   r.CreatedAt.In(time.Local).Format("2006-01-02 15:04:05"),
		})
	}
	resp.Data = out
	resp.Code = config.SuccessReplyCode
	return nil
}
