package readingflow

import (
	"context"
	"io"
	"strings"

	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
)

// RunTask compiles one Eino chain, ChatTemplate then ChatModel, and invokes it.
// Vendor token usage on the response is reported through WithUsageHook.
func RunTask(ctx context.Context, spec ModelSpec, task string, vars map[string]any) (text string, err error) {
	var usage Usage
	defer func() { notifyUsage(ctx, spec, task, usage, err) }()
	if vars == nil {
		vars = map[string]any{}
	}
	for _, key := range []string{"question", "cards", "persona"} {
		if _, ok := vars[key]; !ok {
			vars[key] = ""
		}
	}
	chatModel, err := newChatModel(ctx, spec)
	if err != nil {
		return "", err
	}
	chain := compose.NewChain[map[string]any, *schema.Message]()
	chain.AppendChatTemplate(templateFor(task))
	chain.AppendChatModel(chatModel)
	runnable, err := chain.Compile(ctx)
	if err != nil {
		return "", err
	}
	out, err := runnable.Invoke(ctx, vars)
	if err != nil {
		return "", err
	}
	usage = usageFrom(out)
	if out == nil || out.Content == "" {
		return "", errEmpty
	}
	return out.Content, nil
}

// RunTaskStream 流式跑同一条链。每收到一个增量片段调用 onChunk（供 SSE 下发），
// 并累计拼成完整文本返回，供调用方落库与结算。onChunk 返回错误（如客户端断开）会提前结束。
// 供应商的 usage 常在最后一个空片段上，文本为空时仍要收下。
func RunTaskStream(ctx context.Context, spec ModelSpec, task string, vars map[string]any, onChunk func(string) error) (text string, err error) {
	var usage Usage
	defer func() { notifyUsage(ctx, spec, task, usage, err) }()
	if vars == nil {
		vars = map[string]any{}
	}
	for _, key := range []string{"question", "cards", "persona"} {
		if _, ok := vars[key]; !ok {
			vars[key] = ""
		}
	}
	chatModel, err := newChatModel(ctx, spec)
	if err != nil {
		return "", err
	}
	chain := compose.NewChain[map[string]any, *schema.Message]()
	chain.AppendChatTemplate(templateFor(task))
	chain.AppendChatModel(chatModel)
	runnable, err := chain.Compile(ctx)
	if err != nil {
		return "", err
	}
	sr, err := runnable.Stream(ctx, vars)
	if err != nil {
		return "", err
	}
	defer sr.Close()
	var b strings.Builder
	for {
		var msg *schema.Message
		msg, err = sr.Recv()
		if err == io.EOF {
			err = nil
			break
		}
		if err != nil {
			return b.String(), err
		}
		usage = usage.merge(usageFrom(msg))
		if msg == nil || msg.Content == "" {
			continue
		}
		b.WriteString(msg.Content)
		if onChunk != nil {
			if err = onChunk(msg.Content); err != nil {
				return b.String(), err
			}
		}
	}
	if b.Len() == 0 {
		return "", errEmpty
	}
	return b.String(), nil
}
