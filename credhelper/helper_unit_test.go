// +build unit

// Copyright 2016 Google, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package credhelper

import (
	"errors"
	"testing"

	"github.com/GoogleCloudPlatform/docker-credential-gcr/mock/mock_config" // mocks must be generated before test execution
	"github.com/GoogleCloudPlatform/docker-credential-gcr/mock/mock_store"

	"github.com/GoogleCloudPlatform/docker-credential-gcr/config"
	"github.com/GoogleCloudPlatform/docker-credential-gcr/store"
	"github.com/docker/docker-credential-helpers/credentials"
	"github.com/golang/mock/gomock"
)

const (
	expectedGCRUsername = "oauth2accesstoken"
)

var gcrHosts = [...]string{
	"gcr.io",
	"us.gcr.io",
	"eu.gcr.io",
	"asia.gcr.io",
	"b.gcr.io",
	"bucket.gcr.io",
	"appengine.gcr.io",
	"gcr.kubernetes.io",
	"beta.gcr.io",
}
var otherHosts = [...]string{"docker.io", "otherrepo.com"}

func TestIsAGCRHostname(t *testing.T) {
	t.Parallel()
	// test for GCR hosts
	for _, host := range gcrHosts {
		if !isAGCRHostname(host) {
			t.Error("Expected to be detected as a GCR hostname: ", host)
		}
	}

	// test for GCR hosts + scheme
	for _, host := range gcrHosts {
		if !isAGCRHostname("https://" + host) {
			t.Error("Expected to be detected as a GCR hostname: ", "https://"+host)
		}
	}

	// test for non-GCR hosts
	for _, host := range otherHosts {
		if isAGCRHostname(host) {
			t.Error("Expected to not be a GCR host: ", host)
		}
	}
}

func TestAdd_GCRCredentials(t *testing.T) {
	t.Parallel()
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	mockStore := mock_store.NewMockGCRCredStore(mockCtrl)
	mockUserCfg := mock_config.NewMockUserConfig(mockCtrl)
	tested := NewGCRCredentialHelper(mockStore, mockUserCfg)

	creds := credentials.Credentials{
		Username: "foobarre",
		Secret:   "secret",
	}

	for _, host := range gcrHosts {
		creds.ServerURL = "https://" + host
		err := tested.Add(&creds)
		if err == nil {
			t.Error("Adding GCR credentials should return an error.")
		}
	}
}

func TestAdd_OtherCredentials(t *testing.T) {
	t.Parallel()
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	mockStore := mock_store.NewMockGCRCredStore(mockCtrl)
	mockUserCfg := mock_config.NewMockUserConfig(mockCtrl)

	tested := NewGCRCredentialHelper(mockStore, mockUserCfg)

	creds := credentials.Credentials{
		Username: "foobarre",
		Secret:   "secret",
	}

	for _, host := range otherHosts {
		creds.ServerURL = "https://" + host
		mockStore.EXPECT().SetOtherCreds(&creds).Return(nil)

		err := tested.Add(&creds)

		if err != nil {
			t.Errorf("Add returned an error: %v", err)
		}
	}
}

