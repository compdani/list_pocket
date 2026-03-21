/* eslint-disable */
import { ref, shallowRef, watch } from "vue";

function toFlowNodes(workflow) {
  return (workflow?.nodes ?? []).map((node) => ({
    id: node.id,
    type: "workflow",
    position: { x: node.positionX, y: node.positionY },
    data: {
      label: node.label,
      type: node.type,
      description: node.description,
      config: node.config,
      schema: node.schema,
      contactMode: node.contactMode,
    },
  }));
}

function toFlowEdges(workflow) {
  return (workflow?.edges ?? []).map((edge) => ({
    id: edge.id,
    source: edge.sourceNodeId,
    target: edge.targetNodeId,
    sourceHandle: edge.sourceHandle,
    targetHandle: edge.targetHandle,
    label: edge.condition?.branch ?? edge.sourceHandle,
    data: {
      condition: edge.condition ?? {},
    },
  }));
}

function generateGraphId() {
  return Math.random().toString(36).slice(2, 17).padEnd(15, "0").slice(0, 15);
}

function schema(key, label, kind, options) {
  return { key, label, kind, options };
}

function triggerPresentation(mode) {
  switch (mode) {
    case "manual":
      return {
        label: "Manual Trigger",
        description: "Starts the workflow from an operator action or builder test run.",
        contactMode: "manual",
      };
    case "tag_added":
      return {
        label: "Tag Added Trigger",
        description: "Starts the workflow when a contact receives a matching tag.",
        contactMode: "contact-event",
      };
    case "tag_removed":
      return {
        label: "Tag Removed Trigger",
        description: "Starts the workflow when a contact loses a matching tag.",
        contactMode: "contact-event",
      };
    case "webhook":
    default:
      return {
        label: "Webhook Trigger",
        description: "Starts the workflow and defines webhook matching, payload scope, and security requirements.",
        contactMode: "raw-intake",
      };
  }
}

function defaultNodeLabel(type, config = {}) {
  switch (type) {
    case "trigger":
      return triggerPresentation(String(config.mode ?? "webhook")).label;
    case "condition":
      return "Condition";
    case "event_start":
      return "Set Event Start";
    case "http_request":
      return "HTTP Request";
    case "pb_update":
      return "Update Record";
    case "send_transactional_email":
      return "Send Transactional Email";
    case "pb_query":
      return "Query Record";
    case "campaign_launch":
      return "Launch Campaign";
    case "wait_until":
      return "Wait Until";
    case "transform":
    default:
      return "Transform";
  }
}

