package store

import (
	"context"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/aws/aws-sdk-go-v2/service/ssm/types"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/guardian/devx-config/log"
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

type SSM struct {
	logger log.Logger
	client *ssm.Client
}

func NewSSM(logger log.Logger, client *ssm.Client) SSM {
	return SSM{logger, client}
}

func (s SSM) Get(service Service, name string) (Parameter, error) {
	var item Parameter

	output, err := s.client.GetParameter(context.TODO(), &ssm.GetParameterInput{
		Name:           aws.String(service.Prefix() + "/" + name),
		WithDecryption: true,
	})

	if err != nil {
		return item, err
	}

	return asConfigItem(service, *output.Parameter), nil
}

func (s SSM) List(service Service) ([]Parameter, error) {
	pages := ssm.NewGetParametersByPathPaginator(s.client, &ssm.GetParametersByPathInput{
		Path:           aws.String(service.Prefix()),
		WithDecryption: true,
	})

	var items []Parameter
	for pages.HasMorePages() {
		page, err := pages.NextPage(context.TODO())
		if err != nil {
			return items, fmt.Errorf("unable to get parameters: %w", err)
		}

		items = append(items, asConfigItems(service, page.Parameters)...)
	}

	return items, nil
}

func (s SSM) Set(service Service, name string, value string, isSecret bool) error {
	paramType := types.ParameterTypeString
	if isSecret {
		paramType = types.ParameterTypeSecureString
	}

	_, err := s.client.PutParameter(context.TODO(), &ssm.PutParameterInput{
		Name:  aws.String(service.Prefix() + "/" + name),
		Value: &value,
		Type:  paramType,
	})

	return err
}

func (s SSM) Delete(service Service, name string) error {
	_, err := s.client.DeleteParameter(context.TODO(), &ssm.DeleteParameterInput{
		Name: aws.String(service.Prefix() + "/" + name),
	})

	return err
}

func asConfigItems(service Service, params []types.Parameter) []Parameter {
	items := []Parameter{}
	for _, param := range params {
		items = append(items, asConfigItem(service, param))
	}

	return items
}

func asConfigItem(service Service, param types.Parameter) Parameter {
	return Parameter{
		Name:     *param.Name,
		Value:    *param.Value,
		IsSecret: param.Type == types.ParameterTypeSecureString,
		Service:  service,
	}
}
