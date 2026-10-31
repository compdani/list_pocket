package flow

import "github.com/compdani/list_pocket/internal/flow/tmpl"

// Expressions and templates live in package tmpl (shared with the template
// library and plugin scripts); these aliases keep the workflow API.
var (
	Eval           = tmpl.Eval
	EvalBool       = tmpl.EvalBool
	Render         = tmpl.Render
	RenderMap      = tmpl.RenderMap
	CheckTemplates = tmpl.CheckTemplates
	CheckExpr      = tmpl.CheckExpr
	stringify      = tmpl.Stringify
)
