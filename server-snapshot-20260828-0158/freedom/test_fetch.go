// +build ignore

package main

import (
	"fmt"
	"io"
	"net/http"
	"time"
)

func main() {
	// 测试连接 youmind.com
	url := "https://youmind.com/zh-CN/seedance-2-0-prompts"
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	fmt.Printf("Status: %d, Length: %d\n", resp.StatusCode, len(body))
	// 输出前2000字符看看结构
	if len(body) > 2000 {
		fmt.Println(string(body[:2000]))
	} else {
		fmt.Println(string(body))
	}
}
