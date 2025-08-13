package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/pkg/errors"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/segmentio/kafka-go"
)

/*********** 消息结构（与 Logic 层/Task 层约定） ***********/
type Job struct {
	Op         string `json:"op"` // "ask" | "summarize" | "translate"
	RoomID     int    `json:"roomId"`
	FromUserID int    `json:"fromUserId"`
	FromName   string `json:"fromUserName"`
	Prompt     string `json:"prompt"` // /ai 的问题；/translate 的文本；/summarize 可留空（通常从DB取最近N条）
	Lang       string `json:"lang"`   // translate 目标语言（如 "en", "ja", "zh"）
}

type Result struct {
	RoomID int    `json:"roomId"`
	Text   string `json:"text"`
	Op     string `json:"op"`    // 原样回传：ask/summarize/translate
	Model  string `json:"model"` // 实际使用的模型名
	Err    string `json:"err,omitempty"`
}

/*********** 环境配置 ***********/
var (
	brokers  = getenv("KAFKA_BROKERS", "localhost:9092")
	inTopic  = getenv("AI_JOBS_TOPIC", "ai.jobs")       // 读
	outTopic = getenv("AI_RESULTS_TOPIC", "ai.results") // 写

	// 模型提供方：ollama（本地） | kimi（Moonshot）
	provider = getenv("AI_PROVIDER", "ollama")

	// ollama: http://localhost:11434
	// kimi:   https://api.moonshot.cn 或 https://api.moonshot.ai
	baseURL = getenv("AI_BASE_URL", "http://localhost:11434")

	// 模型名称：ollama 例如 llama3.1；kimi 例如 kimi-k2 / moonshot-v1-8k / -32k / -128k
	model  = getenv("AI_MODEL", "llama3.1")
	apiKey = os.Getenv("AI_API_KEY") // kimi 需要
)

var httpc = &http.Client{Timeout: 180 * time.Second}

/*********** 主程序 ***********/
func main() {
	r := kafka.NewReader(kafka.ReaderConfig{
		Brokers:  splitCSV(brokers),
		Topic:    inTopic,
		GroupID:  "ai-worker", // 同组可水平扩容
		MinBytes: 1,
		MaxBytes: 10 << 20,
	})
	w := &kafka.Writer{
		Addr:                   kafka.TCP(splitCSV(brokers)...),
		Topic:                  outTopic,
		AllowAutoTopicCreation: true,
		BatchTimeout:           10 * time.Millisecond,
	}
	defer r.Close()
	defer w.Close()

	log.Printf("[AI] worker started | provider=%s model=%s | in=%s out=%s", provider, model, inTopic, outTopic)

	ctx := context.Background()
	for {
		msg, err := r.ReadMessage(ctx)
		if err != nil {
			log.Printf("[AI] read err: %v", err)
			time.Sleep(time.Second)
			continue
		}

		var job Job
		if err := json.Unmarshal(msg.Value, &job); err != nil {
			log.Printf("[AI] bad job json: %s", string(msg.Value))
			continue
		}

		prompt := buildPrompt(job)
		answer, usedModel, callErr := callLLM(prompt, job)

		res := Result{
			RoomID: job.RoomID,
			Text:   answer,
			Op:     job.Op,
			Model:  usedModel,
		}
		if callErr != nil {
			res.Err = callErr.Error()
		}

		out, _ := json.Marshal(res)
		if err := w.WriteMessages(ctx, kafka.Message{Value: out}); err != nil {
			log.Printf("[AI] write result err: %v", err)
		}
	}
}

/*********** Prompt 构造（按业务调整即可） ***********/
func buildPrompt(job Job) string {
	switch job.Op {
	case "ask":
		// 直接把用户问题交给模型
		return job.Prompt

	case "translate":
		// 简单翻译提示词（你也可以改成带风格、术语表等指令）
		lang := job.Lang
		if lang == "" {
			lang = "en"
		}
		return fmt.Sprintf("Translate into %s: %s", lang, job.Prompt)

	case "summarize":
		// 真实场景：应从 DB 取最近 N 条消息拼接；这里给简单模板
		if strings.TrimSpace(job.Prompt) == "" {
			return "请用中文简要总结这段群聊（要点化、合并重复、给出结论）。\n[聊天节选]\n(这里应拼接最近N条消息)"
		}
		return "请用中文简要总结这段群聊（要点化、合并重复、给出结论）：\n" + job.Prompt
	}
	// 未知 op：当作普通提问
	return job.Prompt
}

/*********** LLM 调用 ***********/
func callLLM(prompt string, job Job) (string, string, error) {
	switch provider {
	case "ollama":
		return callOllama(prompt)
	case "kimi":
		return callKimi(prompt)
	default:
		return "", model, fmt.Errorf("unknown AI_PROVIDER: %s", provider)
	}
}

