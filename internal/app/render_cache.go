package app

import "git.example.com/ai/wrd-ai-cli/internal/agent"

type cachedChatMessage struct {
	message  chatMessage
	proposal agent.Proposal
}

type aiConversationCache struct {
	width    int
	language string
	thought  string
	review   bool
	messages []cachedChatMessage
	lines    []aiDisplayLine
}

// SSH echo and editor keystrokes do not change conversation text. Reuse its
// layout, but compare proposal values too because risk review mutates them
// through shared pointers while an AI turn is still running.
func (a *App) cachedAIConversationLines(width int) []aiDisplayLine {
	review := a.proposal != nil && isReviewRisk(a.proposal.Risk)
	cache := a.aiDisplayCache
	valid := cache != nil && cache.width == width && cache.language == a.localizer.Language() &&
		cache.thought == a.agentThought && cache.review == review && len(cache.messages) == len(a.chat)
	if valid {
		for index, message := range a.chat {
			proposal := agent.Proposal{}
			if message.Proposal != nil {
				proposal = *message.Proposal
			}
			if cache.messages[index].message != message || cache.messages[index].proposal != proposal {
				valid = false
				break
			}
		}
	}
	if valid {
		return cache.lines
	}
	cache = &aiConversationCache{
		width: width, language: a.localizer.Language(), thought: a.agentThought, review: review,
		messages: make([]cachedChatMessage, len(a.chat)), lines: a.buildAIConversationLines(width),
	}
	for index, message := range a.chat {
		cache.messages[index].message = message
		if message.Proposal != nil {
			cache.messages[index].proposal = *message.Proposal
		}
	}
	a.aiDisplayCache = cache
	return cache.lines
}

type inlineAnswerCache struct {
	width, available int
	language         string
	response, err    string
	lines            []string
}

func (a *App) cachedInlineAnswerLines(state *inlineAIState, width, available int) []string {
	cache := state.answerCache
	if cache != nil && cache.width == width && cache.available == available && cache.language == a.localizer.Language() &&
		cache.response == state.Response && cache.err == state.Error {
		return cache.lines
	}
	state.answerCache = &inlineAnswerCache{
		width: width, available: available, language: a.localizer.Language(), response: state.Response, err: state.Error,
		lines: a.buildInlineAnswerLines(state, width, available),
	}
	return state.answerCache.lines
}
