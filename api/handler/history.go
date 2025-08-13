// api/handler/history.go
package handler

import (
	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	"gochat/api/rpc"
	"gochat/internal/proto"
	"gochat/internal/tools"
)

type FormRoomHistory struct {
	AuthToken string `json:"authToken" binding:"required"`
	RoomId    int    `json:"roomId" binding:"required"`
	Limit     int    `json:"limit"` // 默认 100，最大 500
	// 可选分页：BeforeId *int `json:"beforeId"`
}

func ListRoomHistory(c *gin.Context) {
	var form FormRoomHistory
	if err := c.ShouldBindBodyWith(&form, binding.JSON); err != nil {
		tools.FailWithMsg(c, err.Error())
		return
	}

	// 会话已在中间件校验，这里可不必再校验；如需拿到用户名可再次调用：
	// code, userId, _ := rpc.RpcLogicObj.CheckAuth(&proto.CheckAuthRequest{AuthToken: form.AuthToken})
	// if code == tools.CodeFail { tools.FailWithMsg(c, "auth fail"); return }

	if form.Limit <= 0 || form.Limit > 500 {
		form.Limit = 100
	}

	// 调 logic 拉历史
	req := &proto.ListMessagesRequest{RoomId: form.RoomId, Limit: form.Limit}
	code, list, msg := rpc.RpcLogicObj.ListRoomMessages(req)
	if code == tools.CodeFail {
		tools.FailWithMsg(c, msg)
		return
	}

	// 和你现有 Response 结构兼容：code/message/data
	tools.SuccessWithMsg(c, "ok", list)
}
