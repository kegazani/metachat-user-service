package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

type OAuthProvider interface {
	GetUserInfo(ctx context.Context, token string) (*OAuthUserInfo, error)
}

type OAuthUserInfo struct {
	ID            string
	Email         string
	FirstName     string
	LastName      string
	Provider      string
	ProviderToken string
}

type GoogleOAuthProvider struct {
	clientID string
}

func NewGoogleOAuthProvider(clientID string) *GoogleOAuthProvider {
	return &GoogleOAuthProvider{
		clientID: clientID,
	}
}

func (g *GoogleOAuthProvider) GetUserInfo(ctx context.Context, token string) (*OAuthUserInfo, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", "https://www.googleapis.com/oauth2/v2/userinfo", nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+token)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to get user info: status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var userInfo struct {
		ID        string `json:"id"`
		Email     string `json:"email"`
		GivenName string `json:"given_name"`
		FamilyName string `json:"family_name"`
	}

	if err := json.Unmarshal(body, &userInfo); err != nil {
		return nil, err
	}

	return &OAuthUserInfo{
		ID:        userInfo.ID,
		Email:     userInfo.Email,
		FirstName: userInfo.GivenName,
		LastName:  userInfo.FamilyName,
		Provider:  "google",
		ProviderToken: token,
	}, nil
}

type AppleOAuthProvider struct {
	clientID string
}

func NewAppleOAuthProvider(clientID string) *AppleOAuthProvider {
	return &AppleOAuthProvider{
		clientID: clientID,
	}
}

func (a *AppleOAuthProvider) GetUserInfo(ctx context.Context, token string) (*OAuthUserInfo, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", "https://appleid.apple.com/auth/userinfo", nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+token)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to get user info: status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var userInfo struct {
		Sub   string `json:"sub"`
		Email string `json:"email"`
	}

	if err := json.Unmarshal(body, &userInfo); err != nil {
		return nil, err
	}

	return &OAuthUserInfo{
		ID:        userInfo.Sub,
		Email:     userInfo.Email,
		Provider:  "apple",
		ProviderToken: token,
	}, nil
}

