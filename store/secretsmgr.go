package store

import (
	"context"
	"crypto/md5"
	"encoding/base64"
	"errors"
	"fmt"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager/types"
	"github.com/guardian/devx-config/log"
	"io"
	"time"
)

type SecretsManager struct {
	logger              log.Logger
	client              *secretsmanager.Client
	SecretRetentionDays int64 //time in days to keep a deleted secret
	DefaultTimeout      time.Duration
}

func NewSecretsManagerStore(cfg aws.Config, secretsRetentionDays int64, debugLogging bool, defaultTimeout time.Duration) (Store, error) {
	client := secretsmanager.NewFromConfig(cfg)

	if secretsRetentionDays != 0 && (secretsRetentionDays < 7 || secretsRetentionDays > 30) {
		return nil, errors.New("aws only supports post-deletion retention periods of between 7 and 30 days. Use 0 to force it to delete immediately")
	}

	return &SecretsManager{
		logger:              log.New(debugLogging),
		client:              client,
		SecretRetentionDays: secretsRetentionDays,
		DefaultTimeout:      defaultTimeout,
	}, nil
}

func (s *SecretsManager) timeoutContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), s.DefaultTimeout)
}

/*
Get returns the most recent value of the given secret. NOTE that only string type secrets are supported
at the moment, not binary type.
*/
func (s *SecretsManager) Get(service Service, name string) (Parameter, error) {
	ctx, cancelFunc := s.timeoutContext()
	defer cancelFunc()

	secretNameWithPath := fmt.Sprintf("%s/%s", service.Prefix(), name)
	req := &secretsmanager.GetSecretValueInput{
		SecretId: &secretNameWithPath,
	}

	response, err := s.client.GetSecretValue(ctx, req)
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
func (s *SecretsManager) fetchSecretValue(entry *types.SecretListEntry) (string, error) {
	ctx, cancelFunc := s.timeoutContext()
	defer cancelFunc()

	req := &secretsmanager.GetSecretValueInput{
		SecretId: entry.ARN,
	}
	response, err := s.client.GetSecretValue(ctx, req)
	if err == nil {
		if response.SecretString == nil {
			return "", nil
		} else {
			return *response.SecretString, nil
		}
	} else {
		return "", err
	}
}

func (s *SecretsManager) nextPageOfResults(service Service, nextToken *string, currentResults *[]Parameter) (*[]Parameter, error) {
	req := &secretsmanager.ListSecretsInput{
		Filters: []types.Filter{
			{
				Key: "name",
				Values: []string{
					service.Prefix(),
				},
			},
		},
		NextToken: nextToken,
		SortOrder: types.SortOrderTypeDesc,
	}

	ctx, cancelFunc := s.timeoutContext()
	response, err := s.client.ListSecrets(ctx, req)
	cancelFunc() //ensure that the context is cleared up
	if err != nil {
		return nil, err
	}

	results := make([]Parameter, len(response.SecretList))
	for i, entry := range response.SecretList {
		secretValue, err := s.fetchSecretValue(&entry)
		if err != nil {
			return nil, err
		}
		results[i] = Parameter{
			Service:  service,
			Name:     *entry.Name,
			Value:    secretValue,
			IsSecret: true,
		}
	}

	updatedCompleteResults := append(*currentResults, results...)
	if response.NextToken != nil {
		s.logger.Debugf("Loading next page of secrets...")
		return s.nextPageOfResults(service, response.NextToken, &updatedCompleteResults)
	} else {
		return &updatedCompleteResults, nil
	}
}

func (s *SecretsManager) List(service Service) ([]Parameter, error) {
	resultList := make([]Parameter, 0)
	resultsPtr, err := s.nextPageOfResults(service, nil, &resultList)
	return *resultsPtr, err
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
		Tags: []types.Tag{
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
	ctx, cancelFunc := s.timeoutContext()
	defer cancelFunc()
	response, err := s.client.CreateSecret(ctx, req)
	if err == nil {
		s.logger.Debugf("DEBUG Created new secret with version ID %s and ARN %s", *response.VersionId, *response.ARN)
		return nil
	} else {
		return err
	}
}

func (s *SecretsManager) updateExistingSecret(secretNameWithPath *string, value *string, hashedIdValue *string) error {
	ctx, cancelFunc := s.timeoutContext()
	defer cancelFunc()

	putReq := &secretsmanager.PutSecretValueInput{
		ClientRequestToken: hashedIdValue,
		SecretId:           secretNameWithPath,
		SecretString:       value,
	}
	putResponse, err := s.client.PutSecretValue(ctx, putReq)
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
	var resourceExistsException *types.ResourceExistsException
	if errors.As(err, &resourceExistsException) {
		s.logger.Debugf("DEBUG Secret already exists for name %s, updating existing", secretNameWithPath)
		err = s.updateExistingSecret(&secretNameWithPath, &value, &hashedIdValue)
	}
	return err
}

func (s *SecretsManager) Delete(service Service, name string) error {
	ctx, cancelFunc := s.timeoutContext()
	defer cancelFunc()

	var req *secretsmanager.DeleteSecretInput
	if s.SecretRetentionDays > 0 {
		req = &secretsmanager.DeleteSecretInput{
			RecoveryWindowInDays: &s.SecretRetentionDays,
			SecretId:             &name,
		}
	} else {
		req = &secretsmanager.DeleteSecretInput{
			SecretId:                   &name,
			ForceDeleteWithoutRecovery: aws.Bool(true),
		}
	}

	response, err := s.client.DeleteSecret(ctx, req)
	if err != nil {
		return err
	} else {
		s.logger.Debugf("DEBUG Requested secret deletion for %s. Actual removal should occur at %s", response.ARN, response.DeletionDate.Format(time.RFC3339))
		return nil
	}
}
