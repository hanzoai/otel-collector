package datastorelogsexporter

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/collector/pdata/pcommon"
)

func TestInvalidBodyType(t *testing.T) {
	body := pcommon.NewValueStr("test log")
	promoted := buildPromoted(body, map[string]struct{}{})
	assert.Equal(t, pcommon.NewValueMap(), promoted)
}

func TestPromotedPathSeparation(t *testing.T) {
	testCases := []struct {
		name             string
		body             map[string]any
		promotedPaths    map[string]struct{}
		expectedPromoted map[string]any
	}{
		{
			name: "simple_literal_key_match",
			body: map[string]any{
				"message": "test log",
				"level":   "info",
				"user.id": "123",
			},
			promotedPaths: map[string]struct{}{
				"user.id": {},
			},
			expectedPromoted: map[string]any{
				"user.id": "123",
			},
		},
		{
			name: "nested_path_extraction",
			body: map[string]any{
				"message": "test log",
				"user": map[string]any{
					"id":    "123",
					"name":  "john",
					"email": "john@example.com",
				},
			},
			promotedPaths: map[string]struct{}{
				"user.id":   {},
				"user.name": {},
			},
			expectedPromoted: map[string]any{
				"user.id":   "123",
				"user.name": "john",
			},
		},
		{
			name: "parent_is_promoted_but_is_not_leaf_in_data_input",
			body: map[string]any{
				"message": "test log",
				"user": map[string]any{
					"id":    "123",
					"name":  "john",
					"email": "john@example.com",
				},
			},
			promotedPaths: map[string]struct{}{
				"user": {},
			},
			expectedPromoted: map[string]any{},
		},
		{
			name: "array_leaf_found_promoted",
			body: map[string]any{
				"message": "test log",
				"user": map[string]any{
					"orders": []any{
						map[string]any{
							"id":         "1",
							"created_at": "some date",
						},
					},
					"id": "123",
				},
			},
			promotedPaths: map[string]struct{}{
				"user.orders": {},
			},
			expectedPromoted: map[string]any{
				"user.orders": []any{
					map[string]any{
						"id":         "1",
						"created_at": "some date",
					},
				},
			},
		},
		// This is likely not happen, but is covered for completeness
		// Datastore will fail ingestion if there are multiple occurrences of the same path
		// type_json_skip_duplicated_paths can be enabled during parsing JSON object into JSON type duplicated paths will be ignored and only the first one will be inserted instead of an exception
		// https://clickhouse.com/docs/operations/settings/formats#type_json_skip_duplicated_paths
		{
			name: "ambiguous_dot_notation_literal_preference",
			body: map[string]any{
				"message": "test log",
				"a.b.c":   "literal_value", // use first occurrence of the path
				"a": map[string]any{
					"b": map[string]any{
						"c": "nested_value",
					},
				},
			},
			promotedPaths: map[string]struct{}{
				"a.b.c": {},
			},
			expectedPromoted: map[string]any{
				"a.b.c": "literal_value",
			},
		},
		{
			name: "multiple_siblings_same_parent",
			body: map[string]any{
				"message": "test log",
				"user": map[string]any{
					"id":    "123",
					"name":  "john",
					"email": "john@example.com",
					"address": map[string]any{
						"street": "123 Main St",
						"city":   "New York",
					},
				},
			},
			promotedPaths: map[string]struct{}{
				"user.id":           {},
				"user.name":         {},
				"user.address.city": {},
			},
			expectedPromoted: map[string]any{
				"user.id":           "123",
				"user.name":         "john",
				"user.address.city": "New York",
			},
		},
		{
			name: "multiple_siblings_same_parent_with_dot_notation",
			body: map[string]any{
				"message": "test log",
				"user": map[string]any{
					"id":             "123",
					"name":           "john",
					"email":          "john@example.com",
					"address.street": "123 Main St",
					"address.city":   "New York",
				},
			},
			promotedPaths: map[string]struct{}{
				"user.id":           {},
				"user.name":         {},
				"user.address.city": {},
			},
			expectedPromoted: map[string]any{
				"user.id":           "123",
				"user.name":         "john",
				"user.address.city": "New York",
			},
		},
		{
			name: "deeply_nested_paths",
			body: map[string]any{
				"message": "test log",
				"request": map[string]any{
					"headers": map[string]any{
						"authorization": "Bearer token123",
						"content-type":  "application/json",
					},
					"body": map[string]any{
						"user": map[string]any{
							"profile": map[string]any{
								"settings": map[string]any{
									"theme": "dark",
								},
							},
						},
					},
				},
			},
			promotedPaths: map[string]struct{}{
				"request.headers.authorization":            {},
				"request.body.user.profile.settings.theme": {},
			},
			expectedPromoted: map[string]any{
				"request.headers.authorization":            "Bearer token123",
				"request.body.user.profile.settings.theme": "dark",
			},
		},
		{
			name: "no_promoted_paths",
			body: map[string]any{
				"message": "test log",
				"level":   "info",
				"user": map[string]any{
					"id": "123",
				},
			},
			promotedPaths:    map[string]struct{}{},
			expectedPromoted: map[string]any{},
		},
		{
			name: "non_existent_paths",
			body: map[string]any{
				"message": "test log",
				"level":   "info",
			},
			promotedPaths: map[string]struct{}{
				"non.existent.path": {},
				"another.missing":   {},
			},
			expectedPromoted: map[string]any{},
		},
		{
			name: "mixed_literal_and_nested",
			body: map[string]any{
				"message": "test log",
				"a.b.c":   "literal",
				"a": map[string]any{
					"b": map[string]any{
						"d": "nested",
					},
				},
			},
			promotedPaths: map[string]struct{}{
				"a.b.c": {},
				"a.b.d": {},
			},
			expectedPromoted: map[string]any{
				"a.b.c": "literal",
				"a.b.d": "nested",
			},
		},
		{
			name: "nested_after_match_is_found",
			body: map[string]any{
				"message": "test log",
				"a.b.c":   "literal",
				"a": map[string]any{
					"b": map[string]any{
						"d": map[string]any{
							"nested_key": "nested_value",
						},
					},
				},
			},
			promotedPaths: map[string]struct{}{
				"a.b.c": {},
				"a.b.d": {},
			},
			expectedPromoted: map[string]any{
				"a.b.c": "literal",
			},
		},
		{
			name: "nested_after_match_is_found_with_dot_notation",
			body: map[string]any{
				"message": "test log",
				"a.b.c":   "literal",
				"a": map[string]any{
					"b.d": map[string]any{
						"nested_key": "nested_value",
					},
				},
			},
			promotedPaths: map[string]struct{}{
				"a.b.c": {},
				"a.b.d": {},
			},
			expectedPromoted: map[string]any{
				"a.b.c": "literal",
			},
		},
		{
			name: "nested_2_matches_found",
			body: map[string]any{
				"message": "test log",
				"a.b.c": map[string]any{
					"nested_key": "nested_value",
				},
				"a": map[string]any{
					"b.c": "literal",
				},
			},
			promotedPaths: map[string]struct{}{
				"a.b.c": {},
			},
			expectedPromoted: map[string]any{
				"a.b.c": "literal",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create pcommon.Value from test body
			body := pcommon.NewValueMap()
			err := body.Map().FromRaw(tc.body)
			assert.NoError(t, err, "failed to convert map to pcommon.Map")

			// Extract promoted paths
			promoted := buildPromoted(body, tc.promotedPaths)

			// Convert results back to interface{} for comparison
			actualBody := body.Map().AsRaw()
			actualPromoted := promoted.Map().AsRaw()

			// Assertions
			assert.Equal(t, tc.body, actualBody, "Body should stay the same after extraction")
			assert.Equal(t, tc.expectedPromoted, actualPromoted, "Promoted should match expected")
		})
	}
}
