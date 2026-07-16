package response

import "testing"

func TestFriendlyBadRequestHidesTechnicalErrors(t *testing.T) {
	tests := []string{
		"json: cannot unmarshal string into Go struct field CreatePlanRequest.price_monthly of type float64",
		"Key: 'Request.Name' Error:Field validation for 'Name' failed on the 'required' tag",
		"database error: SQL constraint failed",
		"invalid character '}' looking for beginning of value",
	}
	for _, input := range tests {
		if got := friendlyBadRequest(input); got != invalidRequestMessage {
			t.Fatalf("friendlyBadRequest(%q) = %q", input, got)
		}
	}
}

func TestFriendlyBadRequestKeepsBusinessMessage(t *testing.T) {
	const message = "套餐正在使用，无法删除"
	if got := friendlyBadRequest(message); got != message {
		t.Fatalf("friendlyBadRequest(%q) = %q", message, got)
	}
}
