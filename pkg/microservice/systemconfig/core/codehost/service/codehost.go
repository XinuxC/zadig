/*
Copyright 2021 The KodeRover Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package service

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"go.uber.org/zap"

	"github.com/koderover/zadig/pkg/microservice/systemconfig/core/codehost/internal/oauth"
	"github.com/koderover/zadig/pkg/microservice/systemconfig/core/codehost/internal/oauth/github"
	"github.com/koderover/zadig/pkg/microservice/systemconfig/core/codehost/internal/oauth/gitlab"
	"github.com/koderover/zadig/pkg/microservice/systemconfig/core/codehost/repository/models"
	"github.com/koderover/zadig/pkg/microservice/systemconfig/core/codehost/repository/mongodb"
	"github.com/koderover/zadig/pkg/shared/client/systemconfig"
	"github.com/koderover/zadig/pkg/tool/crypto"
)

func CreateCodeHost(codehost *models.CodeHost, _ *zap.SugaredLogger) (*models.CodeHost, error) {
	if codehost.Type == "codehub" {
		codehost.IsReady = "2"
	}
	if codehost.Type == "gerrit" {
		codehost.IsReady = "2"
		codehost.AccessToken = base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("%s:%s", codehost.Username, codehost.Password)))
	}
	codehost.CreatedAt = time.Now().Unix()
	codehost.UpdatedAt = time.Now().Unix()

	list, err := mongodb.NewCodehostColl().CodeHostList()
	if err != nil {
		return nil, err
	}
	codehost.ID = len(list) + 1
	return mongodb.NewCodehostColl().AddCodeHost(codehost)
}

func List(address, owner, source string, _ *zap.SugaredLogger) ([]*models.CodeHost, error) {
	return mongodb.NewCodehostColl().List(&mongodb.ListArgs{
		Address: address,
		Owner:   owner,
		Source:  source,
	})
}

func DeleteCodeHost(id int, _ *zap.SugaredLogger) error {
	return mongodb.NewCodehostColl().DeleteCodeHostByID(id)
}

func UpdateCodeHost(host *models.CodeHost, _ *zap.SugaredLogger) (*models.CodeHost, error) {
	return mongodb.NewCodehostColl().UpdateCodeHost(host)
}

func UpdateCodeHostByToken(host *models.CodeHost, _ *zap.SugaredLogger) (*models.CodeHost, error) {
	return mongodb.NewCodehostColl().UpdateCodeHostByToken(host)
}

func GetCodeHost(id int, _ *zap.SugaredLogger) (*models.CodeHost, error) {
	return mongodb.NewCodehostColl().GetCodeHostByID(id)
}

type State struct {
	CodeHostID  int    `json:"code_host_id"`
	RedirectURL string `json:"redirect_url"`
	CallbackURL string `json:"callback_url"`
}

func AuthCodeHost(redirectURI, codeHostCallbackURL string, codeHostID int, logger *zap.SugaredLogger) (redirectURL string, err error) {
	codeHost, err := GetCodeHost(codeHostID, logger)
	if err != nil {
		logger.Errorf("GetCodeHost:%s err:%s", codeHostID, err)
		return "", err
	}
	oauth, err := NewOAuth(codeHost.Type, codeHostCallbackURL, codeHost.ApplicationId, codeHost.ClientSecret, codeHost.Address)
	if err != nil {
		logger.Errorf("get Factory:%s err:%s", codeHost.Type, err)
		return "", err
	}
	stateStruct := State{
		CodeHostID:  codeHost.ID,
		RedirectURL: redirectURI,
		CallbackURL: codeHostCallbackURL,
	}
	bs, err := json.Marshal(stateStruct)
	if err != nil {
		logger.Errorf("Marshal err:%s", err)
		return "", err
	}
	aes, err := crypto.NewAes(crypto.GetAesKey())
	encrypted, err := aes.Encrypt(string(bs))
	if err != nil {
		return "", err
	}
	return oauth.LoginURL(encrypted), nil
}

func NewOAuth(provider, callbackURL, clientID, clientSecret, address string) (oauth.Oauth, error) {
	switch provider {
	case systemconfig.GitHubProvider:
		return github.New(callbackURL, clientID, clientSecret, address), nil
	case systemconfig.GitLabProvider:
		return gitlab.New(callbackURL, clientID, clientSecret, address), nil
	}
	return nil, errors.New("illegal provider")
}

func HandleCallback(stateStr string, r *http.Request, logger *zap.SugaredLogger) (string, error) {
	aes, err := crypto.NewAes(crypto.GetAesKey())
	decrypted, err := aes.Decrypt(stateStr)
	if err != nil {
		logger.Errorf("Decrypt err:%s", err)
		return "", err
	}

	var state State
	if err := json.Unmarshal([]byte(decrypted), &state); err != nil {
		logger.Errorf("Unmarshal err:%s", err)
		return "", err
	}
	codehost, err := GetCodeHost(state.CodeHostID, logger)
	if err != nil {
		return handle(state.RedirectURL, err)
	}
	o, err := NewOAuth(codehost.Type, state.CallbackURL, codehost.ApplicationId, codehost.ClientSecret, codehost.Address)
	if err != nil {
		return handle(state.RedirectURL, err)
	}
	token, err := o.HandleCallback(r)
	if err != nil {
		return handle(state.RedirectURL, err)
	}
	codehost.AccessToken = token.AccessToken
	codehost.RefreshToken = token.RefreshToken
	if _, err := UpdateCodeHostByToken(codehost, logger); err != nil {
		logger.Errorf("UpdateCodeHostByToken err:%s", err)
		return handle(state.RedirectURL, err)
	}
	return handle(state.RedirectURL, nil)
}

func handle(redirectURL string, err error) (string, error) {
	u, parseErr := url.Parse(redirectURL)
	if parseErr != nil {
		return "", parseErr
	}
	if err != nil {
		u.Query().Add("err", err.Error())
	} else {
		u.Query().Add("success", "true")
	}
	return u.String(), nil
}
