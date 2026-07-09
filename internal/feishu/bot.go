package feishu

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/larksuite/oapi-sdk-go/v3/event/dispatcher"
	"github.com/larksuite/oapi-sdk-go/v3/event/dispatcher/callback"
	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"
	larkws "github.com/larksuite/oapi-sdk-go/v3/ws"
	"github.com/mircexiao/go-tiny-claw/internal/engine"
	"github.com/mircexiao/go-tiny-claw/internal/schema"

	lark "github.com/larksuite/oapi-sdk-go/v3"

	ctxpkg "github.com/mircexiao/go-tiny-claw/internal/context"
)

type reporterKey struct{}

func ContextWithReporter(ctx context.Context, r engine.Reporter) context.Context {
	return context.WithValue(ctx, reporterKey{}, r)
}

func ReporterFromContext(ctx context.Context) engine.Reporter {
	return ctx.Value(reporterKey{}).(engine.Reporter)
}

type AgentEngineFactory func(sess *ctxpkg.Session) *engine.AgentEngine
type FeishuBot struct {
	client    *lark.Client
	appID     string
	appSecret string
	workDir   string
	factory   AgentEngineFactory
}

func NewFeishuBotFactory(factory AgentEngineFactory, workDir string) *FeishuBot {
	appID := os.Getenv("FEISHU_APP_ID")
	appSecret := os.Getenv("FEISHU_APP_SECRET")

	if appID == "" || appSecret == "" {
		log.Fatal("请设置 FEISHU_APP_ID 和 FEISHU_APP_SECRET")
	}

	client := lark.NewClient(appID, appSecret)

	return &FeishuBot{
		client:    client,
		appID:     appID,
		appSecret: appSecret,
		workDir:   workDir,
		factory:   factory,
	}
}

func (b *FeishuBot) GetEventDispatcher() *dispatcher.EventDispatcher {
	encryptKey := os.Getenv("FEISHU_ENCRYPT_KEY")
	verifyToken := os.Getenv("FEISHU_VERIFY_TOKEN")

	handler := dispatcher.NewEventDispatcher(verifyToken, encryptKey).
		OnP2MessageReceiveV1(func(ctx context.Context, event *larkim.P2MessageReceiveV1) error {
			contentStr := *event.Event.Message.Content
			contentStr = strings.TrimPrefix(contentStr, `{"text":"`)
			contentStr = strings.TrimSuffix(contentStr, `"}`)

			chatId := *event.Event.Message.ChatId
			log.Printf("[Feishu] 收到会话 %s 消息: %s\n", chatId, contentStr)

			// 拦截人工审批的特殊口令(保留向后兼容)
			if strings.HasPrefix(contentStr, "approve ") {
				taskID := strings.TrimPrefix(contentStr, "approve ")
				taskID = strings.TrimSpace(taskID)
				GlobalApprovalManager.ResolveApproval(taskID, true, "人类管理员已批准操作")
				log.Printf("[Feishu] 会话 %s: ✅ 已为您批准任务 %s", chatId, taskID)
				return nil
			}
			if strings.HasPrefix(contentStr, "reject ") {
				taskID := strings.TrimPrefix(contentStr, "reject ")
				taskID = strings.TrimSpace(taskID)
				GlobalApprovalManager.ResolveApproval(taskID, false, "人类管理员认为该操作存在极高风险，已无情拒绝")
				log.Printf("[Feishu] 会话 %s: 🚫 已拒绝任务 %s", chatId, taskID)
				return nil
			}

			// 如果不是审批命令，则是正常对话，启动一个新的 Agent 任务去处理
			go b.handleAgentRun(chatId, contentStr)

			return nil
		}).
		OnP2CardActionTrigger(func(ctx context.Context, event *callback.CardActionTriggerEvent) (*callback.CardActionTriggerResponse, error) {
			// 处理卡片按钮点击事件
			actionValue := event.Event.Action.Value
			if actionValue == nil {
				return &callback.CardActionTriggerResponse{}, nil
			}

			action, _ := actionValue["action"].(string)
			taskID, _ := actionValue["taskID"].(string)

			if taskID == "" {
				return &callback.CardActionTriggerResponse{}, nil
			}

			if action == "approve" {
				GlobalApprovalManager.ResolveApproval(taskID, true, "人类管理员通过卡片按钮批准操作")
				log.Printf("[Feishu] 卡片审批: ✅ 已批准任务 %s", taskID)
				cardStr := buildApprovalResultCard(taskID, true)
				return &callback.CardActionTriggerResponse{
					Card: &callback.Card{
						Type: "raw",
						Data: cardStr,
					},
				}, nil
			} else if action == "reject" {
				GlobalApprovalManager.ResolveApproval(taskID, false, "人类管理员通过卡片按钮拒绝操作")
				log.Printf("[Feishu] 卡片审批: 🚫 已拒绝任务 %s", taskID)
				cardStr := buildApprovalResultCard(taskID, false)
				return &callback.CardActionTriggerResponse{
					Card: &callback.Card{
						Type: "raw",
						Data: cardStr,
					},
				}, nil
			}

			return &callback.CardActionTriggerResponse{}, nil
		}).
		OnP2MessageReadV1(func(ctx context.Context, event *larkim.P2MessageReadV1) error {
			// 消息已读事件，静默忽略
			return nil
		})

	return handler
}

