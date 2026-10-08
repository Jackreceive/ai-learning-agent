// Package server 提供 HTTP 服务
package server

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"strings"

	"github.com/cloudwego/eino-ext/components/model/ark"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/flow/agent/react"
	"github.com/cloudwego/eino/schema"
	"github.com/google/uuid"

	"github.com/Jackreceive/ai-learning-agent/config"
	"github.com/Jackreceive/ai-learning-agent/model"
	"github.com/Jackreceive/ai-learning-agent/rag"
	"github.com/Jackreceive/ai-learning-agent/tools"
)

//go:embed static
var staticFiles embed.FS

// Server HTTP 服务
type Server struct {
	chatModel *ark.ChatModel
	rag       *rag.Components
	tools     []tool.BaseTool
}

// New 创建服务实例
func New(cfg *config.Config) (*Server, error) {
	ctx := context.Background()

	chatModel, err := model.NewChatModel(ctx, &cfg.ARK)
	if err != nil {
		return nil, fmt.Errorf("创建模型失败: %w", err)
	}

	components, err := rag.New(ctx, cfg)
	if err != nil {
		return nil, err
	}

	return &Server{
		chatModel: chatModel,
		rag:       components,
		tools:     tools.GetAllTools(),
	}, nil
}

// Run 启动服务
func (s *Server) Run(port int) error {
	defer s.rag.Client.Close(context.Background())
	mux := http.NewServeMux()

	// API 路由
	mux.HandleFunc("/api/chat", s.handleChat)
	mux.HandleFunc("/api/chat/stream", s.handleChatStream)
	mux.HandleFunc("/api/knowledge/query", s.handleKnowledgeQuery)
	mux.HandleFunc("/api/knowledge/add", s.handleKnowledgeAdd)
	mux.HandleFunc("/api/knowledge/clear", s.handleKnowledgeClear)
	mux.HandleFunc("/api/knowledge/count", s.handleKnowledgeCount)
	mux.HandleFunc("/api/tools", s.handleTools)
	mux.HandleFunc("/api/code", s.handleCode)
	mux.HandleFunc("/api/translate", s.handleTranslate)

	staticFS, err := fs.Sub(staticFiles, "static")
	if err != nil {
		return err
	}
	mux.Handle("/", http.FileServer(http.FS(staticFS)))

	return http.ListenAndServe(fmt.Sprintf(":%d", port), mux)
}

func (s *Server) handleKnowledgeCount(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	count, err := s.rag.Count(r.Context())
	if err != nil {
		writeJSON(w, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, map[string]any{"count": count})
}

// ChatRequest 对话请求
type ChatRequest struct {
	Message string           `json:"message"`
	History []HistoryMessage `json:"history,omitempty"`
	Mode    string           `json:"mode,omitempty"` // chat, code, translate
}

type HistoryMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ChatResponse 对话响应
type ChatResponse struct {
	Content string `json:"content"`
	Error   string `json:"error,omitempty"`
}

// handleChat 处理对话请求
func (s *Server) handleChat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req ChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, ChatResponse{Error: "无效的请求"})
		return
	}

	ctx := r.Context()
	messages := s.buildMessages(req, "你是一个友好的 AI 学习助手，回答简洁清晰。")

	response, err := s.chatModel.Generate(ctx, messages)
	if err != nil {
		writeJSON(w, ChatResponse{Error: err.Error()})
		return
	}

	writeJSON(w, ChatResponse{Content: response.Content})
}

// handleChatStream 处理流式对话
func (s *Server) handleChatStream(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req ChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "无效的请求", http.StatusBadRequest)
		return
	}

	// 设置 SSE 头
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming not supported", http.StatusInternalServerError)
		return
	}

	ctx := r.Context()

	// 根据模式选择系统提示
	systemPrompt := "你是一个友好的 AI 学习助手，回答简洁清晰。"
	if req.Mode == "code" {
		systemPrompt = "你是编程助手，代码用markdown代码块，解释简洁。"
	} else if req.Mode == "translate" {
		systemPrompt = "翻译助手：中文翻英文，英文翻中文，只输出译文。"
	}

	messages := s.buildMessages(req, systemPrompt)

	stream, err := s.chatModel.Stream(ctx, messages)
	if err != nil {
		data, _ := json.Marshal(map[string]string{"error": err.Error()})
		fmt.Fprintf(w, "data: %s\n\n", data)
		flusher.Flush()
		return
	}
	defer stream.Close()

	for {
		chunk, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			data, _ := json.Marshal(map[string]string{"error": err.Error()})
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
			return
		}
		if chunk != nil && chunk.Content != "" {
			data, _ := json.Marshal(map[string]string{"content": chunk.Content})
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
		}
	}

	fmt.Fprintf(w, "data: {\"done\": true}\n\n")
	flusher.Flush()
}