function buildNodeTemplate(type, index) {
  const base = {
    id: generateGraphId(),
    position: { x: 180 + index * 60, y: 200 + index * 20 },
  };

  switch (type) {
    case "trigger":
      return {
        ...base,
        type: "workflow",
        data: {
          label: triggerPresentation("webhook").label,
          type,
          description: triggerPresentation("webhook").description,
          config: {
            mode: "webhook",
            path: "/new-workflow",
            tagEvent: "tag_added",
            tagName: "demo-booked",
            demoContactId: "",
            payloadField: "body",
            samplePayload: JSON.stringify({
              email: "new-lead@example.com",
              ownerId: "sales-01",
              amount: 4200,
              eventStartAt: "2026-03-20T09:00:00Z",
            }, null, 2),
            payloadSchema: JSON.stringify({
              type: "object",
              required: ["email", "ownerId"],
              properties: {
                email: { type: "string", format: "email" },
                ownerId: { type: "string" },
                amount: { type: "number" },
                eventStartAt: { type: "string", format: "date-time" },
              },
            }, null, 2),
            signatureHeader: "x-workflow-signature",
            secretRef: "env.WEBHOOK_SECRET",
            contactStrategy: "deferred",
            contactKey: "email",
            ownerField: "ownerId",
          },
          schema: [
            { ...schema("mode", "Trigger Mode", "select", ["manual", "webhook", "tag_added", "tag_removed"]), description: "Choose whether this workflow starts manually, from a webhook, or from a contact tag change." },
            { ...schema("path", "Webhook Path", "text"), description: "Relative path under /api/hooks. Enter /new-workflow, not /api/hooks/new-workflow.", placeholder: "/new-workflow" },
            { ...schema("tagName", "Contact Tag", "text"), description: "When using tag triggers, start the workflow when this contact tag is added or removed.", placeholder: "demo-booked" },
            { ...schema("payloadField", "Payload Match Path", "text"), description: "Which part of the inbound payload this workflow should treat as the primary record.", placeholder: "body" },
            { ...schema("samplePayload", "Test Webhook Payload", "textarea"), description: "Paste a representative webhook payload here. You can infer the schema from it and reuse it for Run Test.", placeholder: "{\n  \"email\": \"new-lead@example.com\"\n}" },
            { ...schema("payloadSchema", "Payload Schema", "textarea"), description: "Define the JSON schema this webhook is expected to send into the run.", placeholder: "{\n  \"type\": \"object\"\n}" },
            { ...schema("signatureHeader", "Security Header", "text"), description: "Header containing the webhook signature or API token.", placeholder: "x-workflow-signature" },
            { ...schema("secretRef", "Secret Reference", "text"), description: "Controlled secret reference used to verify webhook requests.", placeholder: "env.WEBHOOK_SECRET" },
            { ...schema("contactStrategy", "Contact Strategy", "select", ["deferred", "lookup-or-create", "require-existing"]), description: "Decide whether the webhook should stay raw, look up/create a contact, or require an existing contact." },
            { ...schema("contactKey", "Contact Lookup Field", "text"), description: "Field used only when the workflow should look up or require a contact.", placeholder: "email" },
            { ...schema("ownerField", "User / Owner Field", "text"), description: "Payload field that identifies the assignee or responsible user.", placeholder: "ownerId" },
          ],
          contactMode: triggerPresentation("webhook").contactMode,
        },
      };
    case "condition":
      return {
        ...base,
        type: "workflow",
        data: {
          label: "Condition",
          type,
          description: "Branches workflow execution using data from the previous step or the larger run context.",
          config: { field: "previous.amount", operator: "greater_than", value: "1000" },
          schema: [
            { ...schema("field", "Field Path", "text"), description: "Supports previous.*, run.triggerPayload.*, contact.*, company.*, or events.* references.", placeholder: "previous.amount" },
            { ...schema("operator", "Operator", "select", ["greater_than", "equals", "contains"]), description: "Comparison operator used to branch the run." },
            { ...schema("value", "Value", "text"), description: "Expected comparison value.", placeholder: "1000" },
          ],
          contactMode: "branch",
        },
      };
    case "event_start":
      return {
        ...base,
        type: "workflow",
        data: {
          label: "Set Event Start",
          type,
          description: "Anchors a named event time in the run so later waits can calculate from it.",
          config: {
            eventKey: "campaignStart",
            sourcePath: "run.triggerPayload.startAt",
            fallbackAt: "",
            timezone: "America/Chicago",
          },
          schema: [
            { ...schema("eventKey", "Event Name", "text"), description: "Stable key used by later wait nodes.", placeholder: "campaignStart" },
            { ...schema("sourcePath", "Source Path", "text"), description: "Where to read the event date from the run context.", placeholder: "run.triggerPayload.startAt" },
            { ...schema("fallbackAt", "Fallback Date", "text"), description: "Used if the source path is empty.", placeholder: "2026-03-20T09:00:00Z" },
            { ...schema("timezone", "Timezone", "text"), description: "Timezone used to interpret calculated wait times.", placeholder: "America/Chicago" },
          ],
          contactMode: "event-anchor",
        },
      };
    case "http_request":
      return {
        ...base,
        type: "workflow",
        data: {
          label: "HTTP Request",
          type,
          description: "Calls an external API using any part of the run context or previous node output.",
          config: {
            url: "https://api.example.com",
            method: "POST",
            bodyMode: "source_path",
            sourcePath: "previous",
            bodyMap: {
              email: "contact.email",
              score: "previous.score",
            },
            authMode: "secret_header",
          },
          schema: [
            { ...schema("url", "URL", "text"), description: "Destination endpoint.", placeholder: "https://api.example.com" },
            { ...schema("method", "Method", "select", ["GET", "POST", "PATCH"]), description: "HTTP method." },
            { ...schema("bodyMode", "Request Body Mode", "select", ["source_path", "custom_map"]), description: "Send a resolved context object or build a custom JSON body field-by-field." },
            { ...schema("sourcePath", "Payload Source", "text"), description: "Which part of the workflow context to send when using source-path mode.", placeholder: "previous" },
            { ...schema("bodyMap", "Request Body Fields", "kv_map"), description: "Build a custom JSON body. Values can reference previous.*, run.*, contact.*, company.*, or events.*." },
            { ...schema("authMode", "Security Mode", "select", ["none", "secret_header", "bearer"]), description: "Approved authentication strategy for this request." },
          ],
          contactMode: "enrich",
        },
      };
    case "pb_update":
      return {
        ...base,
        type: "workflow",
        data: {
          label: "Update Record",
          type,
          description: "Writes workflow results back to contact- or company-scoped records in PocketBase.",
          config: {
            collection: "contacts",
            action: "update",
            recordIdPath: "contact.id",
            fieldMap: {
              stage: "qualified",
              leadScore: "previous.score",
            },
          },
          schema: [
            { ...schema("collection", "Collection", "text"), description: "Target collection.", placeholder: "contacts" },
            { ...schema("action", "Action", "select", ["create", "update", "upsert"]), description: "Persistence strategy." },
            { ...schema("recordIdPath", "Record ID Path", "text"), description: "Where to resolve the target record id from the run context.", placeholder: "contact.id" },
            { ...schema("fieldMap", "Field Map", "kv_map"), description: "Map PocketBase fields to workflow values or literals. Values can reference previous.*, run.*, contact.*, or company.*." },
          ],
          contactMode: "writeback",
        },
      };
    case "send_transactional_email":
      return {
        ...base,
        type: "workflow",
        data: {
          label: "Send Transactional Email",
          type,
          description: "Renders a transactional template, records the send, and tracks opens and link clicks.",
          config: {
            templateId: "",
            templateIdPath: "",
            subscriberEmailPath: "contact.email",
            subscriberIdPath: "contact.id",
            firstNamePath: "contact.firstName",
            lastNamePath: "contact.lastName",
            subscriberNamePath: "",
            phonePath: "contact.phone",
            fromEmail: "",
            fromEmailPath: "",
            subject: "",
            subjectPath: "",
            contentType: "html",
            messenger: "email",
            dataPath: "previous",
            dataMap: {},
            contentTemplate: "",
          },
          schema: [
            { ...schema("templateId", "Template Record ID", "text"), description: "PocketBase template record id only. Legacy numeric ids are not supported here.", placeholder: "abc123def456ghi" },
            { ...schema("templateIdPath", "Template ID Path", "text"), description: "Optional runtime path that resolves a template record id.", placeholder: "previous.templateId" },
            { ...schema("subscriberEmailPath", "Recipient Email Path", "text"), description: "Defaults to the current contact email.", placeholder: "contact.email" },
            { ...schema("subscriberIdPath", "Subscriber ID Path", "text"), description: "Optional subscriber record id for linking the send back to a subscriber. Legacy numeric ids are not supported here.", placeholder: "contact.id" },
            { ...schema("firstNamePath", "First Name Path", "text"), description: "Used to populate subscriber template fields.", placeholder: "contact.firstName" },
            { ...schema("lastNamePath", "Last Name Path", "text"), description: "Used to populate subscriber template fields.", placeholder: "contact.lastName" },
            { ...schema("subscriberNamePath", "Full Name Path", "text"), description: "Optional full-name override.", placeholder: "contact.name" },
            { ...schema("phonePath", "Phone Path", "text"), description: "Optional phone field for subscriber context.", placeholder: "contact.phone" },
            { ...schema("fromEmail", "From Email Override", "text"), description: "Optional explicit From address. Leave blank to use app default.", placeholder: "Team <team@example.com>" },
            { ...schema("fromEmailPath", "From Email Path", "text"), description: "Optional runtime path for the From address.", placeholder: "previous.fromEmail" },
            { ...schema("subject", "Subject Override", "text"), description: "Optional subject override. Leave blank to use the template subject.", placeholder: "Your order is confirmed" },
            { ...schema("subjectPath", "Subject Path", "text"), description: "Optional runtime path for the subject override.", placeholder: "previous.subject" },
            { ...schema("contentType", "Content Type", "select", ["html", "markdown", "plain"]), description: "Message content type." },
            { ...schema("messenger", "Messenger", "text"), description: "Messenger backend to use.", placeholder: "email" },
            { ...schema("dataPath", "Template Data Path", "text"), description: "Object from the run context exposed as Tx.Data.", placeholder: "previous" },
            { ...schema("dataMap", "Template Data Overrides", "kv_map"), description: "Optional extra Tx.Data values. Values can reference previous.*, run.*, contact.*, or company.*." },
            { ...schema("contentTemplate", "Content Template", "richtext"), description: "Optional HTML fragment rendered as {{ template \"content\" . }} inside the selected transactional template.", placeholder: "<p>Hi {{ .Subscriber.FirstName }}</p>" },
          ],
          contactMode: "transactional-email",
        },
      };
    case "pb_query":
      return {
        ...base,
        type: "workflow",
        data: {
          label: "Query Record",
          type,
          description: "Loads contact or company data from PocketBase using current run context.",
          config: { collection: "contacts", filter: "email = {{ contact.email }}" },
          schema: [
            { ...schema("collection", "Collection", "text"), description: "Collection to query.", placeholder: "contacts" },
            { ...schema("filter", "Filter", "textarea"), description: "Query expression. Use run/contact/company references.", placeholder: "email = {{ contact.email }}" },
          ],
          contactMode: "lookup",
        },
      };
    case "campaign_launch":
      return {
        ...base,
        type: "workflow",
        data: {
          label: "Launch Campaign",
          type,
          description: "Starts an existing campaign record so workflows can sequence drip sends.",
          config: {
            campaignId: "",
            campaignIdPath: "",
          },
          schema: [
            { ...schema("campaignId", "Campaign Record ID", "text"), description: "PocketBase record id of the campaign to start.", placeholder: "RECORD_ID" },
            { ...schema("campaignIdPath", "Campaign ID Path", "text"), description: "Optional context path that resolves a campaign record id at runtime.", placeholder: "previous.campaignId" },
          ],
          contactMode: "campaign-launch",
        },
      };
    case "wait_until":
      return {
        ...base,
        type: "workflow",
        data: {
          label: "Wait Until",
          type,
          description: "Pauses the run until a date calculated from an event start or another run-level field.",
          config: {
            referencePath: "run.events.campaignStart",
            offsetDays: "-2",
            offsetHours: "0",
            skipIfPast: "yes",
          },
          schema: [
            { ...schema("referencePath", "Reference Path", "text"), description: "Date field or event anchor this wait should calculate from.", placeholder: "run.events.campaignStart" },
            { ...schema("offsetDays", "Offset Days", "text"), description: "Days added or subtracted from the reference date.", placeholder: "-2" },
            { ...schema("offsetHours", "Offset Hours", "text"), description: "Hours added or subtracted from the reference date.", placeholder: "0" },
            { ...schema("skipIfPast", "Skip If Past", "select", ["yes", "no"]), description: "Allow the workflow to continue if the calculated wait time is already in the past." },
          ],
          contactMode: "wait",
        },
      };
    case "transform":
    default:
      return {
        ...base,
        type: "workflow",
        data: {
          label: "Transform",
          type: "transform",
          description: "Runs a controlled transform with access to the full run instance and previous node output.",
          config: {
            script: "return { fullName: ctx.contact.firstName + ' ' + ctx.contact.lastName, score: ctx.previous.amount > 10000 ? 'high' : 'normal' };",
          },
          schema: [
            {
              ...schema("script", "Script", "textarea"),
              description: "Use ctx.run for the larger workflow instance and ctx.previous for the prior node output.",
              placeholder: "return { ok: true };",
            },
          ],
          contactMode: "enrich",
        },
      };
  }
}