func (b *FeishuBot) StartLongConnection() error {
	eventDispatcher := b.GetEventDispatcher()

	wsClient := larkws.NewClient(b.appID, b.appSecret,
		larkws.WithEventHandler(eventDispatcher),
	)

	log.Println("🔌 正在建立飞书 WebSocket 长连接...")
	err := wsClient.Start(context.Background())
	if err != nil {
		return fmt.Errorf("飞书长连接启动失败: %w", err)
	}

	return nil
}

func (b *FeishuBot) handleAgentRun(chatId string, prompt string) {
	reporter := &FeishuReporter{
		client: b.client,
		chatId: chatId,
	}
	sess := ctxpkg.GlobalSessionManager.GetOrCreate(chatId, b.workDir)
	sess.Append(schema.Message{Role: schema.RoleUser, Content: prompt})
	eng := b.factory(sess)
	runCtx := ContextWithReporter(context.Background(), reporter)
	err := eng.Run(runCtx, sess, reporter)
	if err != nil {
		reporter.sendMessage(fmt.Sprintf("❌ Agent 运行崩溃: %v", err))
	}
}

type FeishuReporter struct {
	client *lark.Client
	chatId string

	// 流式输出相关
	streamMu       sync.Mutex
	streamMsgId    string          // 飞书消息 ID，用于 Patch
	streamContent  strings.Builder // 累积的完整文本
	streamDirty    bool            // 是否有未刷新的内容
	streamLastPatch time.Time      // 上次 Patch 时间，用于限流
}

func (r *FeishuReporter) sendMessage(text string) {
	// Build text message content
	textContent := map[string]string{
		"text": text,
	}
	contentBytes, _ := json.Marshal(textContent)
	contentStr := string(contentBytes)

	msgReq := larkim.NewCreateMessageReqBuilder().
		ReceiveIdType("chat_id").
		Body(larkim.NewCreateMessageReqBodyBuilder().
			ReceiveId(r.chatId).
			MsgType(larkim.MsgTypeText).
			Content(contentStr).
			Build()).
		Build()

	_, _ = r.client.Im.Message.Create(context.Background(), msgReq)
}

func (r *FeishuReporter) OnThinking(ctx context.Context) {
	r.sendMessage("🤔 模型正在慢思考 (Thinking)...")
}

func (r *FeishuReporter) sendCardMessage(cardContent string) {
	msgReq := larkim.NewCreateMessageReqBuilder().
		ReceiveIdType("chat_id").
		Body(larkim.NewCreateMessageReqBodyBuilder().
			ReceiveId(r.chatId).
			MsgType(larkim.MsgTypeInteractive).
			Content(cardContent).
			Build()).
		Build()
	_, _ = r.client.Im.Message.Create(context.Background(), msgReq)
}

