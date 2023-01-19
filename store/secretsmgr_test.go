package store

import (
	"context"
	"errors"
	"fmt"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager/types"
	"github.com/aws/smithy-go/middleware"
	"github.com/guardian/devx-config/log"
	"regexp"
	"testing"
	"time"
)

type SecretsManagerClientMock struct {
	GetSecretValueCallCount  int
	GetSecretValueLastParams secretsmanager.GetSecretValueInput
	GetSecretValueOutputStub func(name *string) (*secretsmanager.GetSecretValueOutput, error)

	CreateSecretCallCount  int
	CreateSecretLastParams secretsmanager.CreateSecretInput
	CreateSecretOutput     secretsmanager.CreateSecretOutput
	CreateSecretError      error

	PutSecretValueCallCount  int
	PutSecretValueLastParams secretsmanager.PutSecretValueInput
	PutSecretValueOutput     secretsmanager.PutSecretValueOutput

	DeleteSecretCallCount  int
	DeleteSecretLastParams secretsmanager.DeleteSecretInput
	DeleteSecretOutput     secretsmanager.DeleteSecretOutput

	ListSecretsCallCount  int
	ListSecretsLastParams secretsmanager.ListSecretsInput
	ListSecretsOutputs    []secretsmanager.ListSecretsOutput
}

func NewSecretsManagerClientMock() *SecretsManagerClientMock {
	return &SecretsManagerClientMock{
		GetSecretValueCallCount:  0,
		GetSecretValueLastParams: secretsmanager.GetSecretValueInput{},
		GetSecretValueOutputStub: func(name *string) (*secretsmanager.GetSecretValueOutput, error) {
			return nil, errors.New("no pre-set value for GetSecretValueOutput")
		},
		CreateSecretCallCount:    0,
		CreateSecretLastParams:   secretsmanager.CreateSecretInput{},
		CreateSecretOutput:       secretsmanager.CreateSecretOutput{},
		PutSecretValueCallCount:  0,
		PutSecretValueLastParams: secretsmanager.PutSecretValueInput{},
		PutSecretValueOutput:     secretsmanager.PutSecretValueOutput{},
		DeleteSecretCallCount:    0,
		DeleteSecretLastParams:   secretsmanager.DeleteSecretInput{},
		DeleteSecretOutput:       secretsmanager.DeleteSecretOutput{},
	}
}

func (m *SecretsManagerClientMock) GetSecretValue(ctx context.Context, params *secretsmanager.GetSecretValueInput, optFunc ...func(options *secretsmanager.Options)) (*secretsmanager.GetSecretValueOutput, error) {
	m.GetSecretValueCallCount++
	m.GetSecretValueLastParams = *params //we intentionally _copy_ the params arg here in case it gets modified elsewhere
	return m.GetSecretValueOutputStub(params.SecretId)
}

func (m *SecretsManagerClientMock) CreateSecret(ctx context.Context, params *secretsmanager.CreateSecretInput, optFunc ...func(*secretsmanager.Options)) (*secretsmanager.CreateSecretOutput, error) {
	m.CreateSecretCallCount++
	m.CreateSecretLastParams = *params //we intentionally _copy_ the params arg here in case it gets modified elsewhere

	if m.CreateSecretError != nil {
		return nil, m.CreateSecretError
	} else {
		rtnContent := m.CreateSecretOutput //again we intentionally copy the output here so as not to return a pointer to our own copy which can then be mutated
		return &rtnContent, nil
	}
}

func (m *SecretsManagerClientMock) PutSecretValue(ctx context.Context, params *secretsmanager.PutSecretValueInput, optFunc ...func(*secretsmanager.Options)) (*secretsmanager.PutSecretValueOutput, error) {
	m.PutSecretValueCallCount++
	m.PutSecretValueLastParams = *params //we intentionally _copy_ the params arg here in case it gets modified elsewhere
	rtnContent := m.PutSecretValueOutput //again we intentionally copy the output here so as not to return a pointer to our own copy which can then be mutated

	return &rtnContent, nil
}