func TestGet_OtherCredentials(t *testing.T) {
	t.Parallel()
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	mockStore := mock_store.NewMockGCRCredStore(mockCtrl)
	mockUserCfg := mock_config.NewMockUserConfig(mockCtrl)
	expectedGCRSecret := "GCR secrets!"
	tested := &gcrCredHelper{
		store:   mockStore,
		userCfg: mockUserCfg,
		envToken: func() (string, error) {
			return expectedGCRSecret, nil
		},
		gcloudSDKToken: func() (string, error) {
			return "", errors.New("No token here!")
		},
		credStoreToken: func(_ store.GCRCredStore) (string, error) {
			return "", errors.New("No token here!")
		},
	}

	expectedUsername := "foobarre"
	expected3pSecret := "3p secrets!"
	creds := credentials.Credentials{
		Username: expectedUsername,
		Secret:   expected3pSecret,
	}

	// positive case
	for _, host := range otherHosts {
		mockStore.EXPECT().GetOtherCreds(host).Return(&creds, nil)

		username, secret, err := tested.Get(host)

		if err != nil {
			t.Errorf("Get returned an error: %v", err)
		} else if username != expectedUsername {
			t.Errorf("Expected username: %s but got: %s", expectedUsername, username)
		} else if secret != expected3pSecret {
			t.Errorf("Expected 3p secret: %s but got: %s", expected3pSecret, secret)
		}
	}

	// negative case - not found, not returning GCR's creds by default.
	mockStore.EXPECT().GetOtherCreds("somewhere.else").Return(nil, credentials.NewErrCredentialsNotFound())
	mockUserCfg.EXPECT().DefaultToGCRAccessToken().Return(false)

	_, _, err := tested.Get("somewhere.else")

	if err == nil {
		t.Error("Expected an error to be returned")
	} else if !credentials.IsErrCredentialsNotFound(err) {
		t.Errorf("Expected a CredentialsNotFound error: %v", err)
	}

	// negative case - 3p creds not found, but configured to return GCR's creds by default.
	mockStore.EXPECT().GetOtherCreds("somewhere.else").Return(nil, credentials.NewErrCredentialsNotFound())
	mockUserCfg.EXPECT().TokenSources().Return(config.DefaultTokenSources[:])
	mockUserCfg.EXPECT().DefaultToGCRAccessToken().Return(true)

	username, secret, err := tested.Get("somewhere.else")

	if err != nil {
		t.Errorf("Get returned an error: %v", err)
	} else if username != expectedGCRUsername {
		t.Errorf("Expected GCR username: %s but got: %s", expectedGCRUsername, username)
	} else if secret != expectedGCRSecret {
		t.Errorf("Expected GCR secret: %s but got: %s", expectedGCRSecret, secret)
	}
}

func TestGet_GCRCredentials(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	// create a mocks for the helper to use
	mockStore := mock_store.NewMockGCRCredStore(mockCtrl)
	mockUserCfg := mock_config.NewMockUserConfig(mockCtrl)

	// mock the helper methods used by getGCRAccessToken
	expectedSecret := "secrets!"
	tested := &gcrCredHelper{
		store:   mockStore,
		userCfg: mockUserCfg,
		envToken: func() (string, error) {
			return expectedSecret, nil
		},
		gcloudSDKToken: func() (string, error) {
			return "", errors.New("No token here!")
		},
		credStoreToken: func(_ store.GCRCredStore) (string, error) {
			return "", errors.New("No token here!")
		},
	}

	// Verify that all of GCR's hostnames return GCR's access token.
	for _, host := range gcrHosts {
		mockUserCfg.EXPECT().TokenSources().Return(config.DefaultTokenSources[:])
		username, secret, err := tested.Get("https://" + host)
		if err != nil {
			t.Errorf("Get returned an error: %v", err)
		} else if username != expectedGCRUsername {
			t.Errorf("Expected GCR username: %s but got: %s", expectedGCRUsername, username)
		} else if secret != expectedSecret {
			t.Errorf("Expected secret: %s but got: %s", expectedSecret, secret)
		}
	}
}

func TestDelete_GCRCredentials(t *testing.T) {
	t.Parallel()
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	mockStore := mock_store.NewMockGCRCredStore(mockCtrl)
	mockUserCfg := mock_config.NewMockUserConfig(mockCtrl)
	tested := NewGCRCredentialHelper(mockStore, mockUserCfg)

	for _, host := range gcrHosts {
		err := tested.Delete("https://" + host)
		if err == nil {
			t.Error("Deleting GCR credentials should return an error.")
		}
	}
}

