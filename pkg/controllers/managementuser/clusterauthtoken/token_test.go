package clusterauthtoken

import (
	"fmt"
	"testing"

	clusterv3 "github.com/rancher/rancher/pkg/apis/cluster.cattle.io/v3"
	v3 "github.com/rancher/rancher/pkg/apis/management.cattle.io/v3"
	"github.com/rancher/rancher/pkg/auth/tokens"
	"github.com/rancher/rancher/pkg/auth/tokens/hashers"
	"github.com/rancher/rancher/pkg/features"
	"github.com/rancher/rancher/pkg/generated/norman/cluster.cattle.io/v3/fakes"
	v1 "github.com/rancher/rancher/pkg/generated/norman/core/v1"
	coreFakes "github.com/rancher/rancher/pkg/generated/norman/core/v1/fakes"
	managementv3 "github.com/rancher/rancher/pkg/generated/norman/management.cattle.io/v3"
	mgmtFakes "github.com/rancher/rancher/pkg/generated/norman/management.cattle.io/v3/fakes"
	"github.com/rancher/wrangler/v3/pkg/generic"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/utils/pointer"
)

const (
	userID               = "user-test"
	tokenKey             = "cccccccccccccccccccccccccccccccccccccccccccccccccccccc"
	hashedTokenKey       = "$3:1:GepdvExsvzA:JXMHpXDZqtU5zNh5y5HB8KmLKbHc2VdeuxQo6CTlLhyNifaYhJTnb+4Rf+xpnbsfd8tIlQ0ZgIi2edJrm9CpoA"
	legacyHashedTokenKey = "$2:jwvzsLqh6Rg:FyeWbQuUt6VEMhQOe5J1kXPf0D4H9MRjub0aNaGzyx8"
	invalidHashKey       = "$-1:invalidsalt"
)

