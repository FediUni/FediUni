package actor

import "testing"

func TestIsIdentifier(t *testing.T) {
	tests := []struct {
		name       string
		identifier string
		wantRes    bool
	}{
		{
			name:       "Test valid identifier",
			identifier: "@brandonstark@test.com",
			wantRes:    true,
		},
		{
			name:       "Test invalid identifier missing leading @",
			identifier: "brandonstark@test.com",
			wantRes:    false,
		},
		{
			name:       "Test invalid identifier missing middle @",
			identifier: "@brandonstarktest.com",
			wantRes:    false,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			gotRes, _ := IsIdentifier(test.identifier)
			if gotRes != test.wantRes {
				t.Errorf("IsIdentifier(%q) returned an unexpected result: got=%v, want=%v", test.identifier, gotRes, test.wantRes)
			}
		})
	}
}