func (m *SecretsManagerClientMock) DeleteSecret(ctx context.Context, params *secretsmanager.DeleteSecretInput, optFunc ...func(*secretsmanager.Options)) (*secretsmanager.DeleteSecretOutput, error) {
	m.DeleteSecretCallCount++
	m.DeleteSecretLastParams = *params //we intentionally _copy_ the params arg here in case it gets modified elsewhere
	rtnContent := m.DeleteSecretOutput //again we intentionally copy the output here so as not to return a pointer to our own copy which can then be mutated

	return &rtnContent, nil
}

func (m *SecretsManagerClientMock) ListSecrets(ctx context.Context, params *secretsmanager.ListSecretsInput, optFunc ...func(*secretsmanager.Options)) (*secretsmanager.ListSecretsOutput, error) {

	m.ListSecretsLastParams = *params //we intentionally _copy_ the params arg here in case it gets modified elsewhere
	idx := m.ListSecretsCallCount
	if idx > len(m.ListSecretsOutputs) {
		idx = len(m.ListSecretsOutputs) - 1
	}
	rtnContent := m.ListSecretsOutputs[idx] //again we intentionally copy the output here so as not to return a pointer to our own copy which can then be mutated

	m.ListSecretsCallCount++

	return &rtnContent, nil
}

func TestSecretsManager_Get(t *testing.T) {
	s := Service{
		Stack: "my-stack",
		Stage: "TEST",
		App:   "my-app",
	}

	toTest := SecretsManager{
		logger:              log.New(false),
		client:              NewSecretsManagerClientMock(),
		SecretRetentionDays: 0,
		DefaultTimeout:      10,
	}

	mock := toTest.client.(*SecretsManagerClientMock)
	ts, _ := time.Parse(time.RFC3339, "2023-01-02T03:04:05Z")

	mock.GetSecretValueOutputStub = func(name *string) (*secretsmanager.GetSecretValueOutput, error) {
		return &secretsmanager.GetSecretValueOutput{
			ARN:            aws.String("arn:aws:some-region::::/TEST/my-stack/my-app/some-parameter/path"),
			CreatedDate:    &ts,
			Name:           aws.String("/TEST/my-stack/my-app/some-parameter/path"),
			SecretBinary:   nil,
			SecretString:   aws.String("somevaluehere"),
			VersionId:      aws.String("dshjdfsdfs"),
			VersionStages:  nil,
			ResultMetadata: middleware.Metadata{},
		}, nil
	}

	result, err := toTest.Get(s, "some-parameter/path")

	if mock.GetSecretValueCallCount != 1 {
		t.Error("Expected GetSecretValue to be called once, got ", mock.GetSecretValueCallCount)
	}
	if *mock.GetSecretValueLastParams.SecretId != "/TEST/my-stack/my-app/some-parameter/path" {
		t.Error("Expected GetSecretValue to be called on path")
	}
	if err != nil {
		t.Error("Unexpected error calling SecretsManager.get: ", err)
		t.FailNow()
	}

	if result.IsSecret != true {
		t.Error("result should always have IsSecret=true")
	}
	if result.Value != "somevaluehere" {
		t.Errorf("result value was wrong, got '%s' expected 'somevaluehere'", result.Value)
	}
	if result.Name != "/TEST/my-stack/my-app/some-parameter/path" {
		t.Errorf("result name was wrong, got '%s' expected '/TEST/my-stack/my-app/some-parameter/path'", result.Name)
	}

	mock.GetSecretValueOutputStub = func(name *string) (*secretsmanager.GetSecretValueOutput, error) {
		return &secretsmanager.GetSecretValueOutput{
			ARN:            aws.String("arn:aws:some-region::::/TEST/my-stack/my-app/some-parameter/path"),
			CreatedDate:    &ts,
			Name:           aws.String("/TEST/my-stack/my-app/some-parameter/path"),
			SecretBinary:   []byte("dfdfsfs"),
			SecretString:   nil,
			VersionId:      aws.String("dshjdfsdfs"),
			VersionStages:  nil,
			ResultMetadata: middleware.Metadata{},
		}, nil
	}

	nullResult, err := toTest.Get(s, "some-parameter/path")
	if err != nil {
		t.Error("Unexpected error retrieving second test ", err)
		t.FailNow()
	}
	if nullResult.Value != "" {
		t.Error("Expected an empty string for the value of a nil secretstring but got ", nullResult.Value)
	}
}