func TestCreate(t *testing.T) {
	testToken := &managementv3.Token{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-token",
		},
		ExpiresAt: "10000000000",
		UserID:    userID,
		Token:     tokenKey,
		Enabled:   pointer.Bool(true),
	}

	testAuthToken := &clusterv3.ClusterAuthToken{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-token",
		},
		TypeMeta: metav1.TypeMeta{
			Kind: "ClusterAuthToken",
		},
		ExpiresAt: "10000000000",
		UserName:  userID,
		Enabled:   true,
	}
	testAuthSecret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name: "cat-test-token",
		},
		TypeMeta: metav1.TypeMeta{
			Kind: "Secret",
		},
		Data: map[string][]byte{
			"hash": []byte(legacyHashedTokenKey),
		},
	}

	authTokenNotFoundError := apierrors.NewNotFound(schema.GroupResource{Group: "cluster.cattle.io", Resource: "ClusterAuthToken"}, testToken.Name)
	tests := []struct {
		name  string
		token *managementv3.Token

		existingClusterAuthToken  *clusterv3.ClusterAuthToken
		existingClusterAuthSecret *v1.Secret
		existingTokenError        error
		tokenHashingEnabled       bool
		updateAuthTokenErr        error
		createAuthTokenErr        error

		wantClusterAuthToken bool
		wantAuthTokenUpdate  bool
		wantAuthTokenEnabled bool
		wantAuthTokenDeleted bool
		wantError            bool
		wantSkipError        bool
	}{
		{
			name:                "token hashing disabled, create token",
			token:               testToken,
			existingTokenError:  authTokenNotFoundError,
			tokenHashingEnabled: false,

			wantClusterAuthToken: true,
			wantAuthTokenEnabled: true,
		},
		{
			name:                "token hashing enabled, create token",
			token:               hashToken(testToken, hashedTokenKey),
			existingTokenError:  authTokenNotFoundError,
			tokenHashingEnabled: true,

			wantClusterAuthToken: true,
			wantAuthTokenEnabled: true,
		},
		{
			name:                "token hashing enabled, legacy token hash, don't create token",
			token:               hashToken(testToken, legacyHashedTokenKey),
			existingTokenError:  authTokenNotFoundError,
			tokenHashingEnabled: true,

			wantClusterAuthToken: false,
			wantError:            true,
			wantSkipError:        true,
		},
		{
			name:               "token disabled, create token",
			token:              setTokenEnabled(testToken, pointer.BoolPtr(false)),
			existingTokenError: authTokenNotFoundError,

			wantClusterAuthToken: true,
			wantAuthTokenEnabled: false,
		},
		{
			name:               "token enabled empty, create token",
			token:              setTokenEnabled(testToken, nil),
			existingTokenError: authTokenNotFoundError,

			wantClusterAuthToken: true,
			wantAuthTokenEnabled: true,
		},
		{
			name:                "token hashing enabled, token not hashed yet",
			token:               testToken,
			existingTokenError:  authTokenNotFoundError,
			tokenHashingEnabled: true,

			wantError: true,
		},
		{
			name:                      "existing cluster auth token, update secretHash",
			token:                     hashToken(testToken, hashedTokenKey),
			tokenHashingEnabled:       true,
			existingClusterAuthToken:  testAuthToken,
			existingClusterAuthSecret: testAuthSecret,

			wantClusterAuthToken: true,
			wantAuthTokenUpdate:  true,
			wantAuthTokenEnabled: true,
		},
		{
			name:                "invalid hash version",
			token:               hashToken(testToken, invalidHashKey),
			existingTokenError:  authTokenNotFoundError,
			tokenHashingEnabled: true,

			wantError:     true,
			wantSkipError: true,
		},
		{
			name:               "create error",
			token:              testToken,
			existingTokenError: authTokenNotFoundError,
			createAuthTokenErr: fmt.Errorf("server not available"),

			wantError:            true,
			wantClusterAuthToken: true,
			wantAuthTokenEnabled: true,
			wantAuthTokenDeleted: true,
		},
		{
			name:               "get current token error",
			token:              testToken,
			existingTokenError: fmt.Errorf("server not available"),

			wantError: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			output := runCreateUpdateTest(t, &testInput{
				Token:                     test.token,
				ExistingClusterAuthToken:  test.existingClusterAuthToken,
				ExistingClusterAuthSecret: test.existingClusterAuthSecret,
				ExistingTokenError:        test.existingTokenError,
				TokenHashingEnabled:       test.tokenHashingEnabled,
				UpdateAuthTokenErr:        test.updateAuthTokenErr,
				CreateAuthTokenErr:        test.createAuthTokenErr,
				CallCreate:                true,
			})
			if test.wantError {
				require.Error(t, output.Error)
				if test.wantSkipError {
					require.ErrorIs(t, generic.ErrSkip, output.Error)
				} else {
					require.NotErrorIs(t, generic.ErrSkip, output.Error)
				}
			} else {
				require.NoError(t, output.Error)
			}
			if test.wantClusterAuthToken {
				modifiedToken := output.ModifiedClusterAuthToken
				modifiedSecret := output.ModifiedClusterAuthSecret
				require.NotNil(t, modifiedToken)
				require.Equal(t, test.wantAuthTokenUpdate, output.AuthTokenUpdated)
				require.Equal(t, test.wantAuthTokenDeleted, output.AuthTokenDeleted)

				require.Equal(t, "ClusterAuthToken", modifiedToken.Kind)
				require.Equal(t, test.token.Name, modifiedToken.Name)
				require.Equal(t, test.token.UserID, modifiedToken.UserName)
				require.Equal(t, test.token.ExpiresAt, modifiedToken.ExpiresAt)
				require.Equal(t, test.wantAuthTokenEnabled, modifiedToken.Enabled)

				if modifiedSecret != nil {
					hashedToken := string(modifiedSecret.Data["hash"])
					if test.tokenHashingEnabled {
						// if tokenHashing is enabled hash should be the same on the token and cluster auth token
						require.Equal(t, test.token.Token, hashedToken)
					} else {
						// if tokenHashing is not enabled, the clusterAuthToken will be hashed but the token won't be
						// so we verify that the cluster auth token is a valid hash for the token
						hasher, err := hashers.GetHasherForHash(hashedToken)
						require.NoError(t, err)
						require.NoError(t, hasher.VerifyHash(hashedToken, test.token.Token))
					}
				}
			} else {
				require.Nil(t, output.ModifiedClusterAuthToken)
			}
		})
	}
}