// “流式”不是“一行行把请求打出来”，而是一次发请求，服务端把响应分片（token/句子）实时往回推。你这次把 stream=true 开了，所以：
//
// 服务器会立刻返回响应头并保持连接；
//
// 之后不断写入很多小 JSON 块（Ollama 是 NDJSON：一行一个 JSON，最后有 {"done":true}）；
//
// 你的客户端边读边拼接 → 不用等整段生成完，能更快看到结果，也避免“等待响应头超时”。
// 普通流式：
func callOllama(prompt string) (string, string, error) {
	req := map[string]any{
		"model": model,
		"messages": []map[string]string{
			{"role": "system", "content": "You are a helpful assistant. Prefer Chinese when appropriate."},
			{"role": "user", "content": prompt},
		},
		"stream": true, // 开流
	}
	b, _ := json.Marshal(req)

	resp, err := httpc.Post(strings.TrimRight(baseURL, "/")+"/api/chat", "application/json", bytes.NewBuffer(b))
	if err != nil {
		return "", model, err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		rb, _ := io.ReadAll(resp.Body)
		return "", model, fmt.Errorf("ollama %d: %s", resp.StatusCode, string(rb))
	}

	// 按行读取增量 JSON（每行一个 JSON，包含 done 字段）
	dec := json.NewDecoder(resp.Body)
	var buf strings.Builder
	used := model
	for dec.More() {
		var chunk struct {
			Model   string `json:"model"`
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
			Done bool `json:"done"`
		}
		if err := dec.Decode(&chunk); err != nil {
			// 流式时，服务端常用 \n 分隔；必要时可用 bufio.Scanner 按行再 json.Unmarshal
			if errors.Is(err, io.EOF) {
				break
			}
			return "", used, fmt.Errorf("ollama stream decode: %w", err)
		}
		if chunk.Model != "" {
			used = chunk.Model
		}
		if chunk.Message.Content != "" {
			buf.WriteString(chunk.Message.Content)
		}
		if chunk.Done {
			break
		}
	}
	if buf.Len() == 0 {
		return "", used, fmt.Errorf("empty response")
	}
	return buf.String(), used, nil
}

// 稳健流式：
func callOllamaStream(prompt string) (string, string, error) {
	req := map[string]any{
		"model": model,
		"messages": []map[string]string{
			{"role": "system", "content": "You are a helpful assistant. Prefer Chinese when appropriate."},
			{"role": "user", "content": prompt},
		},
		"stream": true,
	}
	body, _ := json.Marshal(req)

	// 建议：client 不设全局 Timeout，改用 Transport 的 ResponseHeaderTimeout
	// 并给这次请求一个可取消的 ctx（整体 5 分钟示例）
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	httpReq, _ := http.NewRequestWithContext(ctx, "POST", strings.TrimRight(baseURL, "/")+"/api/chat", bytes.NewBuffer(body))
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := httpc.Do(httpReq)
	if err != nil {
		return "", model, err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		b, _ := io.ReadAll(resp.Body)
		return "", model, fmt.Errorf("ollama %d: %s", resp.StatusCode, string(b))
	}

	// 按行读（NDJSON），并增大扫描缓冲防止长行
	sc := bufio.NewScanner(resp.Body)
	buf := make([]byte, 0, 1<<20) // 1MB
	sc.Buffer(buf, 10<<20)        // 10MB
	var out strings.Builder
	used := model

	for sc.Scan() {
		line := sc.Bytes()
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		var chunk struct {
			Model   string `json:"model"`
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
			Done bool `json:"done"`
		}
		if err := json.Unmarshal(line, &chunk); err != nil {
			// 有些环境可能一行不是完整 JSON，可换 json.Decoder 或手动切分
			continue
		}
		if chunk.Model != "" {
			used = chunk.Model
		}
		if chunk.Message.Content != "" {
			out.WriteString(chunk.Message.Content)
			// 这里也可以把增量内容发到 Kafka/WS 做“正在输入”效果
		}
		if chunk.Done {
			break
		}
	}
	if err := sc.Err(); err != nil {
		return out.String(), used, err
	}
	return out.String(), used, nil
}

// Kimi (Moonshot) /v1/chat/completions（不开流）
func callKimi(prompt string) (string, string, error) {
	if apiKey == "" {
		return "", model, fmt.Errorf("missing AI_API_KEY for kimi")
	}

	reqBody := map[string]any{
		"model": model, // kimi-k2 / moonshot-v1-8k / -32k / -128k
		"messages": []map[string]string{
			{"role": "system", "content": "You are Kimi by Moonshot AI. Prefer Chinese when appropriate."},
			{"role": "user", "content": prompt},
		},
		"temperature": 0.3,
		"stream":      false,
	}
	b, _ := json.Marshal(reqBody)

	req, _ := http.NewRequest("POST", strings.TrimRight(baseURL, "/")+"/v1/chat/completions", bytes.NewBuffer(b))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := httpc.Do(req)
	if err != nil {
		return "", model, err
	}
	defer resp.Body.Close()
	rb, _ := io.ReadAll(resp.Body)
	if resp.StatusCode/100 != 2 {
		return "", model, fmt.Errorf("kimi %d: %s", resp.StatusCode, string(rb))
	}

	var out struct {
		Model   string `json:"model"`
		Choices []struct {
			Message struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(rb, &out); err != nil {
		return "", model, fmt.Errorf("kimi decode: %w, raw=%s", err, string(rb))
	}
	used := out.Model
	if used == "" {
		used = model
	}
	if len(out.Choices) == 0 {
		return "", used, fmt.Errorf("kimi: empty choices")
	}
	return out.Choices[0].Message.Content, used, nil
}

/*********** 小工具 ***********/
func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func splitCSV(s string) []string {
	var xs []string
	for _, p := range strings.Split(s, ",") {
		if q := strings.TrimSpace(p); q != "" {
			xs = append(xs, q)
		}
	}
	return xs
}