func TestSecretsManager_Set_Nonsecret(t *testing.T) {
	s := Service{
		Stack: "my-stack",
		Stage: "TEST",
		App:   "my-app",
	}
	toTest := SecretsManager{
		logger:              log.New(false),
		client:              NewSecretsManagerClientMock(),
		SecretRetentionDays: 0,
		DefaultTimeout:      10,
	}
	mock := toTest.client.(*SecretsManagerClientMock)

	err := toTest.Set(s, "some-new-secret", "some-secret-value", false)
	if err == nil {
		t.Error("Set should fail on secretsmanager store if `isSecret` is false")
	}
	if mock.CreateSecretCallCount != 0 {
		t.Error("Set should not attempt to call CreateSecret if `isSecret` is false")
	}
	if mock.PutSecretValueCallCount != 0 {
		t.Error("Set should not attempt to call PutSecretValue if `isSecret` is false")
	}
}

func TestSecretsManager_Set_Nonexisting(t *testing.T) {
	s := Service{
		Stack: "my-stack",
		Stage: "TEST",
		App:   "my-app",
	}

	toTest := SecretsManager{
		logger:              log.New(false),
		client:              NewSecretsManagerClientMock(),
		SecretRetentionDays: 0,
		DefaultTimeout:      10,
	}

	mock := toTest.client.(*SecretsManagerClientMock)

	mock.CreateSecretOutput = secretsmanager.CreateSecretOutput{
		ARN:               aws.String("arn:aws:::::/TEST/my-stack/my-app/some-new-secret"),
		Name:              aws.String("/TEST/my-stack/my-app/some-new-secret"),
		ReplicationStatus: nil,
		VersionId:         aws.String("fsdjdfs"),
		ResultMetadata:    middleware.Metadata{},
	}

	err := toTest.Set(s, "some-new-secret", "some-secret-value", true)
	if err != nil {
		t.Error("Unexpected error when trying to set a new value: ", err)
	}
	if mock.CreateSecretCallCount != 1 {
		t.Errorf("Set should call CreateSecret once but got %d times", mock.CreateSecretCallCount)
	}
	if mock.PutSecretValueCallCount != 0 {
		t.Error("Set should only call PutSecretValue if the secret already exists")
	}
}

func TestSecretsManager_Set_Preexisting(t *testing.T) {
	s := Service{
		Stack: "my-stack",
		Stage: "TEST",
		App:   "my-app",
	}

	toTest := SecretsManager{
		logger:              log.New(false),
		client:              NewSecretsManagerClientMock(),
		SecretRetentionDays: 0,
		DefaultTimeout:      10,
	}

	mock := toTest.client.(*SecretsManagerClientMock)

	mock.CreateSecretError = &types.ResourceExistsException{
		Message:           aws.String("this already exists"),
		ErrorCodeOverride: aws.String("409"),
	}
	mock.PutSecretValueOutput = secretsmanager.PutSecretValueOutput{
		ARN:            aws.String("arn:aws:::::/TEST/my-stack/my-app/some-new-secret"),
		Name:           aws.String("/TEST/my-stack/my-app/some-new-secret"),
		VersionId:      aws.String("fsdjdfs"),
		ResultMetadata: middleware.Metadata{},
	}
	err := toTest.Set(s, "some-new-secret", "some-secret-value", true)
	if err != nil {
		t.Error("Unexpected error when trying to update an existing value: ", err)
	}
	if mock.CreateSecretCallCount != 1 {
		t.Errorf("Set should call CreateSecret once but got %d times", mock.CreateSecretCallCount)
	}
	if mock.PutSecretValueCallCount != 1 {
		t.Errorf("Set should always call PutSecretValue if the secret already exists, got %d times", mock.PutSecretValueCallCount)
	}
}

