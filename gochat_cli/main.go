package gochat_cli

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/gorilla/websocket"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
)

const (
	apiHost = "http://localhost:7070"  // 后端API地址
	wsHost  = "ws://localhost:7000/ws" // WebSocket地址
)

var (
	authToken = "" // 认证令牌
	roomID    = 1  // 默认房间ID
)

type Response struct {
	Code    int         `json:"code"`
	Data    interface{} `json:"data"`
	Message string      `json:"message"`
}

type User struct {
	UserName string `json:"userName"`
	PassWord string `json:"passWord"`
}

func Run() {
	fmt.Println("GoChat 命令行客户端")

	for {
		fmt.Println("\n1. 登录")
		fmt.Println("2. 注册")
		fmt.Println("3. 进入聊天室")
		fmt.Println("4. 退出")
		fmt.Print("请选择操作: ")

		scanner := bufio.NewScanner(os.Stdin)
		scanner.Scan()
		choice := scanner.Text()

		switch choice {
		case "1":
			login()
		case "2":
			register()
		case "3":
			enterChatRoom()
		case "4":
			fmt.Println("再见!")
			return
		default:
			fmt.Println("无效选择，请重新输入")
		}
	}
}

// 用户登录 (对应前端登录功能)
func login() {
	scanner := bufio.NewScanner(os.Stdin)

	fmt.Print("用户名: ")
	scanner.Scan()
	username := scanner.Text()

	fmt.Print("密码: ")
	scanner.Scan()
	password := scanner.Text()

	user := User{UserName: username, PassWord: password}
	jsonData, _ := json.Marshal(user)

	resp, err := http.Post(apiHost+"/user/login", "application/json", strings.NewReader(string(jsonData)))
	if err != nil {
		log.Fatal("登录请求失败:", err)
	}
	defer resp.Body.Close()

	var response Response
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		log.Fatal("响应解析失败:", err)
	}

	if response.Code != 0 {
		log.Fatal("登录失败:", response.Message)
	}

	authToken = response.Data.(string)
	fmt.Println("登录成功! 认证令牌:", authToken)
}

// 用户注册 (对应前端注册功能)
func register() {
	scanner := bufio.NewScanner(os.Stdin)

	fmt.Print("用户名: ")
	scanner.Scan()
	username := scanner.Text()

	fmt.Print("密码: ")
	scanner.Scan()
	password := scanner.Text()

	user := User{UserName: username, PassWord: password}
	jsonData, _ := json.Marshal(user)

	resp, err := http.Post(apiHost+"/user/register", "application/json", strings.NewReader(string(jsonData)))
	if err != nil {
		log.Fatal("注册请求失败:", err)
	}
	defer resp.Body.Close()

	var response Response
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		log.Fatal("响应解析失败:", err)
	}

	if response.Code != 0 {
		log.Fatal("注册失败:", response.Message)
	}

	authToken = response.Data.(string)
	fmt.Println("注册成功! 认证令牌:", authToken)
}

// 进入聊天室 (对应前端Room组件)
func enterChatRoom() {
	if authToken == "" {
		fmt.Println("请先登录或注册")
		return
	}

	u, _ := url.Parse(wsHost)
	conn, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		log.Fatal("WebSocket连接失败:", err)
	}
	defer conn.Close()

	// 发送认证消息 (对应前端onopen)
	authData := map[string]interface{}{"authToken": authToken, "roomId": roomID}
	if err := conn.WriteJSON(authData); err != nil {
		log.Fatal("认证消息发送失败:", err)
	}

	fmt.Println("已连接到聊天室 (输入 /exit 退出, /users 查看在线用户)")

	// 启动消息接收 (对应前端onmessage)
	go receiveMessages(conn)

	// 处理用户输入 (对应前端消息输入区)
	scanner := bufio.NewScanner(os.Stdin)
	for {
		fmt.Print("> ")
		scanner.Scan()
		text := scanner.Text()

		if text == "/exit" {
			break
		}

		if text == "/users" {
			getOnlineUsers()
			continue
		}

		if len(text) > 0 {
			sendMessage(text) // 使用HTTP POST发送消息
		}
	}
}

// 接收消息 (对应前端消息处理)
func receiveMessages(conn *websocket.Conn) {
	for {
		_, message, err := conn.ReadMessage()
		if err != nil {
			log.Println("读取消息错误:", err)
			return
		}

		var data map[string]interface{}
		if err := json.Unmarshal(message, &data); err != nil {
			log.Println("消息解析错误:", err)
			continue
		}

		switch op := data["op"].(float64); op {
		case 3: // 聊天消息 (对应前端op=3处理)
			fmt.Printf("\n[%s](%s): %s\n> ",
				data["fromUserName"],
				data["createTime"],
				data["msg"])
		case 4: // 在线人数更新 (对应前端op=4)
			fmt.Printf("\n[系统] 在线人数: %v\n> ", data["count"])
		case 5: // 用户列表更新 (对应前端op=5)
			fmt.Printf("\n[系统] 用户列表已更新\n> ")
		}
	}
}

// 发送消息 (完全对应前端pushRoom API)
func sendMessage(text string) {
	params := map[string]interface{}{
		"op":        5,
		"roomId":    roomID,
		"authToken": authToken,
		"msg":       text,
	}

	jsonData, _ := json.Marshal(params)
	resp, err := http.Post(apiHost+"/push/pushRoom", "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		log.Println("消息发送失败:", err)
		return
	}
	defer resp.Body.Close()

	var response Response
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		log.Println("响应解析失败:", err)
		return
	}

	if response.Code != 0 {
		log.Println("发送失败:", response.Message)
	}
}

// 获取在线用户 (对应前端getRoomInfo)
func getOnlineUsers() {
	params := map[string]interface{}{
		"roomId":    1,
		"authToken": authToken, // 添加中间件必需的字段
	}

	jsonData, _ := json.Marshal(params)
	req, _ := http.NewRequest("POST", apiHost+"/push/getRoomInfo", bytes.NewBuffer(jsonData))
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		log.Println("请求失败:", err)
		return
	}
	defer resp.Body.Close()

	// 在解析响应前打印原始响应体
	body, _ := ioutil.ReadAll(resp.Body)
	log.Println("原始响应:", string(body))

	// 需要再次将body放入新的Reader中
	responseBody := bytes.NewReader(body)

	var response Response
	if err := json.NewDecoder(responseBody).Decode(&response); err != nil {
		log.Println("响应解析失败:", err)
		return
	}

	//var response Response
	//if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
	//	log.Println("响应解析失败:", err)
	//	return
	//}

	if response.Code != 0 {
		log.Println("获取用户列表失败:", response.Message)
		return
	}

	fmt.Println("\n在线用户列表:")
	if users, ok := response.Data.([]interface{}); ok {
		for i, user := range users {
			fmt.Printf("%d. %s\n", i+1, user)
		}
	}
	fmt.Print("> ")
}
