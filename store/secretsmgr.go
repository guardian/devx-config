package store

import (
	"crypto/md5"
	"encoding/base64"
	"errors"
	"fmt"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/guardian/devx-config/log"
	"io"
)

type SecretsManager struct {
	logger              log.Logger
	client              *secretsmanager.Client
	SecretRetentionDays int64 //time in days to keep a deleted secret
}

func NewSecretsManagerStore(cfg aws.Config, secretsRetentionDays int64, debugLogging bool) Store {
	client := secretsmanager.NewFromConfig(cfg)

	return &SecretsManager{
		logger:              log.New(debugLogging),
		client:              client,
		SecretRetentionDays: secretsRetentionDays,
	}
}

/*
Get returns the most recent value of the given secret. NOTE that only string type secrets are supported
at the moment, not binary type.
*/
func (s *SecretsManager) Get(service Service, name string) (Parameter, error) {
	secretNameWithPath := fmt.Sprintf("%s/%s", service.Prefix(), name)
	req := &secretsmanager.GetSecretValueInput{
		SecretId: &secretNameWithPath,
	}

	response, err := s.client.GetSecretValue(req)
	if err != nil {
		return Parameter{}, err
	}

	safeValue := ""
	if response.SecretString != nil {
		safeValue = *response.SecretString
	}

	return Parameter{
		Name:     *response.Name,
		Value:    safeValue,
		IsSecret: true,
		Service:  service,
	}, nil
}

/*
fetchSecretValue returns the most recent version of the listed SecretListEntry.
This two-stage retrieval is necessary because the List call does _not_ decrypt the
secrets.
NOTE: we assume that secrets are strings, not binary values, for the time being
*/
func (s *SecretsManager) fetchSecretValue(entry *secretsmanager.SecretListEntry) (*string, error) {
	req := &secretsmanager.GetSecretValueInput{
		SecretId: entry.ARN,
	}
	response, err := s.client.GetSecretValue(req)
	if err == nil {
		return response.SecretString, nil
	} else {
		return nil, err
	}
}

func (s *SecretsManager) List(service Service) ([]Parameter, error) {
	req := &secretsmanager.ListSecretsInput{
		Filters: []*secretsmanager.Filter{
			{
				Key: aws.String(secretsmanager.FilterNameStringTypeName),
				Values: []*string{
					aws.String(service.Prefix()),
				},
			},
		},
		MaxResults: nil,
		NextToken:  nil,
		SortOrder:  aws.String(secretsmanager.SortOrderTypeDesc),
	}
	var wrappedError error
	results := make([]Parameter, 0)

	err := s.client.ListSecretsPages(req, func(output *secretsmanager.ListSecretsOutput, lastPage bool) bool {
		newParams := make([]Parameter, len(output.SecretList))
		for i, entry := range output.SecretList {
			value, wrappedError := s.fetchSecretValue(entry)
			if wrappedError != nil {
				s.logger.Infof("ERROR Could not fetch value for secret %s: %s", *entry.ARN, wrappedError)
				return false //stops iterating the pages of results
			}
			safeValue := ""
			if value != nil {
				safeValue = *value
			}
			newParams[i] = Parameter{
				Service:  service,
				Name:     *entry.Name,
				Value:    safeValue,
				IsSecret: true, //if it comes from the secrets manager store, then by definition it's a secret :)
			}
		}
		results = append(results, newParams...)
		return true //continue iterating pages of results
	})
	if err != nil {
		return nil, err
	}
	if wrappedError != nil {
		return nil, wrappedError
	}
	return results, nil
}

//getVersionIdHash returns a checksum to use as a unique version ID for the given string
func getVersionIdHash(value string) string {
	hasher := md5.New()
	io.WriteString(hasher, value)
	rawMd5 := hasher.Sum(nil)
	return base64.StdEncoding.EncodeToString(rawMd5)
}

func (s *SecretsManager) createNewSecret(service Service, secretNameWithPath *string, value *string, hashedIdValue *string) error {
	//the CreateSecret operation will silently fail if we have the same values already present but will error if there is already a version.
	//we catch this error and retry so we can create a new version instead
	req := &secretsmanager.CreateSecretInput{
		ClientRequestToken: hashedIdValue,
		Name:               secretNameWithPath,
		SecretString:       value,
		Tags: []*secretsmanager.Tag{
			{
				Key:   aws.String("App"),
				Value: aws.String(service.App),
			},
			{
				Key:   aws.String("Stack"),
				Value: aws.String(service.Stack),
			},
			{
				Key:   aws.String("Stage"),
				Value: aws.String(service.Stage),
			},
		},
	}
	response, err := s.client.CreateSecret(req)
	if err == nil {
		s.logger.Debugf("DEBUG Created new secret with version ID %s and ARN %s", *response.VersionId, *response.ARN)
		return nil
	} else {
		return err
	}
}

func (s *SecretsManager) updateExistingSecret(secretNameWithPath *string, value *string, hashedIdValue *string) error {
	putReq := &secretsmanager.PutSecretValueInput{
		ClientRequestToken: hashedIdValue,
		SecretId:           secretNameWithPath,
		SecretString:       value,
	}
	putResponse, err := s.client.PutSecretValue(putReq)
	if err != nil {
		s.logger.Infof("ERROR Could not update value for secret %s: %s", secretNameWithPath, value)
		return err
	} else {
		s.logger.Debugf("DEBUG Updated secret with new version ID %s and ARN %s", *putResponse.VersionId, *putResponse.ARN)
		return nil
	}
}

func (s *SecretsManager) Set(service Service, name string, value string, isSecret bool) error {
	if !isSecret {
		return errors.New("you cannot create something that is not a secret in the Secrets store. You should use SSM instead")
	}

	secretNameWithPath := fmt.Sprintf("%s/%s", service.Prefix(), name)
	hashedIdValue := getVersionIdHash(value)
	err := s.createNewSecret(service, &secretNameWithPath, &value, &hashedIdValue)

	//OK, something went wrong. What was it?
	var resourceExistsException *secretsmanager.ResourceExistsException
	if errors.As(err, &resourceExistsException) {
		s.logger.Debugf("DEBUG Secret already exists for name %s, updating existing", secretNameWithPath)
		err = s.updateExistingSecret(&secretNameWithPath, &value, &hashedIdValue)
	}
	return err
}

func (s *SecretsManager) Delete(service Service, name string) error {
	req := &secretsmanager.DeleteSecretInput{
		RecoveryWindowInDays: &s.SecretRetentionDays,
		SecretId:             nil,
	}
}
