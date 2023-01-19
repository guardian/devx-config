package store

import "testing"

func TestService_Prefix(t *testing.T) {
	//we should output STAGE, STACK and APP in that order
	toTest := Service{
		Stack: "my-stack",
		Stage: "TEST",
		App:   "my-app",
	}

	if toTest.Prefix() != "/TEST/my-stack/my-app" {
		t.Errorf("Got incorrect service prefix '%s', expected /TEST/my-stack/my-app", toTest.Prefix())
	}
}

func TestParameter_String(t *testing.T) {
	//we should output the parameter in k=v format, where `k` has the prefix removed, dots and slashes replaced by underscores
	toTest := Parameter{
		Service: Service{
			Stack: "my-stack",
			Stage: "TEST",
			App:   "my-app",
		},
		Name:     "/TEST/my-stack/my-app/some-parameter/with/annoying.characters",
		Value:    "some-value",
		IsSecret: false,
	}

	expected := "some-parameter_with_annoying_characters=some-value"
	if toTest.String() != expected {
		t.Errorf("Got incorrect parameter translation '%s', expected '%s'", toTest.String(), expected)
	}
}
