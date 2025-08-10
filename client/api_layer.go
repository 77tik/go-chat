package client

import (
	"encoding/json"
	"fmt"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"log"
	"net/http"
	"strings"
	"sync"
)

// BubbleTea 登录模型
type loginModel struct {
	username textinput.Model
	password textinput.Model
	err      error
}

type registerModel struct {
	username textinput.Model
	password textinput.Model
	err      error
}

func (m *loginModel) Init() tea.Cmd {
	return textinput.Blink
}

func (m *loginModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "enter":
			return m, tea.Quit
		case "ctrl+c", "q":
			return m, tea.Quit
		}
	}

	// 更新两个输入框
	m.username, cmd = m.username.Update(msg)
	cmds = append(cmds, cmd)

	m.password, cmd = m.password.Update(msg)
	cmds = append(cmds, cmd)

	return m, tea.Batch(cmds...)
}

func (m *loginModel) View() string {
	return fmt.Sprintf(
		"用户名:\n%s\n\n密码:\n%s\n\n按 Enter 登录, Ctrl+C 退出",
		m.username.View(),
		m.password.View(),
	) + "\n"
}

// 核心登录函数
func handleLogin() (string, string) {
	// 1. 初始化登录模型
	model := &loginModel{
		username: textinput.New(),
		password: textinput.New(),
	}

	model.username.Placeholder = "用户名"
	model.username.Focus()
	model.username.CharLimit = 20

	model.password.Placeholder = "密码"
	model.password.EchoMode = textinput.EchoPassword
	model.password.EchoCharacter = '•'

	// 2. 启动登录UI
	fmt.Println("聊天系统登录")
	fmt.Println("--------------")
	p := tea.NewProgram(model)

	// 3. 等待用户输入完成
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		if _, err := p.Run(); err != nil {
			log.Fatal(err)
		}
		wg.Done()
	}()
	wg.Wait()

	// 4. 准备登录请求 (对应时序图步骤1)
	loginData, _ := json.Marshal(map[string]string{
		"userName": model.username.Value(),
		"passWord": model.password.Value(),
	})

	// 5. 发送认证请求 (对应时序图步骤2)
	resp, err := http.Post(apiBase+loginPath, "application/json",
		strings.NewReader(string(loginData)))
	if err != nil {
		log.Fatal("登录请求失败:", err)
	}
	defer resp.Body.Close()

	// 6. 处理认证响应 (对应时序图步骤4)
	var result struct {
		Token string `json:"auth_token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		log.Fatal("解析登录响应失败:", err)
	}

	return result.Token, model.username.Value()
}

func handleRegister() (string, string) {
	// 1. 初始化登录模型
	model := &loginModel{
		username: textinput.New(),
		password: textinput.New(),
	}

	model.username.Placeholder = "用户名"
	model.username.Focus()
	model.username.CharLimit = 20

	model.password.Placeholder = "密码"
	model.password.EchoMode = textinput.EchoPassword
	model.password.EchoCharacter = '•'

	// 2. 启动登录UI
	fmt.Println("聊天系统注册")
	fmt.Println("--------------")
	p := tea.NewProgram(model)

	// 3. 等待用户输入完成
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		if _, err := p.Run(); err != nil {
			log.Fatal(err)
		}
		wg.Done()
	}()
	wg.Wait()

	// 4. 准备登录请求 (对应时序图步骤1)
	registerData, _ := json.Marshal(map[string]string{
		"userName": model.username.Value(),
		"passWord": model.password.Value(),
	})

	// 5. 发送认证请求 (对应时序图步骤2)
	resp, err := http.Post(apiBase+registerPath, "application/json",
		strings.NewReader(string(registerData)))
	if err != nil {
		log.Fatal("登录请求失败:", err)
	}
	defer resp.Body.Close()

	// 6. 处理认证响应 (对应时序图步骤4)
	var result struct {
		Token string `json:"auth_token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		log.Fatal("解析登录响应失败:", err)
	}

	return result.Token, model.username.Value()
}