func TestUpdate(t *testing.T) {
	testToken := &managementv3.Token{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-token",
		},
		ExpiresAt: "10000000000",
		UserID:    userID,
		Token:     tokenKey,
		Enabled:   pointer.Bool(true),
	}

	testAuthToken := &clusterv3.ClusterAuthToken{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-token",
		},
		TypeMeta: metav1.TypeMeta{
			Kind: "ClusterAuthToken",
		},
		ExpiresAt: "10000000000",
		UserName:  userID,
		Enabled:   true,
	}
	testAuthSecret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name: "cat-test-token",
		},
		TypeMeta: metav1.TypeMeta{
			Kind: "Secret",
		},
		Data: map[string][]byte{
			"hash": []byte(hashedTokenKey),
		},
	}
	oldAuthToken := testAuthToken.DeepCopy()
	oldAuthSecret := testAuthSecret.DeepCopy()
	oldAuthSecret.Data["hash"] = []byte(legacyHashedTokenKey)

	authTokenNotFoundError := apierrors.NewNotFound(schema.GroupResource{Group: "cluster.cattle.io", Resource: "ClusterAuthToken"}, testToken.Name)
	tests := []struct {
		name  string
		token *managementv3.Token

		existingClusterAuthToken  *clusterv3.ClusterAuthToken
		existingClusterAuthSecret *v1.Secret
		existingTokenError        error
		tokenHashingEnabled       bool
		updateAuthTokenErr        error
		createAuthTokenErr        error

		wantClusterAuthToken bool
		wantAuthTokenUpdate  bool
		wantAuthTokenEnabled bool
		wantError            bool
		wantSkipError        bool
	}{

		{
			name:                      "token disabled, update token",
			token:                     setTokenEnabled(testToken, pointer.Bool(false)),
			existingClusterAuthToken:  testAuthToken,
			existingClusterAuthSecret: testAuthSecret,

			wantClusterAuthToken: true,
			wantAuthTokenEnabled: false,
			wantAuthTokenUpdate:  true,
		},
		{
			name:                      "token enabled missing, no token update",
			token:                     setTokenEnabled(testToken, nil),
			existingClusterAuthToken:  testAuthToken,
			existingClusterAuthSecret: testAuthSecret,

			wantClusterAuthToken: false,
		},
		{
			name:                      "token expiry change, update token",
			token:                     setTokenExpiry(testToken, "2000"),
			existingClusterAuthToken:  testAuthToken,
			existingClusterAuthSecret: testAuthSecret,

			wantClusterAuthToken: true,
			wantAuthTokenUpdate:  true,
			wantAuthTokenEnabled: true,
		},
		{
			name:                      "token username change, update token",
			token:                     setTokenUser(testToken, "new-user"),
			existingClusterAuthToken:  testAuthToken,
			existingClusterAuthSecret: testAuthSecret,

			wantClusterAuthToken: true,
			wantAuthTokenUpdate:  true,
			wantAuthTokenEnabled: true,
		},
		{
			name:                      "token hash change sha3, update token",
			token:                     hashToken(testToken, hashedTokenKey),
			existingClusterAuthToken:  oldAuthToken,
			existingClusterAuthSecret: oldAuthSecret,
			tokenHashingEnabled:       true,

			wantClusterAuthToken: true,
			wantAuthTokenUpdate:  true,
			wantAuthTokenEnabled: true,
		},
		{
			name:                      "token hash change non-sha3, don't update token",
			token:                     hashToken(testToken, legacyHashedTokenKey),
			existingClusterAuthToken:  testAuthToken,
			existingClusterAuthSecret: testAuthSecret,
			tokenHashingEnabled:       true,

			wantClusterAuthToken: false,
		},
		{
			name:                      "no change, don't update",
			token:                     testToken,
			existingClusterAuthToken:  testAuthToken,
			existingClusterAuthSecret: testAuthSecret,

			wantClusterAuthToken: false,
		},
		{
			name:                "token hashing disabled, create token",
			token:               testToken,
			existingTokenError:  authTokenNotFoundError,
			tokenHashingEnabled: false,

			wantClusterAuthToken: true,
			wantAuthTokenEnabled: true,
		},
		{
			name:               "get current token error",
			token:              testToken,
			existingTokenError: fmt.Errorf("server not available"),

			wantError: true,
		},
		{
			name:                      "invalid token hash version",
			token:                     hashToken(testToken, invalidHashKey),
			existingClusterAuthToken:  testAuthToken,
			existingClusterAuthSecret: testAuthSecret,
			tokenHashingEnabled:       true,

			wantError:     true,
			wantSkipError: true,
		},
		{
			name:                      "update auth token error",
			token:                     setTokenUser(testToken, "new-user"),
			existingClusterAuthToken:  testAuthToken,
			existingClusterAuthSecret: testAuthSecret,
			updateAuthTokenErr:        fmt.Errorf("server unavailable"),

			wantError:            true,
			wantAuthTokenUpdate:  true,
			wantClusterAuthToken: true,
			wantAuthTokenEnabled: true,
		},
		{
			name:                      "update auth token not found, create token success",
			token:                     setTokenUser(testToken, "new-user"),
			existingClusterAuthToken:  testAuthToken,
			existingClusterAuthSecret: testAuthSecret,
			updateAuthTokenErr:        authTokenNotFoundError,

			wantClusterAuthToken: true,
			wantAuthTokenUpdate:  true,
			wantAuthTokenEnabled: true,
		},
		{
			name:                      "update auth token not found, create token error",
			token:                     setTokenUser(testToken, "new-user"),
			existingClusterAuthToken:  testAuthToken,
			existingClusterAuthSecret: testAuthSecret,
			updateAuthTokenErr:        authTokenNotFoundError,
			createAuthTokenErr:        fmt.Errorf("server not available"),

			wantError:            true,
			wantClusterAuthToken: true,
			wantAuthTokenUpdate:  true,
			wantAuthTokenEnabled: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			output := runCreateUpdateTest(t, &testInput{
				Token:                     test.token,
				ExistingClusterAuthToken:  test.existingClusterAuthToken,
				ExistingClusterAuthSecret: test.existingClusterAuthSecret,
				ExistingTokenError:        test.existingTokenError,
				TokenHashingEnabled:       test.tokenHashingEnabled,
				UpdateAuthTokenErr:        test.updateAuthTokenErr,
				CreateAuthTokenErr:        test.createAuthTokenErr,
				CallCreate:                false,
			})
			if test.wantError {
				require.Error(t, output.Error)
				if test.wantSkipError {
					require.ErrorIs(t, generic.ErrSkip, output.Error)
				} else {
					require.NotErrorIs(t, generic.ErrSkip, output.Error)
				}
			} else {
				require.NoError(t, output.Error)
			}
			if test.wantClusterAuthToken {
				modifiedToken := output.ModifiedClusterAuthToken
				modifiedSecret := output.ModifiedClusterAuthSecret
				require.NotNil(t, modifiedToken)
				require.Equal(t, test.wantAuthTokenUpdate, output.AuthTokenUpdated)

				require.Equal(t, "ClusterAuthToken", modifiedToken.Kind)
				require.Equal(t, test.token.Name, modifiedToken.Name)
				require.Equal(t, test.token.UserID, modifiedToken.UserName)
				require.Equal(t, test.token.ExpiresAt, modifiedToken.ExpiresAt)
				require.Equal(t, test.wantAuthTokenEnabled, modifiedToken.Enabled)

				if modifiedSecret != nil {
					hashedToken := string(modifiedSecret.Data["hash"])
					if test.tokenHashingEnabled {
						// if tokenHashing is enabled hash should be the same on the token and cluster auth token
						require.Equal(t, test.token.Token, hashedToken)
					} else {
						// if tokenHashing is not enabled, the clusterAuthToken will be hashed but the token won't be
						// so we verify that the cluster auth token is a valid hash for the token
						hasher, err := hashers.GetHasherForHash(hashedToken)
						require.NoError(t, err)
						require.NoError(t, hasher.VerifyHash(hashedToken, test.token.Token))
					}
				}
			} else {
				require.Nil(t, output.ModifiedClusterAuthToken)
			}
		})
	}
}

