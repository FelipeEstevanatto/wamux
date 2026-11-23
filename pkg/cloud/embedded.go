package cloud

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	instance_model "github.com/felipeestevanatto/wamux/pkg/instance/model"
)

// EmbeddedAsset is one WABA/phone choice discovered during Embedded Signup.
type EmbeddedAsset struct {
	BusinessID    string `json:"businessId"`
	WABAID        string `json:"wabaId"`
	WABAName      string `json:"wabaName,omitempty"`
	PhoneNumberID string `json:"phoneNumberId"`
	DisplayPhone  string `json:"displayPhoneNumber,omitempty"`
	VerifiedName  string `json:"verifiedName,omitempty"`
}

// EmbeddedStart is the URL/state returned to the manager.
type EmbeddedStart struct {
	URL   string `json:"url"`
	State string `json:"state"`
}

type embeddedSession struct {
	InstanceID string
	AppID      string
	BusinessID string
	Token      string
	Assets     []EmbeddedAsset
	Expires    time.Time
}

// embeddedSessions is per-process. A callback that lands on another replica
// gets a clean state error; operators can retry rather than exposing a token.
var embeddedSessions = struct {
	sync.Mutex
	m map[string]embeddedSession
}{m: map[string]embeddedSession{}}

func (c *Client) ExchangeCode(ctx context.Context, appID, appSecret, code, redirectURI string) (string, time.Time, error) {
	q := url.Values{}
	q.Set("client_id", appID)
	q.Set("client_secret", appSecret)
	q.Set("code", code)
	if redirectURI != "" {
		q.Set("redirect_uri", redirectURI)
	}
	var out struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int64  `json:"expires_in"`
	}
	err := c.doJSON(ctx, "", http.MethodGet, "oauth/access_token?"+q.Encode(), nil, &out, true)
	if err != nil {
		return "", time.Time{}, err
	}
	if out.AccessToken == "" {
		return "", time.Time{}, errors.New("cloud: Meta did not return an access token")
	}
	expires := time.Time{}
	if out.ExpiresIn > 0 {
		expires = time.Now().Add(time.Duration(out.ExpiresIn) * time.Second)
	}
	return out.AccessToken, expires, nil
}

func (c *Client) DiscoverEmbeddedAssets(ctx context.Context, token string) (string, []EmbeddedAsset, error) {
	var me struct {
		ID string `json:"id"`
	}
	if err := c.doJSON(ctx, token, http.MethodGet, "me?fields=id", nil, &me, true); err != nil {
		return "", nil, err
	}
	businessID := me.ID
	if businessID == "" {
		return "", nil, errors.New("cloud: Meta did not return a business id")
	}
	var all []EmbeddedAsset
	after := ""
	for page := 0; page < 20; page++ {
		wabas, next, err := c.ListBusinessWABAs(ctx, token, businessID, true, after)
		if err != nil {
			return "", nil, err
		}
		for _, waba := range wabas {
			phones, _, err := c.ListPhoneNumbers(ctx, token, waba.ID, "")
			if err != nil {
				continue
			}
			for _, phone := range phones {
				all = append(all, EmbeddedAsset{
					BusinessID: businessID, WABAID: waba.ID, WABAName: waba.Name,
					PhoneNumberID: phone.ID, DisplayPhone: phone.DisplayPhoneNumber,
					VerifiedName: phone.VerifiedName,
				})
			}
		}
		if next == "" || next == after {
			break
		}
		after = next
	}
	return businessID, all, nil
}

func randomState() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// StartEmbeddedSignup creates a short-lived, instance-scoped state. The URL is
// a Facebook Login for Business dialog; the token is never placed in it.
func (s *Service) StartEmbeddedSignup(instance *instance_model.Instance) (*EmbeddedStart, error) {
	if s.cfg == nil || strings.TrimSpace(s.cfg.CloudAPIEmbeddedConfigID) == "" || strings.TrimSpace(s.cfg.CloudAPIEmbeddedRedirectURI) == "" {
		return nil, &APIError{Status: http.StatusServiceUnavailable, Code: "cloud_embedded_not_configured", Message: "Embedded Signup needs CLOUD_API_EMBEDDED_CONFIG_ID and CLOUD_API_EMBEDDED_REDIRECT_URI"}
	}
	appID := s.cfg.CloudAPIAppID
	if c, err := s.repo.GetCredentials(instance.Id); err == nil && c.AppID != "" {
		appID = c.AppID
	}
	if appID == "" {
		return nil, &APIError{Status: http.StatusServiceUnavailable, Code: "cloud_embedded_app_missing", Message: "Meta app id is not configured"}
	}
	state, err := randomState()
	if err != nil {
		return nil, err
	}
	embeddedSessions.Lock()
	embeddedSessions.m[state] = embeddedSession{
		InstanceID: instance.Id, AppID: appID, Expires: time.Now().Add(10 * time.Minute),
	}
	// Opportunistically clear expired states so a busy process cannot grow
	// this map without bound.
	for k, v := range embeddedSessions.m {
		if time.Now().After(v.Expires) {
			delete(embeddedSessions.m, k)
		}
	}
	embeddedSessions.Unlock()

	q := url.Values{}
	q.Set("client_id", appID)
	q.Set("config_id", s.cfg.CloudAPIEmbeddedConfigID)
	q.Set("response_type", "code")
	q.Set("override_default_response_type", "true")
	q.Set("redirect_uri", s.cfg.CloudAPIEmbeddedRedirectURI)
	q.Set("state", state)
	return &EmbeddedStart{
		URL:   "https://www.facebook.com/v27.0/dialog/oauth?" + q.Encode(),
		State: state,
	}, nil
}