// createStreamMessage 发送首条流式消息，返回 messageId
func (r *FeishuReporter) createStreamMessage(text string) string {
	textContent := map[string]string{"text": text}
	contentBytes, _ := json.Marshal(textContent)
	contentStr := string(contentBytes)

	msgReq := larkim.NewCreateMessageReqBuilder().
		ReceiveIdType("chat_id").
		Body(larkim.NewCreateMessageReqBodyBuilder().
			ReceiveId(r.chatId).
			MsgType(larkim.MsgTypeText).
			Content(contentStr).
			Build()).
		Build()

	resp, err := r.client.Im.Message.Create(context.Background(), msgReq)
	if err != nil || !resp.Success() {
		log.Printf("[Feishu] 创建流式消息失败: %v", err)
		return ""
	}
	return *resp.Data.MessageId
}

// patchStreamMessage 更新已发送消息的内容
func (r *FeishuReporter) patchStreamMessage(text string) {
	if r.streamMsgId == "" {
		return
	}
	textContent := map[string]string{"text": text}
	contentBytes, _ := json.Marshal(textContent)
	contentStr := string(contentBytes)

	patchReq := larkim.NewPatchMessageReqBuilder().
		MessageId(r.streamMsgId).
		Body(larkim.NewPatchMessageReqBodyBuilder().
			Content(contentStr).
			Build()).
		Build()

	_, err := r.client.Im.Message.Patch(context.Background(), patchReq)
	if err != nil {
		log.Printf("[Feishu] Patch 消息失败: %v", err)
	}
}

func (r *FeishuReporter) OnToolCall(ctx context.Context, toolName string, args string) {
	r.sendMessage(fmt.Sprintf("🛠️ **正在执行工具**：`%s`\n参数：`%s`", toolName, args))
}

func (r *FeishuReporter) OnToolResult(ctx context.Context, toolName string, result string, isError bool) {
	if isError {
		r.sendMessage(fmt.Sprintf("⚠️ **执行报错** (%s)：\n%s", toolName, result))
	} else {
		r.sendMessage(fmt.Sprintf("✅ **执行成功** (%s)", toolName))
	}
}

func (r *FeishuReporter) OnMessage(ctx context.Context, content string) {
	// 流式模式：content 为空表示流结束，做最终 Patch
	if content == "" && r.streamMsgId != "" {
		r.streamMu.Lock()
		defer r.streamMu.Unlock()
		// 最终 Patch：去掉光标，发送完整内容
		r.patchStreamMessage(r.streamContent.String())
		r.streamMsgId = ""
		r.streamContent.Reset()
		r.streamDirty = false
		return
	}
	// 非流式模式：直接发新消息
	r.sendMessage(content)
}

func (r *FeishuReporter) OnStreamDelta(ctx context.Context, delta string) {
	r.streamMu.Lock()
	defer r.streamMu.Unlock()

	// 第一次收到 delta：先发一条消息，拿到 messageId
	if r.streamMsgId == "" {
		r.streamContent.WriteString(delta)
		msgId := r.createStreamMessage(r.streamContent.String() + " ▌")
		r.streamMsgId = msgId
		r.streamLastPatch = time.Now()
		r.streamDirty = false
		return
	}

	// 后续 delta：累积到 buffer
	r.streamContent.WriteString(delta)
	r.streamDirty = true

	// 限流：距上次 Patch 超过 500ms 才刷新
	if time.Since(r.streamLastPatch) >= 500*time.Millisecond {
		r.patchStreamMessage(r.streamContent.String() + " ▌")
		r.streamLastPatch = time.Now()
		r.streamDirty = false
	}
}

var _ engine.Reporter = (*FeishuReporter)(nil)

func buildApprovalResultCard(taskID string, approved bool) string {
	var title, template string
	if approved {
		title = "审批已通过"
		template = "green"
	} else {
		title = "审批未通过"
		template = "red"
	}
	card := map[string]interface{}{
		"config": map[string]interface{}{
			"wide_screen_mode": true,
		},
		"header": map[string]interface{}{
			"title": map[string]interface{}{
				"tag":     "plain_text",
				"content": title,
			},
			"template": template,
		},
		"elements": []interface{}{
			map[string]interface{}{
				"tag": "div",
				"text": map[string]interface{}{
					"tag":     "lark_md",
					"content": fmt.Sprintf("**任务ID:** %s\n\n审批操作已完成。", taskID),
				},
			},
		},
	}
	cardBytes, _ := json.Marshal(card)
	return string(cardBytes)
}
