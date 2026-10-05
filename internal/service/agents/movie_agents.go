//go:build ignore

package agents

import (
	"context"

	"movieagent/internal/service/pipeline"
)

// MovieAgent is a concrete agent that schedules movie-related pipelines.
type MovieAgent struct {
	*BaseAgent
}

func NewMovieAgent(id string) *MovieAgent {
	return &MovieAgent{BaseAgent: NewBaseAgent(id)}
}

// Schedule runs a named pipeline. Empty name falls back to the default pipeline.
func (a *MovieAgent) Schedule(ctx context.Context, pipelineName string, input map[string]any) (*pipeline.Result, error) {
	if pipelineName == "" {
		return a.Run(ctx, input)
	}
	return a.RunPipeline(ctx, pipelineName, input)
}

// WithDefaultMoviePipelines registers a sample recommend + analyze flow.
func (a *MovieAgent) WithDefaultMoviePipelines() *MovieAgent {
	_ = a.RegisterPipeline(pipeline.New(
		"recommend",
		pipeline.StepFunc{
			StepName: "collect_preferences",
			Fn: func(ctx *pipeline.Context) error {
				ctx.Set("preferences", ctx.Input["preferences"])
				return nil
			},
		},
		pipeline.StepFunc{
			StepName: "rank_candidates",
			Fn: func(ctx *pipeline.Context) error {
				ctx.Set("candidates", []string{"movie-a", "movie-b", "movie-c"})
				return nil
			},
		},
	))

	_ = a.RegisterPipeline(pipeline.New(
		"analyze",
		pipeline.StepFunc{
			StepName: "load_movie",
			Fn: func(ctx *pipeline.Context) error {
				ctx.Set("movie_id", ctx.Input["movie_id"])
				return nil
			},
		},
		pipeline.StepFunc{
			StepName: "generate_summary",
			Fn: func(ctx *pipeline.Context) error {
				ctx.Set("summary", "movie analysis placeholder")
				return nil
			},
		},
	))

	_ = a.SetDefaultPipeline("recommend")
	return a
}

type ScriptBreakAgentReq struct {
	args map[string]any
	
}

func (a *MovieAgent) ScriptBreakAgent() {

}