package client

// 一个简易的客户端，用于沟通api层和connect层
// 沟通api层主要是提供日常服务，connect层则是登录连接

const (
	// api层 与 connect层 地址
	apiBase = "http://localhost:7070"
	wsBase  = "ws://localhost:7000"

	// /user 路由 会话与认证相关
	loginPath     = "/user/login"
	registerPath  = "/user/register"
	logoutPath    = "/user/logout"
	checkAuthPath = "/user/checkAuth"

	// /push 路由 聊天相关
	pushPath        = "/push/push"
	getRoomInfoPath = "/push/getRoomInfo"
	pushRoomPath    = "/push/pushRoom"
	countPath       = "/path/count"

	// ws
	connectPath = "/ws"
)

type Client struct{}

func New() *Client {
	return &Client{}
}

func (c *Client) Run() {
	UIRun()
}
