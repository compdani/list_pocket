package main

// The chat module's turn API, behind LISTPOCKET_CHAT_BACKEND=module. It shares
// the assistant's tools, prompt and limits with cmd/ai_assistant.go and
// cmd/agent.go, so a turn behaves like the surface it came from while the
// legacy endpoints keep serving the front end.

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"time"

	"github.com/compdani/list_pocket/internal/auth"
	pbmcp "github.com/compdani/pocketbase-mcp"
	pbadapter "github.com/compdani/pocketbase-mcp/adapters/pocketbase"
	"github.com/compdani/pocketbase-mcp/chat"
)

// chatAgentScreen is the page-context screen of the Agent page. Every other
// screen, and a turn with no context, is treated as the side panel.
const chatAgentScreen = "agent"

// chatPageContext is the page-context contract both surfaces send. Only Screen,
// Path and References drive a turn today; the rest is for the front ends that
// move over in later phases (params, filters, the unsaved editor snapshot and
// media-library pictures). Nothing here is trusted: references are read as the
// caller through permission-checked handlers, and limits come from the screen
// alone, never from a number the browser sent.
//
//	{"screen": "panel"|"agent", "path": "...", "page": "...",
//	 "params": {...}, "filters": {...},
//	 "editor": {"key","unsaved","revision","current","selection","label"}|null,
//	 "pictureReferences": [{"name","url"}],
//	 "references": [{"type","id","label"}]}
type chatPageContext struct {
	Screen            string          `json:"screen"`
	Path              string          `json:"path"`
	Page              string          `json:"page"`
	Params            json.RawMessage `json:"params"`
	Filters           json.RawMessage `json:"filters"`
	Editor            json.RawMessage `json:"editor"`
	PictureReferences json.RawMessage `json:"pictureReferences"`
	References        []agentRef      `json:"references"`
}

// chatModuleEnabled reports the opt-in flag. Anything else, including unset,
// leaves chat off and only /mcp mounted.
func chatModuleEnabled() bool {
	return strings.EqualFold(strings.TrimSpace(os.Getenv("LISTPOCKET_CHAT_BACKEND")), "module")
}

// enableChat builds the module's chat surface for List Pocket's existing
// settings and tools. Options mirror the legacy engines: review for every
// server write, no review for the plugin drafts that write today, argument
// edits, uploads, stored references, the side panel's context size and the
// assistant's output ceiling.
func (a *App) enableChat(srv *pbadapter.Server) error {
	store := &chatProviderStore{app: a}
	// EnableChat builds the first catalog snapshot synchronously. Off the
	// network until it returns, then reload for real model lists in the
	// background: startup must never wait on /models.
	store.cacheOnly.Store(true)
	noParallel := false
	err := srv.EnableChat(pbadapter.ChatConfig{
		Options: pbmcp.ChatOptions{
			Namespace:          "listpocket",
			Proposals:          true,
			AutoApprove:        true,
			AllowArgumentEdits: true,
			// Approvals run plugin installs, workflow publishes and sends inside
			// the decision request, so they get longer than the module default.
			DecisionTimeout:   5 * time.Minute,
			Uploads:           true,
			AcceptOtherImages: true,
			TurnContext:       true,
			StoreContext:      true,
			// The side panel sends its whole unsaved editor snapshot each turn,
			// and today's message ceiling is 200,000 characters.
			MaxContextBytes: 2 << 20,
			MaxMessageChars: 200000,
			MaxBodyBytes:    4 << 20,
			Imports:         false,
		},
		ProviderStore: store,
		// First-message titles: no model call, cut to 60 characters.
		TitleModel: "first-message",
		RunnerConfig: chat.RunnerConfig{
			MaxSteps:             assistantMaxRounds,
			TurnTimeout:          time.Duration(defaultAIBuilderTimeoutSeconds) * time.Second,
			MaxConcurrentGlobal:  4,
			MaxConcurrentPerUser: 4,
			MaxHistoryMessages:   agentHistoryMessages,
			ParallelToolCalls:    &noParallel,
			// 0 from LISTPOCKET_AI_MAX_OUTPUT_TOKENS means no limit, and the
			// runner leaves the parameter out exactly as the legacy client does.
			MaxOutputTokens: assistantOutputTokens(),
			Setup:           a.chatTurnSetup,
			Logger:          a.moduleLogger(),
		},
	})
	if err != nil {
		return err
	}
	store.cacheOnly.Store(false)
	go a.reloadChatProviders(context.Background())
	return nil
}