func TestDelete_OtherCredentials(t *testing.T) {
	t.Parallel()
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	mockStore := mock_store.NewMockGCRCredStore(mockCtrl)
	mockUserCfg := mock_config.NewMockUserConfig(mockCtrl)
	tested := NewGCRCredentialHelper(mockStore, mockUserCfg)

	for _, host := range otherHosts {
		schemedHost := "https://" + host
		mockStore.EXPECT().DeleteOtherCreds(schemedHost).Return(nil)

		err := tested.Delete(schemedHost)

		if err != nil {
			t.Errorf("Delete returned an error: %v", err)
		}
	}
}

/*
	The following tests verify the behavior of getGCRAccessToken. Preference
	is defined by tokenSources
*/

func TestGetGCRAccessToken_Env(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	// create a mock store to use
	mockStore := mock_store.NewMockGCRCredStore(mockCtrl)

	mockUserCfg := mock_config.NewMockUserConfig(mockCtrl)
	mockUserCfg.EXPECT().TokenSources().Return(config.DefaultTokenSources[:])

	// mock the helper methods used by getGCRAccessToken
	const expected = "application default creds!"
	tested := &gcrCredHelper{
		store:   mockStore,
		userCfg: mockUserCfg,
		envToken: func() (string, error) {
			return expected, nil
		},
		gcloudSDKToken: func() (string, error) {
			return "gcloud sdk creds!", nil
		},
		credStoreToken: func(_ store.GCRCredStore) (string, error) {
			return "private creds!", nil
		},
	}

	token, err := tested.getGCRAccessToken()

	if err != nil {
		t.Fatalf("getGCRAccessToken returned an error: %v", err)
	} else if token != expected {
		t.Fatalf("Expected: %s got: %s", expected, token)
	}
}

func TestGetGCRAccessToken_GcloudSDK(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	// create a mock store to use
	mockStore := mock_store.NewMockGCRCredStore(mockCtrl)

	mockUserCfg := mock_config.NewMockUserConfig(mockCtrl)
	mockUserCfg.EXPECT().TokenSources().Return(config.DefaultTokenSources[:])

	// mock the helper methods used by getGCRAccessToken
	const expected = "gcloud sdk creds!"
	tested := &gcrCredHelper{
		store:   mockStore,
		userCfg: mockUserCfg,
		envToken: func() (string, error) {
			return "", errors.New("No token here!")
		},
		gcloudSDKToken: func() (string, error) {
			return expected, nil
		},
		credStoreToken: func(_ store.GCRCredStore) (string, error) {
			return "private creds!", nil
		},
	}

	token, err := tested.getGCRAccessToken()

	if err != nil {
		t.Fatalf("getGCRAccessToken returned an error: %v", err)
	} else if token != expected {
		t.Fatalf("Expected: %s got: %s", expected, token)
	}
}

func TestGetGCRAccessToken_PrivateStore(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	// create a mock store to use
	mockStore := mock_store.NewMockGCRCredStore(mockCtrl)

	mockUserCfg := mock_config.NewMockUserConfig(mockCtrl)
	mockUserCfg.EXPECT().TokenSources().Return(config.DefaultTokenSources[:])

	// mock the helper methods used by getGCRAccessToken
	const expected = "private creds!"
	tested := &gcrCredHelper{
		store:   mockStore,
		userCfg: mockUserCfg,
		envToken: func() (string, error) {
			return "", errors.New("No token here!")
		},
		gcloudSDKToken: func() (string, error) {
			return "", errors.New("Still no token here!")
		},
		credStoreToken: func(_ store.GCRCredStore) (string, error) {
			return expected, nil
		},
	}

	token, err := tested.getGCRAccessToken()

	if err != nil {
		t.Fatalf("getGCRAccessToken returned an error: %v", err)
	} else if token != expected {
		t.Fatalf("Expected: %s got: %s", expected, token)
	}
}

