package gochat_cli

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"github.com/gorilla/websocket"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	apiHost = "http://localhost:7070"  // 后端API地址
	wsHost  = "ws://localhost:7000/ws" // WebSocket地址
)

var (
	authToken   = "" // 认证令牌
	roomID      = 1  // 默认房间ID
	currentUser = "" // 当前登录用户名（用于自我高亮）
	wsConn      *websocket.Conn
)

// —— 终端样式 —— //
const (
	reset  = "\x1b[0m"
	bold   = "\x1b[1m"
	dim    = "\x1b[2m"
	faint  = "\x1b[2m"
	italic = "\x1b[3m"

	fgGray    = "\x1b[90m"
	fgRed     = "\x1b[31m"
	fgGreen   = "\x1b[32m"
	fgYellow  = "\x1b[33m"
	fgBlue    = "\x1b[34m"
	fgMagenta = "\x1b[35m"
	fgCyan    = "\x1b[36m"
	fgWhite   = "\x1b[97m"
)

// —— 通用响应结构 —— //
type RespToken struct {
	Code    int    `json:"code"`
	Data    string `json:"data"`
	Message string `json:"message"`
}

type CommonResp struct {
	Code    int             `json:"code"`
	Data    json.RawMessage `json:"data"`
	Message string          `json:"message"`
}

type User struct {
	UserName string `json:"userName"`
	PassWord string `json:"passWord"`
}

// 内层业务消息（op=3 时会在外层 msg 里 base64/或直接 JSON）
type InnerMsg struct {
	Code         int    `json:"code"`
	Msg          string `json:"msg"`
	FromUserId   int    `json:"fromUserId"`
	FromUserName string `json:"fromUserName"`
	ToUserId     int    `json:"toUserId"`
	ToUserName   string `json:"toUserName"`
	RoomId       int    `json:"roomId"`
	Op           int    `json:"op"`
	CreateTime   string `json:"createTime"`
}

func Run() {
	clearScreen()
	fmt.Printf("%s%sGoChat 终端客户端%s  •  %s输入 %s/help%s 查看命令\n", bold, fgCyan, reset, fgGray, fgYellow, reset)

	for {
		fmt.Printf("\n%s1.%s 登录    %s2.%s 注册    %s3.%s 进入聊天室    %s4.%s 退出\n",
			fgYellow, reset, fgYellow, reset, fgYellow, reset, fgYellow, reset)
		fmt.Print("请选择操作: ")

		scanner := bufio.NewScanner(os.Stdin)
		if !scanner.Scan() {
			fmt.Println()
			return
		}
		choice := stringsTrim(scanner.Text())

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

// 用户登录
func login() {
	sc := bufio.NewScanner(os.Stdin)

	fmt.Print("用户名: ")
	sc.Scan()
	username := stringsTrim(sc.Text())

	fmt.Print("密码: ")
	sc.Scan()
	password := stringsTrim(sc.Text())

	user := User{UserName: username, PassWord: password}
	jsonData, _ := json.Marshal(user)

	resp, err := http.Post(apiHost+"/user/login", "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		printErr("登录请求失败: %v", err)
		return
	}
	defer resp.Body.Close()

	var r RespToken
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		printErr("响应解析失败: %v", err)
		return
	}
	if r.Code != 0 {
		printErr("登录失败: %s", r.Message)
		return
	}
	authToken = r.Data
	currentUser = username
	printOk("登录成功！%s令牌已就绪%s", dim, reset)
}

// 用户注册
func register() {
	sc := bufio.NewScanner(os.Stdin)

	fmt.Print("用户名: ")
	sc.Scan()
	username := stringsTrim(sc.Text())

	fmt.Print("密码: ")
	sc.Scan()
	password := stringsTrim(sc.Text())

	user := User{UserName: username, PassWord: password}
	jsonData, _ := json.Marshal(user)

	resp, err := http.Post(apiHost+"/user/register", "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		printErr("注册请求失败: %v", err)
		return
	}
	defer resp.Body.Close()

	var r RespToken
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		printErr("响应解析失败: %v", err)
		return
	}
	if r.Code != 0 {
		printErr("注册失败: %s", r.Message)
		return
	}
	authToken = r.Data
	currentUser = username
	printOk("注册成功！已自动登录")
}

