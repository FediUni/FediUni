package actor

import (
	"context"
	"github.com/go-fed/activity/streams"
	"github.com/go-fed/activity/streams/vocab"
	"github.com/google/go-cmp/cmp"
	"net/url"
	"testing"
)

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

func TestParseActor(t *testing.T) {
	tests := []struct {
		name    string
		actor   vocab.Type
		want    Actor
		wantErr bool
	}{
		{
			name:  "Test Parsing Typical Person",
			actor: generateTestPerson(),
			want:  generateTestPerson(),
		},
		{
			name:  "Test Parsing Typical Service",
			actor: generateTestService(),
			want:  generateTestService(),
		},
		{
			name:    "Test Parsing nil Actor",
			actor:   nil,
			want:    nil,
			wantErr: true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			a, err := ParseActor(context.Background(), test.actor)
			if err != nil && !test.wantErr {
				t.Fatalf("Failed to Parse Actor: got err=%v", err)
			}
			var gotActor, wantActor map[string]interface{}
			if a != nil {
				gotActor, err = streams.Serialize(a)
				if err != nil && !test.wantErr {
					t.Fatalf("Failed to Serialize Actor: got err=%v", err)
				}
			}
			if test.want != nil {
				wantActor, err = streams.Serialize(test.want)
				if err != nil && !test.wantErr {
					t.Fatalf("Failed to Serialize Want Actor: got err=%v", err)
				}
			}
			if d := cmp.Diff(wantActor, gotActor); d != "" {
				t.Errorf("ParseActor() returned an unexpected diff: (+got -want) %s", d)
			}
		})
	}
}

func generateTestPerson() vocab.ActivityStreamsPerson {
	p := streams.NewActivityStreamsPerson()
	id, _ := url.Parse("http://testserver.com/actor/brandonstark")
	idProperty := streams.NewJSONLDIdProperty()
	idProperty.Set(id)
	p.SetJSONLDId(idProperty)
	return p
}

func generateTestService() vocab.ActivityStreamsService {
	s := streams.NewActivityStreamsService()
	id, _ := url.Parse("http://testserver.com/actor/testbot")
	idProperty := streams.NewJSONLDIdProperty()
	idProperty.Set(id)
	s.SetJSONLDId(idProperty)
	return s
}
