package main

import (
	"errors"
	"flag"
	"fmt"
	"net"
	"os"
	"strconv"
)

type config struct {
	addr, dataDir string
	selfcheck     bool
}

func parseConfig() (config, error) {
	var c config
	flag.StringVar(&c.addr, "addr", "127.0.0.1:19081", "HTTP 监听地址")
	flag.StringVar(&c.dataDir, "data", "./data", "本地数据目录")
	flag.BoolVar(&c.selfcheck, "selfcheck", false, "运行全流程自检后退出")
	flag.Parse()
	explicit := false
	flag.Visit(func(f *flag.Flag) {
		if f.Name == "addr" {
			explicit = true
		}
	})
	if !explicit {
		if p := os.Getenv("PORT"); p != "" {
			port, err := strconv.Atoi(p)
			if err != nil || port < 1 || port > 65535 {
				return c, errors.New("PORT 必须是有效端口号")
			}
			c.addr = fmt.Sprintf("127.0.0.1:%d", port)
		}
	}
	host, port, err := net.SplitHostPort(c.addr)
	if err != nil || host == "" || port == "" {
		return c, errors.New("-addr 必须是完整的 host:port")
	}
	if c.selfcheck && net.ParseIP(host) == nil {
		return c, errors.New("自检地址必须使用 IP 回环地址")
	}
	if c.selfcheck && !net.ParseIP(host).IsLoopback() {
		return c, errors.New("自检禁止使用非回环地址")
	}
	return c, nil
}
