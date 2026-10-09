package model

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestChatRequestPreservesExplicitAssertionSelection(t *testing.T) {
	input := `{"content":"Show saved assertion: []","assertion_selection":{"asserting_document_id":"00000000-0000-0000-0000-000000000001","assertion_id":"00000000-0000-0000-0000-000000000002","revision":2}}`
	var request ChatMessageRequest
	if err := json.Unmarshal([]byte(input), &request); err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(encoded), `"assertion_selection"`) {
		t.Fatalf("selection silently discarded: %s", encoded)
	}
}
