package main

import (
	"net/http"
	"strings"

	"github.com/compdani/list_pocket/internal/apperr"
	"github.com/compdani/list_pocket/internal/auth"
	"github.com/compdani/list_pocket/internal/core"
	"github.com/compdani/list_pocket/internal/flow"
	"github.com/compdani/list_pocket/models"
	pbcore "github.com/pocketbase/pocketbase/core"
)

const (
	boardActorWorkflow  = "Workflow"
	boardActorAssistant = "AI assistant"
	maxBoardBulkCards   = 500
)

// moveContactOnBoard moves a contact and raises a stage change, so workflows can react to
// moves however they happened. Reordering within a stage raises nothing.
func (a *App) moveContactOnBoard(in core.BoardCardMove) (core.BoardMove, error) {
	out, err := a.core.MoveBoardCard(in)
	if err != nil {
		return out, err
	}
	a.emitBoardMove(out)
	return out, nil
}

// removeContactFromBoard takes a contact off a board and raises a stage change.
func (a *App) removeContactFromBoard(board, subID, actor string) (core.BoardMove, error) {
	out, err := a.core.RemoveBoardCard(board, subID, actor)
	if err != nil {
		return out, err
	}
	a.emitBoardMove(out)
	return out, nil
}

func (a *App) emitBoardMove(m core.BoardMove) {
	if !m.Changed {
		return
	}
	payload := map[string]any{
		"contact_id": m.Subscriber, "board": m.Board, "board_name": m.BoardName,
		"stage": m.Stage, "stage_name": m.StageName, "previous_stage": m.PreviousStage,
	}
	if ids, err := a.core.ResolveSubscriberIDs(nil, []string{m.Subscriber}); err == nil && len(ids) == 1 {
		if sub, err := a.core.GetSubscriber(ids[0], "", ""); err == nil {
			email := sub.Email
			if models.IsPlaceholderEmail(email) {
				email = ""
			}
			payload["email"], payload["name"] = email, sub.Name
		}
	}
	a.emitFlowEvent(flow.EventContactStageChanged, payload)
}

// requireAllContacts guards routes that show every contact on a board, which don't filter
// by list.
func requireAllContacts(re *pbcore.RequestEvent) error {
	user := auth.GetUserRE(re)
	if !user.HasPerm("subscribers:get_all") {
		return apperr.Forbidden("viewing a board needs access to all subscribers")
	}
	return nil
}

// GetBoards lists the boards.
func (a *App) GetBoards(re *pbcore.RequestEvent) error {
	out, err := a.core.ListBoards()
	if err != nil {
		return apperr.Internal(err.Error())
	}
	return re.JSON(http.StatusOK, okResp{Data: out})
}

// GetBoard returns a board and its cards.
func (a *App) GetBoard(re *pbcore.RequestEvent) error {
	if err := requireAllContacts(re); err != nil {
		return err
	}
	out, err := a.core.GetBoard(pathParam(re, "board"))
	if err != nil {
		return err
	}
	return okJSON(re, out)
}

// CreateBoard adds a board.
func (a *App) CreateBoard(re *pbcore.RequestEvent) error {
	var in struct {
		Name   string   `json:"name"`
		Stages []string `json:"stages"`
	}
	if err := bindJSON(re, &in); err != nil {
		return err
	}
	out, err := a.core.CreateBoard(in.Name, in.Stages)
	if err != nil {
		return err
	}
	return okJSON(re, out)
}

// UpdateBoard renames a board.
func (a *App) UpdateBoard(re *pbcore.RequestEvent) error {
	var in struct {
		Name string `json:"name"`
	}
	if err := bindJSON(re, &in); err != nil {
		return err
	}
	out, err := a.core.RenameBoard(pathParam(re, "board"), in.Name)
	if err != nil {
		return err
	}
	return okJSON(re, out)
}

// DeleteBoard removes a board and its cards.
func (a *App) DeleteBoard(re *pbcore.RequestEvent) error {
	if err := a.core.DeleteBoard(pathParam(re, "board")); err != nil {
		return err
	}
	return okJSON(re, true)
}

// CreateBoardStage adds a stage to a board.
func (a *App) CreateBoardStage(re *pbcore.RequestEvent) error {
	var in struct {
		Name  string `json:"name"`
		Color string `json:"color"`
	}
	if err := bindJSON(re, &in); err != nil {
		return err
	}
	out, err := a.core.AddBoardStage(pathParam(re, "board"), in.Name, in.Color)
	if err != nil {
		return err
	}
	return okJSON(re, out)
}

// UpdateBoardStage renames or recolors a stage.
func (a *App) UpdateBoardStage(re *pbcore.RequestEvent) error {
	var in core.BoardStageInput
	if err := bindJSON(re, &in); err != nil {
		return err
	}
	out, err := a.core.UpdateBoardStage(pathParam(re, "board"), pathParam(re, "stage"), in)
	if err != nil {
		return err
	}
	return okJSON(re, out)
}

