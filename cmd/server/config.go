package main

import (
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
)

const defaultAddr = "127.0.0.1:19091"

func configuredAddr(flagValue string) (string, error) {
	value := flagValue
	if value == defaultAddr {
		if port := os.Getenv("PORT"); port != "" {
			n, err := strconv.Atoi(port)
			if err != nil || n < 1024 || n > 65535 {
				return "", fmt.Errorf("PORT 必须是 1024 到 65535 的端口号")
			}
			value = net.JoinHostPort("127.0.0.1", port)
		}
	}
	host, port, err := net.SplitHostPort(value)
	if err != nil {
		return "", fmt.Errorf("-addr 格式无效: %w", err)
	}
	if host != "127.0.0.1" && host != "localhost" && host != "::1" {
		return "", fmt.Errorf("监听地址必须使用回环主机")
	}
	n, err := strconv.Atoi(port)
	if err != nil || n < 1024 || n > 65535 {
		return "", fmt.Errorf("监听端口必须在 1024 到 65535 之间")
	}
	if strings.TrimSpace(host) == "" {
		return "", fmt.Errorf("监听主机不能为空")
	}
	return value, nil
}