// CompleteEmbeddedSignup exchanges the callback code and caches the assets.
// It is safe to call once per state; a second call is rejected.
func (s *Service) CompleteEmbeddedSignup(code, state string) ([]EmbeddedAsset, error) {
	embeddedSessions.Lock()
	session, ok := embeddedSessions.m[state]
	embeddedSessions.Unlock()
	if !ok || time.Now().After(session.Expires) {
		return nil, &APIError{Status: http.StatusBadRequest, Code: "cloud_embedded_state_invalid", Message: "Embedded Signup state is invalid or expired"}
	}
	if strings.TrimSpace(code) == "" {
		return nil, &APIError{Status: http.StatusBadRequest, Code: "invalid_request", Message: "authorization code is required"}
	}
	app, err := s.repo.GetApp(session.AppID)
	if err != nil {
		return nil, &APIError{Status: http.StatusServiceUnavailable, Code: "cloud_embedded_app_missing", Message: "Meta app is not configured"}
	}
	secret, err := s.decryptSecret(app.AppSecretEnc)
	if err != nil || secret == "" {
		return nil, &APIError{Status: http.StatusServiceUnavailable, Code: "cloud_embedded_secret_missing", Message: "Meta app secret is not configured"}
	}
	token, _, err := s.client.ExchangeCode(context.Background(), session.AppID, secret, code, s.cfg.CloudAPIEmbeddedRedirectURI)
	if err != nil {
		return nil, MapGraphError(err)
	}
	businessID, assets, err := s.client.DiscoverEmbeddedAssets(context.Background(), token)
	if err != nil {
		return nil, MapGraphError(err)
	}
	session.Token = token
	session.BusinessID = businessID
	session.Assets = assets
	embeddedSessions.Lock()
	embeddedSessions.m[state] = session
	embeddedSessions.Unlock()
	return assets, nil
}

// SelectEmbeddedAsset writes encrypted credentials and verifies/subscribes.
func (s *Service) SelectEmbeddedAsset(instance *instance_model.Instance, state, wabaID, phoneID string) (*CloudStatus, error) {
	embeddedSessions.Lock()
	session, ok := embeddedSessions.m[state]
	embeddedSessions.Unlock()
	if !ok || time.Now().After(session.Expires) || session.InstanceID != instance.Id {
		return nil, &APIError{Status: http.StatusBadRequest, Code: "cloud_embedded_state_invalid", Message: "Embedded Signup state is invalid or expired"}
	}
	var chosen *EmbeddedAsset
	for i := range session.Assets {
		a := session.Assets[i]
		if a.WABAID == wabaID && a.PhoneNumberID == phoneID {
			chosen = &a
			break
		}
	}
	if chosen == nil {
		return nil, &APIError{Status: http.StatusUnprocessableEntity, Code: "cloud_embedded_asset_invalid", Message: "the selected WABA/phone is not part of this signup"}
	}
	enforce := true
	if old, err := s.repo.GetCredentials(instance.Id); err == nil {
		enforce = old.EnforceWindow
	}
	if err := s.SaveCredentials(instance.Id, CredentialsInput{
		AppID: session.AppID, WABAID: chosen.WABAID, PhoneNumberID: chosen.PhoneNumberID,
		BusinessID: session.BusinessID, AccessToken: session.Token,
		DisplayPhoneNumber: chosen.DisplayPhone, EnforceWindow: &enforce,
	}); err != nil {
		return nil, err
	}
	embeddedSessions.Lock()
	delete(embeddedSessions.m, state)
	embeddedSessions.Unlock()
	status, err := s.Connect(instance)
	if err != nil {
		return nil, err
	}
	return status, nil
}

func (s *Service) decryptSecret(enc string) (string, error) {
	if enc == "" || s.codec == nil {
		return "", errors.New("cloud: secret is missing")
	}
	return s.codec.Decrypt(enc)
}

// EmbeddedAssets returns the non-secret assets cached for an active state.
func (s *Service) EmbeddedAssets(state string) ([]EmbeddedAsset, error) {
	embeddedSessions.Lock()
	session, ok := embeddedSessions.m[state]
	embeddedSessions.Unlock()
	if !ok || time.Now().After(session.Expires) {
		return nil, &APIError{Status: http.StatusBadRequest, Code: "cloud_embedded_state_invalid", Message: "Embedded Signup state is invalid or expired"}
	}
	return session.Assets, nil
}