// 进入聊天室
func enterChatRoom() {
	if authToken == "" {
		printWarn("请先登录或注册")
		return
	}

	conn, _, err := websocket.DefaultDialer.Dial(wsHost, nil)
	if err != nil {
		printErr("WebSocket连接失败: %v", err)
		return
	}
	wsConn = conn
	defer func() {
		_ = conn.Close()
		wsConn = nil
	}()

	// 认证（onopen）
	authData := map[string]interface{}{"authToken": authToken, "roomId": roomID}
	if err := conn.WriteJSON(authData); err != nil {
		printErr("认证消息发送失败: %v", err)
		return
	}

	header(roomID)
	loadHistory(50) // <<< 新增：初始拉取 50 条历史

	done := make(chan struct{})
	go receiveMessages(conn, done)

	// 输入循环
	sc := bufio.NewScanner(os.Stdin)
	printPrompt()
	for sc.Scan() {
		cmd := stringsTrim(sc.Text())
		switch {
		case cmd == "/exit":
			_ = conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""), time.Now().Add(1*time.Second))
			<-done
			fmt.Println()
			return
		case cmd == "/users":
			triggerRoomInfo()
		case strings.HasPrefix(cmd, "/room "):
			id := stringsTrim(strings.TrimPrefix(cmd, "/room "))
			if id == "" {
				printWarn("用法: /room <房间ID>")
			} else {
				printWarn("当前示例客户端只在启动时加入房间。要切换房间，请重进。")
			}
		case strings.HasPrefix(cmd, "/history"):
			// /history 或 /history 100
			fields := strings.Fields(cmd)
			n := 50
			if len(fields) >= 2 {
				if v, err := strconv.Atoi(fields[1]); err == nil {
					n = v
				}
			}
			loadHistory(n)

		case strings.HasPrefix(cmd, "/sum"):
			// /sum 或 /sum 120
			fields := strings.Fields(cmd)
			n := 120
			if len(fields) >= 2 {
				if v, err := strconv.Atoi(fields[1]); err == nil {
					n = v
				}
			}
			aiSummarize(n)
		case cmd == "/help":
			fmt.Println()
			fmt.Println("可用命令：")
			fmt.Println("  /users            查看在线用户（由服务端通过 WS 推送）")
			fmt.Println("  /history [N]      拉取最近 N 条历史（默认 50，最大 500）")
			fmt.Println("  /sum [N]          让 AI 总结最近 N 条历史（默认 120，最大 500）")
			fmt.Println("  /exit             退出聊天室")

		default:
			if cmd != "" {
				sendRoomMessage(cmd)
			}
		}
		printPrompt()
	}
	if err := sc.Err(); err != nil {
		printErr("输入错误: %v", err)
	}
}

// 接收 WS 消息
func receiveMessages(conn *websocket.Conn, done chan struct{}) {
	defer close(done)
	for {
		mt, payload, err := conn.ReadMessage()
		if err != nil {
			if !websocket.IsCloseError(err, websocket.CloseNormalClosure) {
				printErr("网络断开: %v", err)
			}
			return
		}
		if mt != websocket.TextMessage {
			continue
		}

		var evt map[string]interface{}
		if err := json.Unmarshal(payload, &evt); err != nil {
			printErr("消息解析错误: %v", err)
			continue
		}
		op := asInt(evt["op"])

		switch op {
		case 3: // 房间聊天
			inner := decodeInnerMsg(evt["msg"])
			if inner == nil {
				// 兜底：有些服务端直接把所有字段放外层
				inner = innerFromOuter(evt)
			}
			// 如果还是拿不到内容，就别再打印原始 JSON 了，给个温和提示
			if inner == nil || strings.TrimSpace(inner.Msg) == "" {
				printSystem("收到一条空消息或未知格式")
				break
			}
			printChat(inner)
		case 4: // 在线人数
			cnt := asInt(evt["count"])
			if cnt == 0 {
				if inner := decodeInnerMsg(evt["msg"]); inner != nil {
					cnt = inner.Code // 如果你把 count 放在别的字段，请改这里
				}
			}
			printSystem("在线人数：%d", cnt)
		case 5: // 房间用户列表
			if m, ok := evt["roomUserInfo"].(map[string]interface{}); ok {
				printUserList(m)
			} else if raw := decodeInnerRaw(evt["msg"]); len(raw) > 0 {
				var x struct {
					RoomUserInfo map[string]string `json:"roomUserInfo"`
					Count        int               `json:"count"`
					RoomId       int               `json:"roomId"`
				}
				if err := json.Unmarshal(raw, &x); err == nil && len(x.RoomUserInfo) > 0 {
					m2 := make(map[string]interface{}, len(x.RoomUserInfo))
					for k, v := range x.RoomUserInfo {
						m2[k] = v
					}
					printUserList(m2)
				} else {
					printSystem("用户列表已更新")
				}
			} else {
				printSystem("用户列表已更新")
			}
		default:
			printSystem("事件 op=%d：%s", op, string(payload))
		}
	}
}