function fromFlowNodes(nodes) {
  return nodes.map((node) => {
    const data = node.data ?? {
      label: "",
      type: "transform",
      description: "",
      config: {},
      schema: [],
      contactMode: "",
    };

    return {
      id: node.id,
      type: data.type,
      label: data.label,
      description: data.description,
      config: data.config,
      schema: data.schema,
      positionX: node.position.x,
      positionY: node.position.y,
      contactMode: data.contactMode,
    };
  });
}

function fromFlowEdges(edges) {
  return edges.map((edge) => ({
    id: edge.id,
    sourceNodeId: edge.source,
    targetNodeId: edge.target,
    sourceHandle: typeof edge.label === "string" ? edge.label : edge.sourceHandle ?? "",
    targetHandle: edge.targetHandle ?? "",
    condition: {
      ...((edge.data?.condition ?? {})),
      branch: typeof edge.label === "string" ? edge.label : "",
    },
  }));
}

export function useBuilderState(workflow) {
  const nodes = shallowRef(toFlowNodes(workflow?.value));
  const edges = shallowRef(toFlowEdges(workflow?.value));
  const initializedWorkflowId = ref("");
  const selectedEdgeId = ref("");
  const selectedNodeId = ref("");
  const emptyData = {
    label: "",
    type: "transform",
    description: "",
    config: {},
    schema: [],
    contactMode: "",
  };

  watch(
    () => workflow?.value,
    (nextWorkflow) => {
      if (!nextWorkflow) {
        nodes.value = [];
        edges.value = [];
        initializedWorkflowId.value = "";
        selectedEdgeId.value = "";
        selectedNodeId.value = "";
        return;
      }

      if (!initializedWorkflowId.value || initializedWorkflowId.value !== nextWorkflow.workflow.id) {
        initializedWorkflowId.value = nextWorkflow.workflow.id;
        nodes.value = toFlowNodes(nextWorkflow);
        edges.value = toFlowEdges(nextWorkflow);
        selectedEdgeId.value = "";
        selectedNodeId.value = "";
      }
    },
    { immediate: true }
  );

  function selectNode(nodeId) {
    selectedEdgeId.value = "";
    selectedNodeId.value = nodeId;
  }

  function selectEdge(edgeId) {
    selectedNodeId.value = "";
    selectedEdgeId.value = edgeId;
  }

  function clearSelection() {
    selectedNodeId.value = "";
    selectedEdgeId.value = "";
  }

  function updateNodeConfig(nodeId, key, value) {
    const nextNodes = [...nodes.value];
    const index = nextNodes.findIndex((node) => node.id === nodeId);
    if (index === -1) {
      return;
    }

    const currentNode = nextNodes[index];
    const currentData = currentNode.data ?? emptyData;
    const nextData = {
      ...currentData,
      config: {
        ...currentData.config,
        [key]: value,
      },
    };

    if (currentData.type === "trigger" && key === "mode") {
      const presentation = triggerPresentation(String(value));
      if (currentData.label === defaultNodeLabel(currentData.type, currentData.config)) {
        nextData.label = presentation.label;
      }
      nextData.description = presentation.description;
      nextData.contactMode = presentation.contactMode;
    }

    nextNodes[index] = {
      ...currentNode,
      data: nextData,
    };

    nodes.value = nextNodes;
  }

  function updateNodeLabel(nodeId, value) {
    const nextNodes = [...nodes.value];
    const index = nextNodes.findIndex((node) => node.id === nodeId);
    if (index === -1) {
      return;
    }

    const currentNode = nextNodes[index];
    const currentData = currentNode.data ?? emptyData;
    nextNodes[index] = {
      ...currentNode,
      data: {
        ...currentData,
        label: value,
      },
    };

    nodes.value = nextNodes;
  }

  function addNode(type) {
    const nextNode = buildNodeTemplate(type, nodes.value.length);
    nodes.value = [...nodes.value, nextNode];
    selectedNodeId.value = nextNode.id;
    return nextNode.id;
  }

  function connectNodes(connection) {
    if (!connection.source || !connection.target) {
      return;
    }

    edges.value = [
      ...edges.value,
      {
        id: generateGraphId(),
        source: connection.source,
        target: connection.target,
        sourceHandle: connection.sourceHandle ?? undefined,
        targetHandle: connection.targetHandle ?? undefined,
        label: connection.sourceHandle ?? "next",
        data: {
          condition: {
            branch: connection.sourceHandle ?? "next",
            expression: "",
          },
        },
      },
    ];
  }

  function updateNodePosition(nodeId, x, y) {
    nodes.value = nodes.value.map((node) => (node.id === nodeId ? { ...node, position: { x, y } } : node));
  }

  function removeSelectedNode() {
    if (!selectedNodeId.value) {
      return;
    }

    const nodeId = selectedNodeId.value;
    nodes.value = nodes.value.filter((node) => node.id !== nodeId);
    edges.value = edges.value.filter((edge) => edge.source !== nodeId && edge.target !== nodeId);
    selectedNodeId.value = nodes.value[0]?.id ?? "";
  }

  function removeSelectedEdge() {
    if (!selectedEdgeId.value) {
      return;
    }

    edges.value = edges.value.filter((edge) => edge.id !== selectedEdgeId.value);
    selectedEdgeId.value = "";
  }

  function updateSelectedEdgeField(field, value) {
    if (!selectedEdgeId.value) {
      return;
    }

    edges.value = edges.value.map((edge) => {
      if (edge.id !== selectedEdgeId.value) {
        return edge;
      }

      const currentCondition = edge.data?.condition ?? {};
      const nextCondition = {
        ...currentCondition,
        [field]: value,
      };

      return {
        ...edge,
        label: field === "branch" ? value : edge.label,
        sourceHandle: field === "branch" ? value : edge.sourceHandle,
        data: {
          ...(edge.data ?? {}),
          condition: nextCondition,
        },
      };
    });
  }

  function graphSignature() {
    return JSON.stringify({
      nodes: fromFlowNodes(nodes.value),
      edges: fromFlowEdges(edges.value),
    });
  }

  return {
    addNode,
    clearSelection,
    connectNodes,
    edges,
    exportWorkflowGraph() {
      return {
        nodes: fromFlowNodes(nodes.value),
        edges: fromFlowEdges(edges.value),
      };
    },
    graphSignature,
    nodes,
    removeSelectedEdge,
    removeSelectedNode,
    selectEdge,
    selectNode,
    selectedEdgeId,
    selectedNodeId,
    updateNodeConfig,
    updateNodeLabel,
    updateNodePosition,
    updateSelectedEdgeField,
  };
}
