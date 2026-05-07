package engine

import "testing"

func TestTranslateNodeType(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"n8n-nodes-base.manualTrigger", "flow-nodes-base.trigger"},
		{"n8n-nodes-base.webhook", "flow-nodes-base.trigger"},
		{"n8n-nodes-base.scheduleTrigger", "flow-nodes-base.trigger"},
		{"n8n-nodes-base.cron", "flow-nodes-base.trigger"},
		{"n8n-nodes-base.set", "flow-nodes-base.set"},
		{"n8n-nodes-base.function", "flow-nodes-base.code"},
		{"n8n-nodes-base.functionItem", "flow-nodes-base.code"},
		{"n8n-nodes-base.noop", "flow-nodes-base.noOp"},
		{"n8n-nodes-base.if", "flow-nodes-base.if"},
		{"@n8n/n8n-nodes-base.set", "flow-nodes-base.set"},
		{"flow-nodes-base.set", "flow-nodes-base.set"},
		{"unknown.type", "unknown.type"},
	}
	for _, c := range cases {
		got := TranslateNodeType(c.in)
		if got != c.want {
			t.Errorf("TranslateNodeType(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestParseWorkflow_RequiresExactlyOneTrigger(t *testing.T) {
	noTrigger := []byte(`{"name":"x","nodes":[{"id":"1","name":"a","type":"n8n-nodes-base.set","parameters":{}}],"connections":{}}`)
	if _, err := ParseWorkflow(noTrigger); err == nil {
		t.Fatal("expected error when no trigger node, got nil")
	}

	twoTriggers := []byte(`{"name":"x","nodes":[
		{"id":"1","name":"a","type":"n8n-nodes-base.manualTrigger","parameters":{}},
		{"id":"2","name":"b","type":"n8n-nodes-base.webhook","parameters":{}}
	],"connections":{}}`)
	if _, err := ParseWorkflow(twoTriggers); err == nil {
		t.Fatal("expected error when two trigger nodes, got nil")
	}

	one := []byte(`{"name":"x","nodes":[{"id":"1","name":"a","type":"n8n-nodes-base.manualTrigger","parameters":{}}],"connections":{}}`)
	wf, err := ParseWorkflow(one)
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if wf.Nodes[0].Type != "flow-nodes-base.trigger" {
		t.Errorf("trigger not translated: got %q", wf.Nodes[0].Type)
	}
}

func TestParseWorkflow_BuildsConnections(t *testing.T) {
	data := []byte(`{
		"name":"x",
		"nodes":[
			{"id":"1","name":"a","type":"n8n-nodes-base.manualTrigger","parameters":{}},
			{"id":"2","name":"b","type":"n8n-nodes-base.set","parameters":{}}
		],
		"connections":{
			"a":{"main":[[{"node":"b","type":"main","index":0}]]}
		}
	}`)
	wf, err := ParseWorkflow(data)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if len(wf.Connections) != 1 {
		t.Fatalf("expected 1 connection, got %d", len(wf.Connections))
	}
	c := wf.Connections[0]
	if c.SourceNode != "a" || c.TargetNode != "b" || c.SourceOutputIndex != 0 || c.TargetInputIndex != 0 {
		t.Errorf("unexpected connection: %+v", c)
	}
}

func TestParseWorkflow_SkipsConnectionsToMissingNodes(t *testing.T) {
	data := []byte(`{
		"name":"x",
		"nodes":[
			{"id":"1","name":"a","type":"n8n-nodes-base.manualTrigger","parameters":{}}
		],
		"connections":{
			"a":{"main":[[{"node":"missing","type":"main","index":0}]]}
		}
	}`)
	wf, err := ParseWorkflow(data)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if len(wf.Connections) != 0 {
		t.Fatalf("expected dangling connections to be dropped, got %d", len(wf.Connections))
	}
}