// handleKnowledgeQuery 知识库查询
func (s *Server) handleKnowledgeQuery(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req ChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, ChatResponse{Error: "无效的请求"})
		return
	}

	ctx := r.Context()

	// 检索知识
	docs, err := s.rag.Retriever.Retrieve(ctx, req.Message)
	ragContext := rag.BuildContext(docs)
	if err != nil {
		writeJSON(w, ChatResponse{Error: err.Error()})
		return
	}

	// 构建消息
	systemPrompt := fmt.Sprintf(`基于以下资料回答问题，资料不足请说明：

%s`, ragContext)

	messages := []*schema.Message{
		schema.SystemMessage(systemPrompt),
		schema.UserMessage(req.Message),
	}

	response, err := s.chatModel.Generate(ctx, messages)
	if err != nil {
		writeJSON(w, ChatResponse{Error: err.Error()})
		return
	}

	content := response.Content
	if len(docs) > 0 {
		content += fmt.Sprintf("\n\n📎 参考了 %d 条知识", len(docs))
	}

	writeJSON(w, ChatResponse{Content: content})
}

// KnowledgeRequest 知识请求
type KnowledgeRequest struct {
	Content string `json:"content"`
}

// handleKnowledgeAdd 添加知识
func (s *Server) handleKnowledgeAdd(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req KnowledgeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, map[string]interface{}{"success": false, "error": "无效的请求"})
		return
	}

	if strings.TrimSpace(req.Content) == "" {
		writeJSON(w, map[string]interface{}{"success": false, "error": "内容不能为空"})
		return
	}

	ctx := r.Context()
	docs, err := s.rag.Transformer.Transform(ctx, []*schema.Document{{ID: uuid.NewString(), Content: req.Content, MetaData: map[string]any{"source": "user"}}})
	if err == nil {
		_, err = s.rag.Indexer.Store(ctx, docs)
	}
	if err != nil {
		writeJSON(w, map[string]any{"success": false, "error": err.Error()})
		return
	}
	count, err := s.rag.Count(ctx)
	if err != nil {
		writeJSON(w, map[string]any{"success": true, "warning": "知识已保存，但读取总数失败: " + err.Error()})
		return
	}

	writeJSON(w, map[string]interface{}{"success": true, "count": count})
}

// handleKnowledgeClear 清空知识库
func (s *Server) handleKnowledgeClear(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if err := s.rag.Clear(r.Context()); err != nil {
		writeJSON(w, map[string]any{"success": false, "error": err.Error()})
		return
	}
	writeJSON(w, map[string]interface{}{"success": true})
}

// ToolRequest 工具请求
type ToolRequest struct {
	Query string `json:"query"`
}

// handleTools 处理工具调用
func (s *Server) handleTools(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req ToolRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, ChatResponse{Error: "无效的请求"})
		return
	}

	ctx := r.Context()

	agent, err := react.NewAgent(ctx, &react.AgentConfig{
		ToolCallingModel: s.chatModel, ToolsConfig: compose.ToolsNodeConfig{Tools: s.tools}, MaxStep: 10,
	})
	if err != nil {
		writeJSON(w, ChatResponse{Error: err.Error()})
		return
	}
	response, err := agent.Generate(ctx, []*schema.Message{
		schema.SystemMessage("根据用户需求调用工具，并用工具结果回答问题。"), schema.UserMessage(req.Query),
	})
	if err != nil {
		writeJSON(w, ChatResponse{Error: err.Error()})
		return
	}
	writeJSON(w, ChatResponse{Content: response.Content})
}

// handleCode 代码助手
func (s *Server) handleCode(w http.ResponseWriter, r *http.Request) {
	s.handleChatWithPrompt(w, r, "你是编程助手，代码用markdown代码块，解释简洁。")
}

// handleTranslate 翻译助手
func (s *Server) handleTranslate(w http.ResponseWriter, r *http.Request) {
	s.handleChatWithPrompt(w, r, "翻译助手：中文翻英文，英文翻中文，只输出译文。")
}

func (s *Server) handleChatWithPrompt(w http.ResponseWriter, r *http.Request, systemPrompt string) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req ChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, ChatResponse{Error: "无效的请求"})
		return
	}

	ctx := r.Context()
	messages := []*schema.Message{
		schema.SystemMessage(systemPrompt),
		schema.UserMessage(req.Message),
	}

	response, err := s.chatModel.Generate(ctx, messages)
	if err != nil {
		writeJSON(w, ChatResponse{Error: err.Error()})
		return
	}

	writeJSON(w, ChatResponse{Content: response.Content})
}

func (s *Server) buildMessages(req ChatRequest, systemPrompt string) []*schema.Message {
	messages := []*schema.Message{
		schema.SystemMessage(systemPrompt),
	}

	for _, h := range req.History {
		if h.Role == "user" {
			messages = append(messages, schema.UserMessage(h.Content))
		} else {
			messages = append(messages, schema.AssistantMessage(h.Content, nil))
		}
	}

	messages = append(messages, schema.UserMessage(req.Message))
	return messages
}

func writeJSON(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}
