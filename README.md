# onehub-sync

一键同步各 AI 渠道模型列表到 one-api / onehub 数据库。

自动从 OpenAI、DeepSeek、Gemini、Groq、Anthropic 等渠道拉取最新可用模型，批量写入数据库，省去手动维护的麻烦。

## 功能

- 自动识别渠道类型并获取对应 API 的模型列表
- 并发处理，实时终端进度展示
- 新旧模型对比，记录变更详情
- 支持清除所有渠道模型
- 支持调试模式输出详细日志

## 编译

```bash
go build -o model-updater main.go
```

## 使用

```bash
# 正常运行
./model-updater

# 调试模式
./model-updater -debug

# 指定数据库路径
./model-updater -db /path/to/one-api.db

# 清除所有渠道模型
./model-updater -clear-all
```

## 参数说明

| 参数 | 默认值 | 说明 |
|------|--------|------|
| `-db` | `/root/onehub/one-api.db` | 数据库文件路径 |
| `-clear-all` | `false` | 清除所有渠道的模型 |
| `-debug` | `false` | 启用调试模式，显示详细日志 |
| `-help` | `false` | 显示帮助信息 |

## Python 脚本

`update_models.py` 是一个轻量级的 Python 替代版本，功能类似但更简单。

```bash
python3 update_models.py
```

## 依赖

- Go 1.24+
- `github.com/mattn/go-sqlite3`

## 许可证

[MIT](LICENSE)
