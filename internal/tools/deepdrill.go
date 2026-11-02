package tools

import (
	"context"
	"strings"
	"time"
)

type DeepDrillPlan struct {
	SessionID          string                `json:"session_id"`
	Collection         string                `json:"collection"`
	BranchStatus       DeepDrillBranchStatus `json:"branch_status"`
	TopUncertainty     DeepDrillUncertainty  `json:"top_uncertainty"`
	CandidateQuestions []string              `json:"candidate_questions"`
	SelectedQuestion   string                `json:"selected_question"`
	SelectedStrategy   DeepDrillStrategy     `json:"selected_strategy"`
	StrategyReason     string                `json:"strategy_reason"`
	ReuseStrategy      bool                  `json:"reuse_strategy"`
	SystemPrompt       string                `json:"system_prompt"`
	State              DeepDrillState        `json:"state"`
}

func (rt *MindDrillResearchRuntime) ConfigureDeepDrill(thoughtCollection string, freezeAfterLowGain int) {
	if rt == nil {
		return
	}
	if strings.TrimSpace(thoughtCollection) != "" {
		rt.deepDrillThoughts = strings.TrimSpace(thoughtCollection)
	}
	if freezeAfterLowGain > 0 {
		rt.deepDrillFreezeAfterLow = freezeAfterLowGain
	}
}

func (rt *MindDrillResearchRuntime) DeepDrillPlan(ctx context.Context, req DeepDrillPlanRequest) (DeepDrillPlan, error) {
	session, collection, state, err := rt.prepareDeepDrillSession(ctx, req)
	if err != nil {
		return DeepDrillPlan{}, err
	}
	uncertainty := identifyHighestValueUncertainty(session, state)
	questions := generateCompetingQuestions(session, state, uncertainty)
	selectedQuestion := selectMostDiscriminatingQuestion(questions, state)
	strategy, reason, reused := selectStrategy(state, uncertainty)
	state.CurrentUncertainty = uncertainty
	state.CurrentQuestion = selectedQuestion
	state.LastUpdated = time.Now().UTC()
	session.DeepDrill = state
	rt.sessions.put(session)
	return DeepDrillPlan{
		SessionID:          session.SessionID,
		Collection:         collection,
		BranchStatus:       state.BranchStatus,
		TopUncertainty:     uncertainty,
		CandidateQuestions: questions,
		SelectedQuestion:   selectedQuestion,
		SelectedStrategy:   strategy,
		StrategyReason:     reason,
		ReuseStrategy:      reused,
		SystemPrompt:       DeepDrillResearcherSystemPrompt,
		State:              *state,
	}, nil
}

func (rt *MindDrillResearchRuntime) DeepDrillRun(ctx context.Context, req DeepDrillRunRequest) (DeepDrillRunResult, error) {
	if req.Steps <= 0 {
		req.Steps = 1
	}
	if req.Steps > 50 {
		req.Steps = 50
	}
	result := DeepDrillRunResult{SessionID: strings.TrimSpace(req.DeepDrillPlanRequest.SessionID), Steps: []DeepDrillStepResult{}}
	maxIterations := req.Steps
	if req.UntilStable {
		maxIterations = 25
	}
	for i := 0; i < maxIterations; i++ {
		plan, err := rt.DeepDrillPlan(ctx, req.DeepDrillPlanRequest)
		if err != nil {
			return result, err
		}
		step, state, err := rt.executeDeepDrillStep(ctx, plan)
		if err != nil {
			return result, err
		}
		result.State = *state
		if step.Terminal {
			break
		}
		result.Steps = append(result.Steps, step)
		if state.BranchStatus == DeepDrillFrozen {
			break
		}
	}
	return result, nil
}

func newDefaultDeepDrillState(thoughtCollection string) *DeepDrillState {
	return &DeepDrillState{
		ThoughtCollection: strings.TrimSpace(thoughtCollection),
		BranchStatus:      DeepDrillOpen,
		KnownBlockers:     []DeepDrillUncertainty{},
		Hypotheses:        []DeepDrillHypothesisState{},
		LastUpdated:       time.Now().UTC(),
	}
}