func hashToken(token *managementv3.Token, hashedToken string) *managementv3.Token {
	newToken := token.DeepCopy()
	newToken.Token = hashedToken
	if newToken.Annotations == nil {
		newToken.Annotations = map[string]string{}
	}
	newToken.Annotations[tokens.TokenHashed] = "true"
	return newToken
}

func setTokenEnabled(token *managementv3.Token, enabled *bool) *managementv3.Token {
	newToken := token.DeepCopy()
	newToken.Enabled = enabled
	return newToken
}

func setTokenExpiry(token *managementv3.Token, expiry string) *managementv3.Token {
	newToken := token.DeepCopy()
	newToken.ExpiresAt = expiry
	return newToken
}

func setTokenUser(token *managementv3.Token, user string) *managementv3.Token {
	newToken := token.DeepCopy()
	newToken.UserID = user
	return newToken
}

type testInput struct {
	Token                     *managementv3.Token
	ExistingClusterAuthToken  *clusterv3.ClusterAuthToken
	ExistingClusterAuthSecret *v1.Secret
	ExistingTokenError        error
	TokenHashingEnabled       bool
	UpdateAuthTokenErr        error
	CreateAuthTokenErr        error
	CallCreate                bool
}

type testOutput struct {
	ModifiedClusterAuthToken  *clusterv3.ClusterAuthToken
	ModifiedClusterAuthSecret *v1.Secret
	AuthTokenUpdated          bool
	AuthTokenDeleted          bool
	Error                     error
}

