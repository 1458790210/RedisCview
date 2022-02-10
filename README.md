# RedisCview

 web 与命令行的 redis 管理工具

<p align="center">
  <img src="https://github.com/1458790210/RedisCview/blob/master/web.png">
  <img src="https://github.com/1458790210/RedisCview/blob/master/client.png">
</p>

## 特性：

1. 提供websocket、命令行窗口、web界面三种方式
2. 单文件可执行，无需安装配置
3. 简洁美观的操作界面

## 使用：

```
cd 项目根目录 go run main.go # 启动服务

不需要windows窗口执行：go build -ldflags "-s -w -H=windowsgui"
```