// 发送群聊消息（HTTP 触发，由服务端广播）
func sendRoomMessage(text string) {
	params := map[string]interface{}{
		"roomId":    roomID,
		"authToken": authToken,
		"msg":       text,
	}
	jsonData, _ := json.Marshal(params)
	resp, err := http.Post(apiHost+"/push/pushRoom", "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		printErr("消息发送失败: %v", err)
		return
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)
}

// 触发下发房间信息（数据走 WS）
func triggerRoomInfo() {
	params := map[string]interface{}{
		"roomId":    roomID,
		"authToken": authToken,
	}
	jsonData, _ := json.Marshal(params)
	resp, err := http.Post(apiHost+"/push/getRoomInfo", "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		printErr("请求失败: %v", err)
		return
	}
	defer resp.Body.Close()
	printSystem("已请求最新在线用户列表，请留意 WS 推送")
}

// 历史消息（对应 /history/list 返回的每条）
type HistMsg struct {
	Id           int64  `json:"id"`
	RoomId       int    `json:"roomId"`
	FromUserId   int    `json:"fromUserId"`
	FromUserName string `json:"fromUserName"`
	Content      string `json:"content"`
	CreateTime   string `json:"createTime"` // "YYYY-MM-DD HH:MM:SS"（服务端已转本地时区）
}

// 进入房间后调用：拉取最近 N 条历史，按时间正序打印
func loadHistory(limit int) {
	if limit <= 0 || limit > 500 {
		limit = 50
	}

	params := map[string]interface{}{
		"authToken": authToken,
		"roomId":    roomID,
		"limit":     limit,
	}
	b, _ := json.Marshal(params)
	resp, err := http.Post(apiHost+"/history/list", "application/json", bytes.NewBuffer(b))
	if err != nil {
		printErr("拉取历史失败: %v", err)
		return
	}
	defer resp.Body.Close()

	var r CommonResp
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		printErr("解析历史失败: %v", err)
		return
	}
	if r.Code != 0 {
		printErr("历史接口错误: %s", r.Message)
		return
	}

	var list []HistMsg
	if err := json.Unmarshal(r.Data, &list); err != nil {
		printErr("历史数据解析失败: %v", err)
		return
	}
	if len(list) == 0 {
		printSystem("暂无历史消息")
		return
	}
	printSystem("载入历史 %d 条：", len(list))
	for _, m := range list {
		// 复用现有渲染
		im := &InnerMsg{
			Msg:          m.Content,
			FromUserName: m.FromUserName,
			CreateTime:   m.CreateTime,
		}
		printChat(im)
	}
}

// 触发 AI 总结（结果稍后由 WS 推送回来，FromUserName 通常是 "🤖 AI"）
func aiSummarize(limit int) {
	if limit <= 0 || limit > 500 {
		limit = 120
	}
	params := map[string]interface{}{
		"authToken": authToken,
		"roomId":    roomID,
		"limit":     limit,
	}
	b, _ := json.Marshal(params)
	resp, err := http.Post(apiHost+"/ai/summarize", "application/json", bytes.NewBuffer(b))
	if err != nil {
		printErr("提交 AI 总结失败: %v", err)
		return
	}
	defer resp.Body.Close()

	var r CommonResp
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		printErr("AI 总结响应解析失败: %v", err)
		return
	}
	if r.Code != 0 {
		printErr("AI 总结接口错误: %s", r.Message)
		return
	}
	printSystem("已提交 AI 总结任务，请稍候留意机器人消息")
}

// —— 样式化输出 —— //

func header(room int) {
	clearScreen()
	bar := strings.Repeat("─", 38)
	fmt.Printf("%s%s%s\n", fgGray, bar, reset)
	fmt.Printf("%s%sGoChat%s  %s@%s%s  •  房间 #%d\n", bold, fgCyan, reset, fgYellow, currentUser, reset, room)
	fmt.Printf("%s%s%s\n\n", fgGray, bar, reset)
	fmt.Printf("%s提示%s：输入消息直接发送；%s/help%s 查看命令；%s/users%s 查看在线用户；%s/exit%s 退出\n\n",
		fgGray, reset, fgYellow, reset, fgYellow, reset, fgYellow, reset)
}