func TestSecretsManager_Set_Erroring(t *testing.T) {
	s := Service{
		Stack: "my-stack",
		Stage: "TEST",
		App:   "my-app",
	}

	toTest := SecretsManager{
		logger:              log.New(false),
		client:              NewSecretsManagerClientMock(),
		SecretRetentionDays: 0,
		DefaultTimeout:      10,
	}

	mock := toTest.client.(*SecretsManagerClientMock)

	mock.CreateSecretError = errors.New("something else went wrong")

	err := toTest.Set(s, "some-new-secret", "some-secret-value", true)
	if err == nil {
		t.Error("we should have received an error from Set but did not get anything")
	}
	if mock.CreateSecretCallCount != 1 {
		t.Errorf("Set should call CreateSecret once but got %d times", mock.CreateSecretCallCount)
	}
	if mock.PutSecretValueCallCount != 0 {
		t.Error("Set should only call PutSecretValue if the secret already exists")
	}
}

func TestSecretsManager_List(t *testing.T) {
	s := Service{
		Stack: "my-stack",
		Stage: "TEST",
		App:   "my-app",
	}

	toTest := SecretsManager{
		logger:              log.New(false),
		client:              NewSecretsManagerClientMock(),
		SecretRetentionDays: 0,
		DefaultTimeout:      10,
	}

	mock := toTest.client.(*SecretsManagerClientMock)
	mock.ListSecretsOutputs = []secretsmanager.ListSecretsOutput{
		{
			NextToken: aws.String("somenexttoken-1"),
			SecretList: []types.SecretListEntry{
				{
					ARN:  aws.String("arn:aws:::::secret-1-id"),
					Name: aws.String("secret-1-id"),
				},
				{
					ARN:  aws.String("arn:aws:::::secret-2-id"),
					Name: aws.String("secret-2-id"),
				},
			},
		},
		{
			NextToken: aws.String("somenexttoken-2"),
			SecretList: []types.SecretListEntry{
				{
					ARN:  aws.String("arn:aws:::::secret-3-id"),
					Name: aws.String("secret-3-id"),
				},
			},
		},
		{
			NextToken: nil,
			SecretList: []types.SecretListEntry{
				{
					ARN:  aws.String("arn:aws:::::secret-4-id"),
					Name: aws.String("secret-4-id"),
				},
			},
		},
	}
	mock.GetSecretValueOutputStub = func(name *string) (*secretsmanager.GetSecretValueOutput, error) {
		matcher := regexp.MustCompile("secret-(\\d+)-id$")
		matches := matcher.FindAllStringSubmatch(*name, -1)
		if matches == nil {
			t.Error("Unexpected secret name requested: ", *name)
			return nil, errors.New("unexpected secret name requested")
		} else {
			secretIdNumber := matches[0][1]
			ts := time.Now()
			return &secretsmanager.GetSecretValueOutput{
				ARN:            aws.String(*name), //intentionally copy the string value
				CreatedDate:    &ts,
				Name:           aws.String(fmt.Sprintf("secret-%s-id", secretIdNumber)),
				SecretBinary:   nil,
				SecretString:   aws.String(fmt.Sprintf("secret-%s-value", secretIdNumber)),
				VersionId:      aws.String("someversionid"),
				VersionStages:  nil,
				ResultMetadata: middleware.Metadata{},
			}, nil
		}
	}

	results, err := toTest.List(s)
	if err != nil {
		t.Error(err)
		t.FailNow()
	}

	if len(results) != 4 {
		t.Errorf("Expected 4 results but got %d", len(results))
	}
	for i := 0; i < len(results); i++ {
		if results[i].IsSecret != true {
			t.Error("IsSecret should always be set to true")
		}
		if results[i].Service != s {
			t.Error("service is not set correctly")
		}
		expectedName := fmt.Sprintf("secret-%d-id", i+1)
		expectedValue := fmt.Sprintf("secret-%d-value", i+1)
		if results[i].Name != expectedName {
			t.Errorf("expected secret name to be %s but got %s", expectedName, results[i].Name)
		}
		if results[i].Value != expectedValue {
			t.Errorf("expected secret value to be %s but got %s", expectedValue, results[i].Value)
		}
	}
}

