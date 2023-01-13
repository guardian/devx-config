package config

import (
	"io"
	"reflect"
	"strings"
	"testing"
)

func TestRead(t *testing.T) {
	file := io.NopCloser(strings.NewReader(`{"Stack":"deploy","Stage":"PROD","App":"example"}`))

	want := Config{"deploy", "CODE", "example"}
	got, err := Read(Config{Stage: "CODE"}, file)
	if err != nil {
		t.Fatalf("unexpected read error: %v", err)
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got: %v; want %v", got, want)
	}
}