func printPrompt() {
	fmt.Printf("%s> %s", fgGray, reset)
}

func printSystem(format string, a ...any) {
	msg := fmt.Sprintf(format, a...)
	fmt.Printf("\r%s[系统]%s %s\n", fgMagenta, reset, msg)
}

func printOk(format string, a ...any) {
	msg := fmt.Sprintf(format, a...)
	fmt.Printf("%s✔%s %s\n", fgGreen, reset, msg)
}

func printWarn(format string, a ...any) {
	msg := fmt.Sprintf(format, a...)
	fmt.Printf("%s⚠%s %s\n", fgYellow, reset, msg)
}

func printErr(format string, a ...any) {
	msg := fmt.Sprintf(format, a...)
	fmt.Printf("%s✖%s %s\n", fgRed, reset, msg)
}

func printRaw(payload []byte) {
	fmt.Printf("\r%s[消息]%s %s\n", fgCyan, reset, string(payload))
}

func printUserList(m map[string]interface{}) {
	fmt.Printf("\r%s[在线用户]%s（共 %d 人）\n", fgCyan, reset, len(m))
	fmt.Printf("%s%-6s %-18s%s\n", faint, "UserID", "UserName", reset)
	fmt.Printf("%s%-6s %-18s%s\n", fgGray, "------", "------------------", reset)
	for uid, nameAny := range m {
		name := fmt.Sprintf("%v", nameAny)
		color := fgWhite
		deco := ""
		if name == currentUser {
			color = fgGreen
			deco = " (你)"
		}
		fmt.Printf("%-6s %s%-18s%s%s\n", uid, color, name, reset, deco)
	}
}

func printChat(im *InnerMsg) {
	t := im.CreateTime
	if t == "" {
		t = time.Now().Format("15:04:05")
	} else if len(t) >= 8 {
		// 截到 HH:MM:SS（无论服务端给的是完整日期还是时分秒）
		t = t[len(t)-8:]
	}
	name := im.FromUserName
	me := (name == currentUser)

	timeTag := fmt.Sprintf("%s[%s]%s", fgGray, t, reset)
	nameTag := name
	if me {
		nameTag = fmt.Sprintf("%s%s%s", fgGreen, name, reset)
	} else {
		nameTag = fmt.Sprintf("%s%s%s", fgYellow, name, reset)
	}
	fmt.Printf("\r%s %s │ %s\n", timeTag, nameTag, im.Msg)
}

// —— 解码工具 —— //

func decodeInnerMsg(msgAny interface{}) *InnerMsg {
	raw := decodeInnerRaw(msgAny)
	if len(raw) == 0 {
		return nil
	}
	var im InnerMsg
	if err := json.Unmarshal(raw, &im); err != nil {
		return nil
	}
	return &im
}

func decodeInnerRaw(msgAny interface{}) []byte {
	switch v := msgAny.(type) {
	case string:
		if b, err := base64.StdEncoding.DecodeString(v); err == nil {
			return b
		}
		return []byte(v)
	case []byte:
		if b, err := base64.StdEncoding.DecodeString(string(v)); err == nil {
			return b
		}
		return v
	case map[string]interface{}:
		b, _ := json.Marshal(v)
		return b
	default:
		return nil
	}
}

func asInt(x interface{}) int {
	switch v := x.(type) {
	case float64:
		return int(v)
	case int:
		return v
	case int64:
		return int(v)
	case json.Number:
		i, _ := v.Int64()
		return int(i)
	default:
		return 0
	}
}

func asString(x interface{}) string {
	switch v := x.(type) {
	case string:
		return v
	case []byte:
		return string(v)
	default:
		return fmt.Sprintf("%v", v)
	}
}

// 当内层解不出来时，用外层字段组装一条 InnerMsg
func innerFromOuter(evt map[string]interface{}) *InnerMsg {
	return &InnerMsg{
		Code:         asInt(evt["code"]),
		Msg:          asString(evt["msg"]),
		FromUserId:   asInt(evt["fromUserId"]),
		FromUserName: asString(evt["fromUserName"]),
		ToUserId:     asInt(evt["toUserId"]),
		ToUserName:   asString(evt["toUserName"]),
		RoomId:       asInt(evt["roomId"]),
		Op:           asInt(evt["op"]),
		CreateTime:   asString(evt["createTime"]),
	}
}

func stringsTrim(s string) string { return strings.TrimSpace(s) }

func clearScreen() {
	fmt.Print("\x1b[2J\x1b[H") // 清屏并移动到左上角
}
