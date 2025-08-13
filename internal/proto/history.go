package proto

type ListMessagesRequest struct {
	RoomId int    `json:"roomId"`
	Limit  int    `json:"limit"` // 最近 N 条
	Since  string `json:"since"` // 可选：YYYY-MM-DD HH:MM:SS
}
type ListMessagesResponse struct {
	Code int          `json:"code"`
	Data []MessageDTO `json:"data"`
}
type MessageDTO struct {
	Id           int64  `json:"id"`
	RoomId       int    `json:"roomId"`
	FromUserId   int    `json:"fromUserId"`
	FromUserName string `json:"fromUserName"`
	Content      string `json:"content"`
	CreateTime   string `json:"createTime"`
}
