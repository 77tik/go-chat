package client

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/gorilla/websocket"
	"gochat/internal/proto"
	"net/http"
	"time"
)

// 初始化 WebSocket 连接
func (m *model) initWebSocket() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// 1. 建立无认证的WebSocket连接
	wsURL := fmt.Sprintf("%s%s", wsBase, connectPath)
	conn, _, err := websocket.DefaultDialer.DialContext(ctx, wsURL, nil)
	if err != nil {
		m.err = fmt.Errorf("无法连接服务器: %v", err)
		return
	}

	m.wsConn = conn

	// 2. 立即发送认证消息
	authReq := proto.ConnectRequest{
		AuthToken: m.token,
		ServerId:  m.serverId,    // 如果需要
		RoomId:    int(m.roomId), // 如果需要加入房间
	}

	msgData, err := json.Marshal(authReq)
	if err != nil {
		m.err = fmt.Errorf("认证消息序列化失败: %v", err)
		return
	}

	if err := conn.WriteMessage(websocket.TextMessage, msgData); err != nil {
		m.err = fmt.Errorf("发送认证消息失败: %v", err)
		return
	}

	m.wsConn = conn
	m.status = "已连接到聊天服务器"

	// 启动消息接收协程
	go m.receiveMessages()

	// 获取在线用户
	go m.fetchOnlineUsers()
}

// 接收消息 TODO：得看看connect层传过来的是不是就是已经反序列化的了，不然还要反序列化
func (m *model) receiveMessages() {
	for {
		if m.wsConn == nil {
			time.Sleep(1 * time.Second)
			continue
		}

		_, msgBytes, err := m.wsConn.ReadMessage()
		if err != nil {
			m.err = fmt.Errorf("读取消息失败: %v", err)
			time.Sleep(2 * time.Second)
			continue
		}

		// 解析为消息或在线用户更新
		var msg Message
		if err := json.Unmarshal(msgBytes, &msg); err == nil {
			// 处理消息
			m.processMessage(msg)
		} else {
			// 处理在线用户更新
			var users []string
			if err := json.Unmarshal(msgBytes, &users); err == nil {
				m.onlineUsers = users
			}
		}
	}
}

// 处理收到的消息
func (m *model) processMessage(msg Message) {
	m.messages = append(m.messages, msg)
}

// 获取在线用户
func (m *model) fetchOnlineUsers() {
	req, err := http.NewRequest("POST", apiBase+getRoomInfoPath, nil)
	if err != nil {
		return
	}
	req.Header.Set("Authorization", "Bearer "+m.token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()

	var users []string
	if err := json.NewDecoder(resp.Body).Decode(&users); err == nil {
		m.onlineUsers = users
	}
}
