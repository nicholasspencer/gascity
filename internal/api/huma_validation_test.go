package api

import (
	"testing"
)

// TestAgentCreateSpecMarksFieldsRequired verifies that the OpenAPI spec
// marks name and provider as required fields (Phase 2 Fix 2: no more
// omitempty bypass hiding required fields).
func TestAgentCreateSpecMarksFieldsRequired(t *testing.T) {
	spec := readCommittedOpenAPISpec(t)

	// Walk to the request body schema for POST /v0/city/{cityName}/agents.
	paths, _ := spec["paths"].(map[string]any)
	agentsPath, _ := paths["/v0/city/{cityName}/agents"].(map[string]any)
	post, _ := agentsPath["post"].(map[string]any)
	reqBody, _ := post["requestBody"].(map[string]any)
	content, _ := reqBody["content"].(map[string]any)
	appJSON, _ := content["application/json"].(map[string]any)
	schema, _ := appJSON["schema"].(map[string]any)

	// Schema is usually a $ref; resolve it.
	if ref, ok := schema["$ref"].(string); ok {
		// "#/components/schemas/FooRequest" → FooRequest
		name := ref[len("#/components/schemas/"):]
		components, _ := spec["components"].(map[string]any)
		schemas, _ := components["schemas"].(map[string]any)
		resolved, ok := schemas[name].(map[string]any)
		if !ok {
			t.Fatalf("could not resolve $ref %s", ref)
		}
		schema = resolved
	}

	required, _ := schema["required"].([]any)
	reqMap := make(map[string]bool)
	for _, r := range required {
		if s, ok := r.(string); ok {
			reqMap[s] = true
		}
	}

	if !reqMap["name"] {
		t.Errorf("agent create schema does not mark name as required; required=%v", required)
	}
	if !reqMap["provider"] {
		t.Errorf("agent create schema does not mark provider as required; required=%v", required)
	}
}

func TestBeadCloseSpecAcceptsOptionalReason(t *testing.T) {
	spec := readCommittedOpenAPISpec(t)

	paths, _ := spec["paths"].(map[string]any)
	beadPath, _ := paths["/v0/city/{cityName}/bead/{id}/close"].(map[string]any)
	post, _ := beadPath["post"].(map[string]any)
	reqBody, _ := post["requestBody"].(map[string]any)
	if reqBody == nil {
		t.Fatal("bead close operation has no requestBody; want optional reason body")
	}
	if reqBody["required"] == true {
		t.Fatal("bead close requestBody is required; want optional body for backward-compatible closes")
	}

	content, _ := reqBody["content"].(map[string]any)
	appJSON, _ := content["application/json"].(map[string]any)
	schema, _ := appJSON["schema"].(map[string]any)
	if schema == nil {
		t.Fatalf("bead close requestBody has no application/json schema: %v", reqBody)
	}
	if ref, ok := schema["$ref"].(string); ok {
		name := ref[len("#/components/schemas/"):]
		components, _ := spec["components"].(map[string]any)
		schemas, _ := components["schemas"].(map[string]any)
		resolved, ok := schemas[name].(map[string]any)
		if !ok {
			t.Fatalf("could not resolve $ref %s", ref)
		}
		schema = resolved
	}

	props, _ := schema["properties"].(map[string]any)
	reason, _ := props["reason"].(map[string]any)
	if reason == nil {
		t.Fatalf("bead close schema properties = %v, want reason", props)
	}
	if reason["type"] != "string" {
		t.Fatalf("reason schema type = %v, want string", reason["type"])
	}
	if reason["maxLength"] == nil {
		t.Fatalf("reason schema = %v, want maxLength", reason)
	}
}

func TestBeadSpecMarksPriorityNullable(t *testing.T) {
	spec := readCommittedOpenAPISpec(t)

	components, _ := spec["components"].(map[string]any)
	schemas, _ := components["schemas"].(map[string]any)
	bead, _ := schemas["Bead"].(map[string]any)
	props, _ := bead["properties"].(map[string]any)
	priority, _ := props["priority"].(map[string]any)
	if priority == nil {
		t.Fatalf("Bead priority schema missing from properties: %v", props)
	}

	types, ok := priority["type"].([]any)
	if !ok {
		t.Fatalf("Bead priority schema type = %#v, want [\"integer\", \"null\"]", priority["type"])
	}
	seen := map[string]bool{}
	for _, typ := range types {
		if s, ok := typ.(string); ok {
			seen[s] = true
		}
	}
	if !seen["integer"] || !seen["null"] {
		t.Fatalf("Bead priority schema type = %#v, want integer nullable", types)
	}
}