func TestGetGCRAccessToken_NoneExist(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	// create a mock store to use
	mockStore := mock_store.NewMockGCRCredStore(mockCtrl)

	mockUserCfg := mock_config.NewMockUserConfig(mockCtrl)
	mockUserCfg.EXPECT().TokenSources().Return(config.DefaultTokenSources[:])

	// mock the helper methods used by getGCRAccessToken
	tested := &gcrCredHelper{
		store:   mockStore,
		userCfg: mockUserCfg,
		envToken: func() (string, error) {
			return "", errors.New("No token here!")
		},
		gcloudSDKToken: func() (string, error) {
			return "", errors.New("Still no token here!")
		},
		credStoreToken: func(_ store.GCRCredStore) (string, error) {
			return "", errors.New("Sad panda!")
		},
	}

	token, err := tested.getGCRAccessToken()

	if err == nil {
		t.Fatalf("Expected an error, got token: %s", token)
	}
}

func TestGetGCRAccessToken_CustomTokenSources(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	// create a mock store to use
	mockStore := mock_store.NewMockGCRCredStore(mockCtrl)

	// Mock a user config, re-arranging the token sources.
	mockUserCfg := mock_config.NewMockUserConfig(mockCtrl)
	mockUserCfg.EXPECT().TokenSources().Return([]string{"store", "gcloud_sdk", "env"}) // reversed from default

	const (
		gcloudCreds = "gcloud sdk creds!"
		storeCreds  = "private creds!"
		envCreds    = "environment creds!"
	)
	// mock the helper methods used by getGCRAccessToken
	tested := &gcrCredHelper{
		store:   mockStore,
		userCfg: mockUserCfg,
		envToken: func() (string, error) {
			return envCreds, nil
		},
		gcloudSDKToken: func() (string, error) {
			return gcloudCreds, nil
		},
		credStoreToken: func(_ store.GCRCredStore) (string, error) {
			return storeCreds, nil
		},
	}

	token, err := tested.getGCRAccessToken()

	if err != nil {
		t.Fatalf("getGCRAccessToken returned an error: %v", err)
	} else if token != storeCreds {
		t.Fatalf("Expected: %s got: %s", storeCreds, token)
	}
}

func TestGetGCRAccessToken_CustomTokenSources_ValidSourceDisabled(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	// create a mock store to use
	mockStore := mock_store.NewMockGCRCredStore(mockCtrl)

	// Mock a user config, disabling some token sources.
	mockUserCfg := mock_config.NewMockUserConfig(mockCtrl)
	mockUserCfg.EXPECT().TokenSources().Return([]string{"gcloud_sdk"}) // gcloud only configured source

	const (
		storeCreds = "private creds!"
		envCreds   = "environment creds!"
	)
	// mock the helper methods used by getGCRAccessToken
	tested := &gcrCredHelper{
		store:   mockStore,
		userCfg: mockUserCfg,
		envToken: func() (string, error) {
			return envCreds, nil
		},
		gcloudSDKToken: func() (string, error) {
			return "", errors.New("No token here!")
		},
		credStoreToken: func(_ store.GCRCredStore) (string, error) {
			return storeCreds, nil
		},
	}

	token, err := tested.getGCRAccessToken()

	if err == nil {
		t.Fatalf("Expected an error, got token: %s", token)
	}
}

func TestGetGCRAccessToken_CustomTokenSources_InvalidSource(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	// create a mock store to use
	mockStore := mock_store.NewMockGCRCredStore(mockCtrl)

	mockUserCfg := mock_config.NewMockUserConfig(mockCtrl)
	mockUserCfg.EXPECT().TokenSources().Return([]string{"invalid"})

	const (
		gcloudCreds = "gcloud sdk creds!"
		storeCreds  = "private creds!"
		envCreds    = "environment creds!"
	)
	// mock the helper methods used by getGCRAccessToken
	tested := &gcrCredHelper{
		store:   mockStore,
		userCfg: mockUserCfg,
		envToken: func() (string, error) {
			return envCreds, nil
		},
		gcloudSDKToken: func() (string, error) {
			return gcloudCreds, nil
		},
		credStoreToken: func(_ store.GCRCredStore) (string, error) {
			return storeCreds, nil
		},
	}

	token, err := tested.getGCRAccessToken()

	if err == nil {
		t.Fatalf("Expected an error, got token: %s", token)
	}
}