// ReorderBoardStages sets the order of a board's stages.
func (a *App) ReorderBoardStages(re *pbcore.RequestEvent) error {
	var in struct {
		Stages []string `json:"stages"`
	}
	if err := bindJSON(re, &in); err != nil {
		return err
	}
	out, err := a.core.ReorderBoardStages(pathParam(re, "board"), in.Stages)
	if err != nil {
		return err
	}
	return okJSON(re, out)
}

// DeleteBoardStage removes a stage, moving its cards to the stage in move_to. This changes
// the board rather than moving a contact, so it raises no stage changes.
func (a *App) DeleteBoardStage(re *pbcore.RequestEvent) error {
	out, err := a.core.DeleteBoardStage(pathParam(re, "board"), pathParam(re, "stage"), queryParam(re, "move_to"), userDisplayName(auth.GetUserRE(re)))
	if err != nil {
		return err
	}
	return okJSON(re, out)
}

// MoveBoardCard puts a contact in a stage, adding them to the board if needed.
func (a *App) MoveBoardCard(re *pbcore.RequestEvent) error {
	subID, err := a.contactFromRoute(re)
	if err != nil {
		return err
	}
	var in struct {
		Stage    string `json:"stage"`
		Position *int   `json:"position"`
	}
	if err := bindJSON(re, &in); err != nil {
		return err
	}
	out, err := a.moveContactOnBoard(core.BoardCardMove{
		Board: pathParam(re, "board"), Subscriber: subID, Stage: in.Stage, Position: in.Position,
		Actor: userDisplayName(auth.GetUserRE(re)),
	})
	if err != nil {
		return err
	}
	return okJSON(re, out)
}

// RemoveBoardCard takes a contact off a board.
func (a *App) RemoveBoardCard(re *pbcore.RequestEvent) error {
	subID, err := a.contactFromRoute(re)
	if err != nil {
		return err
	}
	out, err := a.removeContactFromBoard(pathParam(re, "board"), subID, userDisplayName(auth.GetUserRE(re)))
	if err != nil {
		return err
	}
	return okJSON(re, out)
}

// AddBoardCards puts many contacts in a stage.
func (a *App) AddBoardCards(re *pbcore.RequestEvent) error {
	var in struct {
		Subscribers []string `json:"subscribers"`
		Stage       string   `json:"stage"`
	}
	if err := bindJSON(re, &in); err != nil {
		return err
	}
	if len(in.Subscribers) == 0 {
		return apperr.BadRequest("choose at least one contact")
	}
	if len(in.Subscribers) > maxBoardBulkCards {
		return apperr.BadRequest("too many contacts at once")
	}
	ids, err := a.core.ResolveSubscriberIDs(nil, in.Subscribers)
	if err != nil {
		return apperr.BadRequest(a.i18n.Ts("globals.messages.errorInvalidIDs", "error", err.Error()))
	}
	user := auth.GetUserRE(re)
	if err := a.hasSubPerm(user, ids); err != nil {
		return err
	}
	recordIDs, err := a.core.ResolveSubscriberRecordIDs(ids)
	if err != nil {
		return apperr.BadRequest(a.i18n.Ts("globals.messages.errorInvalidIDs", "error", err.Error()))
	}
	changed := 0
	for _, subID := range recordIDs {
		out, err := a.moveContactOnBoard(core.BoardCardMove{
			Board: pathParam(re, "board"), Subscriber: subID, Stage: strings.TrimSpace(in.Stage), Actor: userDisplayName(user),
		})
		if err != nil {
			return err
		}
		if out.Changed {
			changed++
		}
	}
	return okJSON(re, map[string]any{"changed": changed})
}

// GetContactBoards lists the boards a contact is on.
func (a *App) GetContactBoards(re *pbcore.RequestEvent) error {
	subID, err := a.contactFromRoute(re)
	if err != nil {
		return err
	}
	out, err := a.core.GetContactBoards(subID)
	if err != nil {
		return apperr.Internal(err.Error())
	}
	return re.JSON(http.StatusOK, okResp{Data: out})
}

// assistantBoardView reads a board for the assistant, whose id is the board key.
func (a *App) assistantBoardView(re *pbcore.RequestEvent) error {
	re.Request.SetPathValue("board", pathParam(re, "id"))
	return a.GetBoard(re)
}

// assistantMoveBoardCard applies a proposal to move a contact, whose id is the contact.
func (a *App) assistantMoveBoardCard(re *pbcore.RequestEvent) error {
	subID, err := a.contactFromRoute(re)
	if err != nil {
		return err
	}
	var in struct {
		Board string `json:"board"`
		Stage string `json:"stage"`
	}
	if err := bindJSON(re, &in); err != nil {
		return err
	}
	out, err := a.moveContactOnBoard(core.BoardCardMove{Board: in.Board, Subscriber: subID, Stage: in.Stage, Actor: boardActorAssistant})
	if err != nil {
		return err
	}
	return okJSON(re, out)
}
