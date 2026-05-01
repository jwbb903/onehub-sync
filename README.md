# onehub 模型更新工具

自动从各渠道 API 获取最新模型列表，批量更新 one-api/onehub 数据库。

## 功能

- 自动识别渠道类型（OpenAI、DeepSeek、Gemini、Groq、Anthropic 等）
- 并发获取模型列表，实时显示进度
- 对比新旧模型列表，记录变更
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
