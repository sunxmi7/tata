package readingflow

import (
	"context"

	"github.com/cloudwego/eino/compose"
)

// Input is what the app passes into the reading workflow.
type Input struct {
	Question string
	Cards    string
	Persona  string
}

// Output is one model result per step. Field names are flat because Eino maps fields, not nested structs.
type Output struct {
	DetectText       string
	DetectVendor     string
	DetectModel      string
	DetectError      string
	RewriteText      string
	RewriteVendor    string
	RewriteModel     string
	RewriteError     string
	RecommendText    string
	RecommendVendor  string
	RecommendModel   string
	RecommendError   string
	InterpretText    string
	InterpretVendor  string
	InterpretModel   string
	InterpretError   string
	TarotistText     string
	TarotistVendor   string
	TarotistModel    string
	TarotistError    string
}

type nodeOut struct {
	Text   string
	Vendor string
	Model  string
	Error  string
}

// RunWorkflow compiles an Eino Workflow. The four reading steps fan out from START.
// Tarotist runs in the same graph and returns empty when Persona is blank.
func RunWorkflow(ctx context.Context, specs map[string]ModelSpec, in Input) (Output, error) {
	wf := compose.NewWorkflow[Input, Output]()
	addTask(wf, "detect", specs)
	addTask(wf, "rewrite", specs)
	addTask(wf, "recommend", specs)
	addTask(wf, "interpret", specs)
	addTask(wf, "tarotist", specs)
	wf.End().
		AddInput("detect", compose.MapFields("Text", "DetectText"), compose.MapFields("Vendor", "DetectVendor"), compose.MapFields("Model", "DetectModel"), compose.MapFields("Error", "DetectError")).
		AddInput("rewrite", compose.MapFields("Text", "RewriteText"), compose.MapFields("Vendor", "RewriteVendor"), compose.MapFields("Model", "RewriteModel"), compose.MapFields("Error", "RewriteError")).
		AddInput("recommend", compose.MapFields("Text", "RecommendText"), compose.MapFields("Vendor", "RecommendVendor"), compose.MapFields("Model", "RecommendModel"), compose.MapFields("Error", "RecommendError")).
		AddInput("interpret", compose.MapFields("Text", "InterpretText"), compose.MapFields("Vendor", "InterpretVendor"), compose.MapFields("Model", "InterpretModel"), compose.MapFields("Error", "InterpretError")).
		AddInput("tarotist", compose.MapFields("Text", "TarotistText"), compose.MapFields("Vendor", "TarotistVendor"), compose.MapFields("Model", "TarotistModel"), compose.MapFields("Error", "TarotistError"))
	runnable, err := wf.Compile(ctx)
	if err != nil {
		return Output{}, err
	}
	return runnable.Invoke(ctx, in)
}

func addTask(wf *compose.Workflow[Input, Output], task string, specs map[string]ModelSpec) {
	wf.AddLambdaNode(task, compose.InvokableLambda(func(ctx context.Context, in Input) (nodeOut, error) {
		spec, ok := specs[task]
		if !ok {
			return nodeOut{Error: "未绑定供应商"}, nil
		}
		if task == "tarotist" && in.Persona == "" {
			return nodeOut{}, nil
		}
		text, err := RunTask(ctx, spec, task, map[string]any{
			"question": in.Question,
			"cards":    in.Cards,
			"persona":  in.Persona,
		})
		if err != nil {
			return nodeOut{Vendor: spec.Vendor, Model: spec.Model, Error: err.Error()}, nil
		}
		return nodeOut{Text: text, Vendor: spec.Vendor, Model: spec.Model}, nil
	})).AddInput(compose.START)
}
