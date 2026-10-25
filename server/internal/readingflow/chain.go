package readingflow

import (
	"context"

	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
)

// RunTask compiles one Eino chain, ChatTemplate then ChatModel, and invokes it.
func RunTask(ctx context.Context, spec ModelSpec, task string, vars map[string]any) (string, error) {
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
	if out == nil || out.Content == "" {
		return "", errEmpty
	}
	return out.Content, nil
}
