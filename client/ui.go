package client

import (
	"fmt"
	"log"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/gorilla/websocket"
)

// 消息模型
type Message struct {
	Sender    string    `json:"sender"`
	Recipient string    `json:"recipient"`
	Content   string    `json:"content"`
	Timestamp time.Time `json:"timestamp"`
}

// UI 状态类型
type model struct {
	messages    []Message       // 所有消息
	input       textinput.Model // 输入框
	recipient   string          // 当前聊天对象
	onlineUsers []string        // 在线用户列表
	status      string          // 状态信息
	err         error           // 错误信息
	width       int             // 终端宽度
	height      int             // 终端高度
	activeChat  string          // 当前活动聊天区域
	wsConn      *websocket.Conn // WebSocket 连接
	token       string          // 认证令牌
	username    string          // 当前用户名
	loading     bool            // 加载状态
	userId      string          // 用户ID
	serverId    string          // connect层ID
	roomId      int32           // 房间号
}

// 初始化客户端
func NewIMClient(token, username string) model {
	ti := textinput.New()
	ti.Placeholder = "输入消息 (输入 '@用户 内容' 私聊, 输入 '/exit' 退出)"
	ti.Focus()
	ti.CharLimit = 256
	ti.Prompt = ">>> "
	ti.PromptStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#57E2E5"))

	m := model{
		input:      ti,
		status:     "正在连接聊天服务器...",
		activeChat: "chat", // 默认聊天区域
		token:      token,
		username:   username,
		loading:    true,
	}

	// 初始化 WebSocket
	go m.initWebSocket()

	return m
}

// BubbleTea UI 模型初始化
func (m model) Init() tea.Cmd {
	return textinput.Blink
}

// BubbleTea UI 更新
func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			// 返回主界面
			m.activeChat = "chat"
		case "ctrl+c", "q":
			// 退出程序
			if m.wsConn != nil {
				m.wsConn.Close()
			}
			return m, tea.Quit
		case "enter":
			// 发送消息
			if !m.loading {
				m.pushMessage()
			}
		case "tab":
			// 切换聊天区域
			if m.activeChat == "chat" {
				m.activeChat = "users"
			} else {
				m.activeChat = "chat"
			}
		}
	case tea.WindowSizeMsg:
		// 更新终端尺寸
		m.width = msg.Width
		m.height = msg.Height
	case error:
		m.err = msg
		return m, nil
	}

	if !m.loading && m.activeChat == "chat" {
		m.input, cmd = m.input.Update(msg)
	}

	return m, cmd
}

// BubbleTea UI 渲染
func (m model) View() string {
	if m.err != nil {
		return fmt.Sprintf("错误: %v\n按任意键退出", m.err)
	}

	if m.width == 0 {
		return "正在初始化..."
	}

	// 状态栏
	statusBar := statusBarStyle.Render(fmt.Sprintf("用户: %s | 状态: %s | %s",
		m.username, m.status, time.Now().Format("15:04:05")))

	// 分割屏幕：聊天区/用户列表
	var body string

	switch m.activeChat {
	case "users":
		// 用户列表视图
		users := userListStyle.Render(" 在线用户:\n\n")
		for _, user := range m.onlineUsers {
			if user != m.username {
				users += fmt.Sprintf("• %s\n", user)
			}
		}
		body = userListContainerStyle.Render(users)
	default:
		// 聊天视图
		chatHeader := "群聊"
		if m.recipient != "" {
			chatHeader = "与 " + m.recipient + " 私聊中"
		}
		header := chatHeaderStyle.Render(chatHeader)

		// 消息区域
		messages := ""
		for _, msg := range m.messages {
			t := msg.Timestamp.Format("15:04")
			sender := msg.Sender

			// 区分发送和接收的消息
			if msg.Sender == m.username {
				sender = "你"
			}

			// 过滤群聊消息
			if m.recipient == "" && msg.Recipient != "" {
				continue
			}

			// 过滤私聊消息
			if m.recipient != "" &&
				!(msg.Sender == m.recipient && msg.Recipient == m.username) &&
				!(msg.Sender == m.username && msg.Recipient == m.recipient) {
				continue
			}

			msgLine := fmt.Sprintf("[%s] %s: %s", t, sender, msg.Content)

			if sender == "你" {
				msgLine = myMessageStyle.Render(msgLine)
			} else {
				msgLine = otherMessageStyle.Render(msgLine)
			}
			messages += msgLine + "\n"
		}
		messagesArea := messagesStyle.Render(messages)

		// 输入区域
		inputArea := ""
		if !m.loading {
			inputArea = m.input.View()
		}

		body = lipgloss.JoinVertical(lipgloss.Left,
			header,
			messagesArea,
			inputArea,
		)
	}

	// 帮助信息
	helpText := helpStyle.Render("快捷键: [TAB] 切换视图  [↑↓] 导航  [ENTER] 发送  [Q] 退出")

	// 整体布局
	fullView := lipgloss.JoinVertical(lipgloss.Left,
		statusBar,
		body,
		helpText,
	)

	return appStyle.Render(fullView)
}

// 主函数
func UIRun() {
	// 处理登录
	token, username := handleLogin()
	if token == "" {
		fmt.Println("登录失败，请先注册")
		token, username = handleRegister()
	}

	if token == "" {
		fmt.Println("未接受到token响应")
		return
	}
	// 初始化程序
	p := tea.NewProgram(NewIMClient(token, username))

	// 启动UI
	if _, err := p.Run(); err != nil {
		log.Fatalf("启动失败: %v", err)
	}
}
