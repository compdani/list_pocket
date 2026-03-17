package executor

import (
	"errors"
	"fmt"

	"github.com/compdani/list_pocket/internal/workflow/domain"
)

type CompiledWorkflow struct {
	Workflow domain.Workflow       `json:"workflow"`
	Nodes    []domain.WorkflowNode `json:"nodes"`
	Edges    []domain.WorkflowEdge `json:"edges"`
	Order    []string              `json:"order"`
}

func Compile(workflow domain.Workflow, nodes []domain.WorkflowNode, edges []domain.WorkflowEdge) (*CompiledWorkflow, error) {
	if workflow.TriggerType == "" {
		return nil, errors.New("workflow triggerType is required")
	}
	if len(nodes) == 0 {
		return nil, errors.New("workflow requires at least one node")
	}

	seenTrigger := 0
	for _, node := range nodes {
		if node.Type == "trigger" {
			seenTrigger++
		}
	}
	if seenTrigger != 1 {
		return nil, fmt.Errorf("workflow requires exactly one trigger node, got %d", seenTrigger)
	}

	order, err := walkOrder(nodes, edges)
	if err != nil {
		return nil, err
	}

	return &CompiledWorkflow{
		Workflow: workflow,
		Nodes:    nodes,
		Edges:    edges,
		Order:    order,
	}, nil
}

func (c *CompiledWorkflow) TriggerNodeID() string {
	for _, node := range c.Nodes {
		if node.Type == "trigger" {
			return node.ID
		}
	}
	return ""
}

func walkOrder(nodes []domain.WorkflowNode, edges []domain.WorkflowEdge) ([]string, error) {
	triggerID := ""
	for _, node := range nodes {
		if node.Type == "trigger" {
			triggerID = node.ID
			break
		}
	}
	if triggerID == "" {
		return nil, errors.New("workflow requires a trigger node")
	}

	order := []string{}
	seen := map[string]bool{}
	queue := []string{triggerID}
	for len(queue) > 0 {
		nodeID := queue[0]
		queue = queue[1:]
		if seen[nodeID] {
			continue
		}
		seen[nodeID] = true
		order = append(order, nodeID)

		for _, edge := range edges {
			if edge.SourceNodeID == nodeID {
				queue = append(queue, edge.TargetNodeID)
			}
		}
	}

	return order, nil
}