func runCreateUpdateTest(t *testing.T, testInput *testInput) *testOutput {
	mockLister := fakes.ClusterAuthTokenListerMock{}
	mockLister.GetFunc = func(namespace, name string) (*clusterv3.ClusterAuthToken, error) {
		return testInput.ExistingClusterAuthToken.DeepCopy(), testInput.ExistingTokenError
	}
	mockSecretLister := coreFakes.SecretListerMock{}
	mockSecretLister.GetFunc = func(namespace, name string) (*v1.Secret, error) {
		return testInput.ExistingClusterAuthSecret.DeepCopy(), testInput.ExistingTokenError
	}

	var modifiedSecret *v1.Secret
	mockSecrets := coreFakes.SecretInterfaceMock{}
	mockSecrets.UpdateFunc = func(in1 *v1.Secret) (*v1.Secret, error) {
		modifiedSecret = in1
		return in1, testInput.UpdateAuthTokenErr
	}
	mockSecrets.CreateFunc = func(in1 *v1.Secret) (*v1.Secret, error) {
		modifiedSecret = in1
		return in1, nil
	}

	var modifiedToken *clusterv3.ClusterAuthToken
	var isUpdated bool
	var isDeleted bool
	mockAuthTokens := fakes.ClusterAuthTokenInterfaceMock{}
	mockAuthTokens.UpdateFunc = func(in1 *clusterv3.ClusterAuthToken) (*clusterv3.ClusterAuthToken, error) {
		isUpdated = true
		modifiedToken = in1
		return in1, testInput.UpdateAuthTokenErr
	}
	mockAuthTokens.CreateFunc = func(in1 *clusterv3.ClusterAuthToken) (*clusterv3.ClusterAuthToken, error) {
		modifiedToken = in1
		return in1, testInput.CreateAuthTokenErr
	}
	mockAuthTokens.DeleteFunc = func(name string, options *metav1.DeleteOptions) error {
		isDeleted = true
		return nil
	}

	// cluster userAttributes are also updated in these functions
	userLister := mgmtFakes.UserListerMock{}
	userLister.GetFunc = func(namespace, name string) (*v3.User, error) {
		return &v3.User{
			ObjectMeta: metav1.ObjectMeta{
				Name: userID,
			},
			Enabled: pointer.BoolPtr(true),
		}, nil
	}
	userAttributeLister := mgmtFakes.UserAttributeListerMock{}
	userAttributeLister.GetFunc = func(namespace, name string) (*v3.UserAttribute, error) {
		return &v3.UserAttribute{
			LastRefresh:  "1000",
			NeedsRefresh: false,
		}, nil
	}
	clusterUserAttributeLister := fakes.ClusterUserAttributeListerMock{}
	clusterUserAttributeLister.GetFunc = func(namespace, name string) (*clusterv3.ClusterUserAttribute, error) {
		return &clusterv3.ClusterUserAttribute{
			LastRefresh:  "1000",
			NeedsRefresh: false,
			Enabled:      true,
		}, nil
	}

	features.TokenHashing.Set(testInput.TokenHashingEnabled)
	h := tokenHandler{
		clusterAuthTokenLister:     &mockLister,
		clusterAuthToken:           &mockAuthTokens,
		userLister:                 &userLister,
		userAttributeLister:        &userAttributeLister,
		clusterUserAttributeLister: &clusterUserAttributeLister,
		clusterUserAttribute:       &fakes.ClusterUserAttributeInterfaceMock{},
		clusterSecret:              &mockSecrets,
		clusterSecretLister:        &mockSecretLister,
	}
	var err error
	if testInput.CallCreate {
		_, err = h.Create(testInput.Token)
	} else {
		_, err = h.Updated(testInput.Token)
	}
	return &testOutput{
		ModifiedClusterAuthToken:  modifiedToken,
		ModifiedClusterAuthSecret: modifiedSecret,
		AuthTokenUpdated:          isUpdated,
		AuthTokenDeleted:          isDeleted,
		Error:                     err,
	}
}
