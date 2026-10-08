// AI 学习助手
package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/Jackreceive/ai-learning-agent/config"
	"github.com/Jackreceive/ai-learning-agent/server"
	"github.com/joho/godotenv"
)

func main() {
	// 命令行参数
	port := flag.Int("port", 8080, "服务端口")
	flag.Parse()

	godotenv.Load(".env")
	// 加载配置
	cfg, err := config.Load()
	if err != nil {
		fmt.Printf("❌ 配置错误: %v\n", err)
		fmt.Println("请确保已设置环境变量: ARK_API_KEY, ARK_MODEL_NAME, ARK_EMBEDDING_MODEL")
		os.Exit(1)
	}

	// 创建并启动服务
	srv, err := server.New(cfg)
	if err != nil {
		log.Fatalf("❌ 初始化失败: %v", err)
	}

	fmt.Println()
	fmt.Println("╔════════════════════════════════════════════╗")
	fmt.Println("║      🎓 AI 学习助手 - Eino 教学示例        ║")
	fmt.Println("╚════════════════════════════════════════════╝")
	fmt.Println()
	fmt.Printf("🚀 服务已启动: http://localhost:%d\n", *port)
	fmt.Println()

	if err := srv.Run(*port); err != nil {
		log.Fatalf("服务启动失败: %v", err)
	}
}