func TestSecretsManager_DeleteImmediate(t *testing.T) {
	s := Service{
		Stack: "my-stack",
		Stage: "TEST",
		App:   "my-app",
	}

	toTest := SecretsManager{
		logger:              log.New(false),
		client:              NewSecretsManagerClientMock(),
		SecretRetentionDays: 0,
		DefaultTimeout:      10,
	}

	mock := toTest.client.(*SecretsManagerClientMock)
	ts := time.Now()
	mock.DeleteSecretOutput = secretsmanager.DeleteSecretOutput{
		ARN:            aws.String("arn:aws::::/TEST/my-stack/my-app/some-secret-name"),
		DeletionDate:   &ts,
		Name:           aws.String("/TEST/my-stack/my-app/some-secret-name"),
		ResultMetadata: middleware.Metadata{},
	}
	err := toTest.Delete(s, "some-secret-name")
	if err != nil {
		t.Error("unexpected error deleting secret: ", err)
	}
	if mock.DeleteSecretCallCount != 1 {
		t.Errorf("DeleteSecret should have been called once but was called %d times", mock.DeleteSecretCallCount)
	}
	expectedName := "/TEST/my-stack/my-app/some-secret-name"
	if *mock.DeleteSecretLastParams.SecretId != expectedName {
		t.Errorf("DeleteSecret would have deleted '%s' but it should have been '%s'", *mock.DeleteSecretLastParams.SecretId, expectedName)
	}
	if mock.DeleteSecretLastParams.RecoveryWindowInDays != nil {
		t.Errorf("DeleteSecret should have deleted immediately but recovery window was %d", *mock.DeleteSecretLastParams.RecoveryWindowInDays)
	}
	if mock.DeleteSecretLastParams.ForceDeleteWithoutRecovery == nil || !*mock.DeleteSecretLastParams.ForceDeleteWithoutRecovery {
		t.Error("DeleteSecret should have set ForceDeleteWithoutRecovery but it did not")
	}
}

func TestSecretsManager_DeleteDelayed(t *testing.T) {
	s := Service{
		Stack: "my-stack",
		Stage: "TEST",
		App:   "my-app",
	}

	toTest := SecretsManager{
		logger:              log.New(false),
		client:              NewSecretsManagerClientMock(),
		SecretRetentionDays: 7,
		DefaultTimeout:      10,
	}

	mock := toTest.client.(*SecretsManagerClientMock)
	ts := time.Now().Add(7 * 24 * time.Hour)
	mock.DeleteSecretOutput = secretsmanager.DeleteSecretOutput{
		ARN:            aws.String("arn:aws::::/TEST/my-stack/my-app/some-secret-name"),
		DeletionDate:   &ts,
		Name:           aws.String("/TEST/my-stack/my-app/some-secret-name"),
		ResultMetadata: middleware.Metadata{},
	}

	err := toTest.Delete(s, "some-secret-name")
	if err != nil {
		t.Error("unexpected error deleting secret: ", err)
	}
	if mock.DeleteSecretCallCount != 1 {
		t.Errorf("DeleteSecret should have been called once but was called %d times", mock.DeleteSecretCallCount)
	}
	expectedName := "/TEST/my-stack/my-app/some-secret-name"
	if *mock.DeleteSecretLastParams.SecretId != expectedName {
		t.Errorf("DeleteSecret would have deleted '%s' but it should have been '%s'", *mock.DeleteSecretLastParams.SecretId, expectedName)
	}
	if mock.DeleteSecretLastParams.RecoveryWindowInDays == nil {
		t.Errorf("DeleteSecret should have deleted with a recovery window but did it immediately")
	} else if *mock.DeleteSecretLastParams.RecoveryWindowInDays != 7 {
		t.Errorf("DeleteSecret window recovery window should have been 7 days but was %d", *mock.DeleteSecretLastParams.RecoveryWindowInDays)
	}

	//we should use the recovery window here, so ForceDeleteWithoutRecovery must BE either nil OR a pointer to `false`
	//logically this is equivalent to ForceDeleteWithoutRecovery must NOT be set AND a pointer to `true`
	if mock.DeleteSecretLastParams.ForceDeleteWithoutRecovery != nil && *mock.DeleteSecretLastParams.ForceDeleteWithoutRecovery {
		t.Error("DeleteSecret have not set ForceDeleteWithoutRecovery but it did")
	}
}
