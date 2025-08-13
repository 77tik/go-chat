package proto

type Job struct {
	Op         string `json:"op"` // "ask" | "summarize" | "translate"
	RoomID     int    `json:"roomId"`
	FromUserID int    `json:"fromUserId"`
	FromName   string `json:"fromUserName"`
	Prompt     string `json:"prompt"` // /ai 的问题；/summarize 则为空，由 worker 读历史拼好
	Lang       string `json:"lang"`   // translate 目标语言
}

type Result struct {
	RoomID int    `json:"roomId"`
	Text   string `json:"text"`  // AI 返回的文本
	Op     string `json:"op"`    // 同上，回显用途
	Model  string `json:"model"` // 记录用
	Err    string `json:"err,omitempty"`
}