// chatTurnSetup gives a turn the same prompt, tools and limits the matching
// legacy surface builds. It runs before anything is stored, so a refusal here
// leaves no turn behind.
func (a *App) chatTurnSetup(ctx context.Context, in chat.TurnInput) (chat.TurnSetup, error) {
	caller, err := pbadapter.ResolveTurnCaller(ctx, a.pb, in)
	if err != nil {
		return chat.TurnSetup{}, err
	}
	e, err := a.callerRequestEvent(ctx, caller)
	if err != nil {
		return chat.TurnSetup{}, err
	}
	superAdmin := isSuperAdmin(e)

	var page chatPageContext
	if len(in.Context) > 0 {
		// A context the browser sent that isn't an object is treated as absent:
		// the module already checked it is a JSON object, so this can only fail
		// on a shape this release doesn't know.
		_ = json.Unmarshal(in.Context, &page)
	}

	if page.Screen == chatAgentScreen {
		refs, err := a.chatAgentRefs(in, page.References)
		if err != nil {
			return chat.TurnSetup{}, err
		}
		// StartAgentTurn sends exactly this page context, so the base prompt is
		// the same string the legacy agent turn builds.
		agentPage, _ := json.Marshal(map[string]string{"page": "agent", "path": page.Path})
		system := a.assistantSystemPrompt(agentPage, superAdmin)
		if extra := a.agentContextText(e, refs, auth.GetUserRE(e).RecordID); extra != "" {
			system += "\n\n" + extra
		}
		return chat.TurnSetup{
			System:      system,
			MaxSteps:    assistantMaxRounds,
			TurnTimeout: time.Duration(agentTurnTimeoutSeconds) * time.Second,
			// The Agent page has no editor.
			Tools: func(name string) bool { return name != "propose_editor_patch" },
		}, nil
	}

	// Side panel, including a missing screen. launchAssistantTurn mirrors this:
	// the AI builder timeout, raised for a super admin whose assistant can also
	// write plugins.
	_, timeout, _ := a.getAIBuilderSettingsValues()
	if timeout <= 0 {
		timeout = defaultAIBuilderTimeoutSeconds
	}
	if a.plugins != nil && superAdmin && timeout < agentTurnTimeoutSeconds {
		timeout = agentTurnTimeoutSeconds
	}
	return chat.TurnSetup{
		System:      a.assistantSystemPrompt(in.Context, superAdmin),
		MaxSteps:    assistantMaxRounds,
		TurnTimeout: time.Duration(timeout) * time.Second,
	}, nil
}

// chatAgentRefs accumulates the Agent page's references: earlier turns' contexts
// first (oldest first), then this message's, de-duplicated by type:id. Files are
// skipped because they are uploads on this path, not references. An unknown type
// in this message is refused; one in an older context is ignored, because a
// conversation must stay usable after a release drops a type.
func (a *App) chatAgentRefs(in chat.TurnInput, current []agentRef) ([]agentRef, error) {
	var refs []agentRef
	seen := map[string]bool{}
	add := func(ref agentRef) {
		if len(refs) >= agentMaxRefs || ref.Type == "file" {
			return
		}
		if _, ok := agentRefSources[ref.Type]; !ok {
			return
		}
		if key := ref.key(); !seen[key] {
			seen[key] = true
			refs = append(refs, ref)
		}
	}
	for _, raw := range in.EarlierContexts {
		var page chatPageContext
		if json.Unmarshal(raw, &page) != nil {
			continue
		}
		for _, ref := range page.References {
			add(ref)
		}
	}
	if len(current) > agentMaxRefs {
		return nil, chat.RefuseTurn("that's too much context for one message")
	}
	for _, ref := range current {
		if ref.Type == "file" {
			continue
		}
		if _, ok := agentRefSources[ref.Type]; !ok {
			return nil, chat.RefuseTurn("unknown context type")
		}
		add(ref)
	}
	return refs, nil
}
