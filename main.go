package main

import (
	"context"
	"fmt"
	"os"

	"github.com/cloudwego/eino-ext/components/model/deepseek"
	"github.com/cloudwego/eino/schema"
	"github.com/joho/godotenv"
)

func main() {
	err := godotenv.Load(".env")
	if err != nil {
		panic(err)
	}
	ctx := context.Background()
	chatModel, err := deepseek.NewChatModel(ctx, &deepseek.ChatModelConfig{
		APIKey:  os.Getenv("deepseek_api_key"),
		Model:   "deepseek-chat",
		BaseURL: "https://api.deepseek.com",
	})
	if err != nil {
		panic(err)
	}
	input := []*schema.Message{
		schema.SystemMessage("你是一个知识渊博的篮球解说员"),
		schema.UserMessage("你好，请介绍一下Kobe Bryant 的职业生涯"),
	}
	message, err := chatModel.Generate(ctx, input)
	if err != nil {
		panic(err)
	}

	fmt.Printf("AI 响应：%s\n", message.Content)
}
