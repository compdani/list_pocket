import test from 'node:test';
import assert from 'node:assert/strict';
import { ref } from 'vue';
import { arrangeWorkflowGraph, canConnectWorkflowNodes } from '../src/workflow/graph.mjs';
import { useBuilderState } from '../src/composables/useBuilderState.js';
const nodes = [{ id: 'start', type: 'trigger', label: 'Start', config: { mode: 'manual' }, positionX: 0, positionY: 0 }, { id: 'wait', type: 'wait_until', label: 'Wait', config: { duration: 5 }, positionX: 0, positionY: 0 }, { id: 'email', type: 'send_transactional_email', label: 'Email', config: { subject: 'Hello' }, positionX: 0, positionY: 0 }];
const edges = [{ id: 'one', sourceNodeId: 'start', targetNodeId: 'wait' }, { id: 'two', sourceNodeId: 'wait', targetNodeId: 'email' }];
test('auto-arrange orders steps without changing configuration, IDs, or connections', () => {
  const graph = { nodes, edges }; const result = arrangeWorkflowGraph(graph);
  assert.ok(result.nodes[0].positionY < result.nodes[1].positionY);
  assert.ok(result.nodes[1].positionY < result.nodes[2].positionY);
  assert.deepEqual(result.nodes.map(node => node.config), nodes.map(node => node.config));
  assert.deepEqual(result.edges, edges); assert.equal(nodes[0].positionY, 0);
  assert.deepEqual(arrangeWorkflowGraph(result), result);
});
test('auto-arrange handles branches, cycles and missing endpoints', () => {
  const result = arrangeWorkflowGraph({ nodes, edges: [...edges, { id: 'cycle', sourceNodeId: 'email', targetNodeId: 'wait' }, { sourceNodeId: 'missing', targetNodeId: 'wait' }] });
  assert.ok(result.nodes.every(node => Number.isFinite(node.positionX) && Number.isFinite(node.positionY)));
  assert.equal(new Set(result.nodes.map(node => `${node.positionX}:${node.positionY}`)).size, 3);
});
test('connections reject self loops, duplicate edges and incoming trigger connections', () => {
  const flowNodes = nodes.map(node => ({ id: node.id, data: { type: node.type } }));
  const flowEdges = [{ id: 'existing', source: 'start', target: 'wait' }];
  assert.equal(canConnectWorkflowNodes(flowNodes, flowEdges, 'start', 'wait'), false);
  assert.equal(canConnectWorkflowNodes(flowNodes, flowEdges, 'start', 'wait', undefined, 'existing'), true);
  assert.equal(canConnectWorkflowNodes(flowNodes, flowEdges, 'wait', 'wait'), false);
  assert.equal(canConnectWorkflowNodes(flowNodes, flowEdges, 'email', 'start'), false);
  assert.equal(canConnectWorkflowNodes(flowNodes, flowEdges, 'wait', 'email'), true);
});
test('duplicate steps keep configuration, get new IDs, and never duplicate the trigger', () => {
  const state = useBuilderState(ref({ workflow: { id: 'fixture' }, nodes, edges }));
  state.selectNode('start'); assert.equal(state.duplicateSelectedNode(), null);
  assert.equal(state.addNode('trigger'), null);
  state.selectNode('wait'); const duplicateID = state.duplicateSelectedNode();
  assert.notEqual(duplicateID, 'wait');
  const duplicate = state.exportWorkflowGraph().nodes.find(node => node.id === duplicateID);
  assert.deepEqual(duplicate.config, nodes[1].config);
  assert.equal(duplicate.label, 'Wait copy'); assert.equal(state.exportWorkflowGraph().edges.length, 2);
});

test('connections survive save, arrangement and label changes without inventing handle IDs', () => {
  const state = useBuilderState(ref({ workflow: { id: 'roundtrip' }, nodes, edges: [] }));
  state.connectNodes({ source: 'start', target: 'wait' });
  const saved = state.exportWorkflowGraph();
  assert.equal(saved.edges[0].sourceNodeId, 'start'); assert.equal(saved.edges[0].targetNodeId, 'wait');
  assert.equal(saved.edges[0].sourceHandle, '');
  state.replaceGraph(arrangeWorkflowGraph(saved));
  assert.equal(state.edges.value[0].source, 'start'); assert.equal(state.edges.value[0].target, 'wait');
  assert.equal(state.edges.value[0].sourceHandle, undefined);
  state.selectEdge(state.edges.value[0].id); state.updateSelectedEdgeField('branch', 'Custom label');
  assert.equal(state.edges.value[0].sourceHandle, undefined);
});
