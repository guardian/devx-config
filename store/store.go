package store

import (
	"fmt"
	"strings"
)

type Service struct {
	Stack, Stage, App string
}

func (s Service) Prefix() string {
	return fmt.Sprintf("/%s/%s/%s", s.Stage, s.Stack, s.App)
}

type Parameter struct {
	Service  Service
	Name     string
	Value    string
	IsSecret bool
}

func (c Parameter) String() string {
	clean := func(s, prefix string) string {
		r := strings.NewReplacer(prefix+"/", "", ".", "_", "/", "_")
		return r.Replace(s)
	}

	return fmt.Sprintf("%s=%s", clean(c.Name, c.Service.Prefix()), c.Value)
}

type Store interface {
	Get(service Service, name string) (Parameter, error)
	List(service Service) ([]Parameter, error)
	Set(service Service, name string, value string, isSecret bool) error
	Delete(service Service, name string) error
}
