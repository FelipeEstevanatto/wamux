package cloud

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	instance_model "github.com/felipeestevanatto/wamux/pkg/instance/model"
)

// ListPhoneNumbers returns all phone numbers attached to the instance's WABA.
// Pagination follows only the opaque Graph cursor, never an absolute next URL.
func (c *Client) ListPhoneNumbers(ctx context.Context, token, wabaID, after string) ([]PhoneNumberInfo, string, error) {
	q := url.Values{}
	q.Set("fields", "id,display_phone_number,verified_name,quality_rating,status,code_verification_status,platform_type,throughput")
	q.Set("limit", "100")
	if after != "" {
		q.Set("after", after)
	}
	var out graphPage[PhoneNumberInfo]
	if err := c.doJSON(ctx, token, http.MethodGet, wabaID+"/phone_numbers?"+q.Encode(), nil, &out, true); err != nil {
		return nil, "", err
	}
	return out.Data, out.Paging.Cursors.After, nil
}

func (c *Client) GetPhoneNumber(ctx context.Context, token, phoneNumberID string) (PhoneNumberInfo, error) {
	q := url.Values{}
	q.Set("fields", "id,display_phone_number,verified_name,quality_rating,status,code_verification_status,platform_type,throughput")
	var out PhoneNumberInfo
	err := c.doJSON(ctx, token, http.MethodGet, phoneNumberID+"?"+q.Encode(), nil, &out, true)
	return out, err
}

func (c *Client) RequestPhoneCode(ctx context.Context, token, phoneNumberID, method, language string) error {
	payload := map[string]string{"code_method": method, "language": language}
	var out map[string]any
	return c.doJSON(ctx, token, http.MethodPost, phoneNumberID+"/request_code", payload, &out, false)
}

func (c *Client) VerifyPhoneCode(ctx context.Context, token, phoneNumberID, code string) error {
	var out map[string]any
	return c.doJSON(ctx, token, http.MethodPost, phoneNumberID+"/verify_code", map[string]string{"code": code}, &out, false)
}

func (c *Client) GetWABA(ctx context.Context, token, wabaID string) (WABAInfo, error) {
	q := url.Values{}
	q.Set("fields", "id,name,currency,timezone,account_review_status,business_verification_status,message_template_namespace")
	var out WABAInfo
	err := c.doJSON(ctx, token, http.MethodGet, wabaID+"?"+q.Encode(), nil, &out, true)
	return out, err
}

func (c *Client) ListBusinessWABAs(ctx context.Context, token, businessID string, owned bool, after string) ([]WABAInfo, string, error) {
	if strings.TrimSpace(businessID) == "" {
		return nil, "", &APIError{Status: http.StatusBadRequest, Code: "invalid_request", Message: "business id is required"}
	}
	q := url.Values{}
	q.Set("fields", "id,name,currency,timezone,account_review_status,business_verification_status,message_template_namespace")
	q.Set("limit", "100")
	if after != "" {
		q.Set("after", after)
	}
	edge := "client_whatsapp_business_accounts"
	if owned {
		edge = "owned_whatsapp_business_accounts"
	}
	var out graphPage[WABAInfo]
	if err := c.doJSON(ctx, token, http.MethodGet, businessID+"/"+edge+"?"+q.Encode(), nil, &out, true); err != nil {
		return nil, "", err
	}
	return out.Data, out.Paging.Cursors.After, nil
}

func (c *Client) GetAnalytics(ctx context.Context, token, wabaID, start, end, granularity, phoneNumbers string) (json.RawMessage, error) {
	if start == "" || end == "" {
		return nil, &APIError{Status: http.StatusBadRequest, Code: "invalid_request", Message: "start and end are required"}
	}
	q := url.Values{}
	q.Set("start", start)
	q.Set("end", end)
	if granularity == "" {
		granularity = "DAY"
	}
	q.Set("granularity", strings.ToUpper(granularity))
	if phoneNumbers != "" {
		q.Set("phone_numbers", phoneNumbers)
	}
	var out json.RawMessage
	if err := c.doJSON(ctx, token, http.MethodGet, wabaID+"/analytics?"+q.Encode(), nil, &out, true); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *Client) ListCatalogs(ctx context.Context, token, businessID, after string) ([]CatalogInfo, string, error) {
	if strings.TrimSpace(businessID) == "" {
		return nil, "", &APIError{Status: http.StatusBadRequest, Code: "invalid_request", Message: "business id is required"}
	}
	q := url.Values{}
	q.Set("fields", "id,name,product_count,vertical")
	q.Set("limit", "100")
	if after != "" {
		q.Set("after", after)
	}
	var out graphPage[CatalogInfo]
	if err := c.doJSON(ctx, token, http.MethodGet, businessID+"/owned_product_catalogs?"+q.Encode(), nil, &out, true); err != nil {
		return nil, "", err
	}
	return out.Data, out.Paging.Cursors.After, nil
}

func (c *Client) CreateCatalog(ctx context.Context, token, businessID, name string) (CatalogInfo, error) {
	if strings.TrimSpace(name) == "" {
		return CatalogInfo{}, &APIError{Status: http.StatusBadRequest, Code: "invalid_request", Message: "catalog name is required"}
	}
	var out CatalogInfo
	err := c.doJSON(ctx, token, http.MethodPost, businessID+"/owned_product_catalogs", map[string]string{"name": name}, &out, false)
	return out, err
}

func (c *Client) ListProducts(ctx context.Context, token, catalogID, after string) ([]ProductInfo, string, error) {
	q := url.Values{}
	q.Set("fields", "id,retailer_id,name,description,availability,condition,price,currency,image_url,url,brand,quantity_to_sell_on_facebook")
	q.Set("limit", "100")
	if after != "" {
		q.Set("after", after)
	}
	var out graphPage[ProductInfo]
	if err := c.doJSON(ctx, token, http.MethodGet, catalogID+"/products?"+q.Encode(), nil, &out, true); err != nil {
		return nil, "", err
	}
	return out.Data, out.Paging.Cursors.After, nil
}

func (c *Client) CreateProduct(ctx context.Context, token, catalogID string, payload any) (ProductInfo, error) {
	var out ProductInfo
	err := c.doJSON(ctx, token, http.MethodPost, catalogID+"/products", payload, &out, false)
	return out, err
}

type graphPage[T any] struct {
	Data   []T `json:"data"`
	Paging struct {
		Next    string `json:"next"`
		Cursors struct {
			After string `json:"after"`
		} `json:"cursors"`
	} `json:"paging"`
}

// BusinessContext is the non-secret identity used for Business Management calls.
type BusinessContext struct {
	WABAID     string `json:"wabaId"`
	PhoneID    string `json:"phoneNumberId"`
	BusinessID string `json:"businessId"`
}

func (s *Service) BusinessContext(instance *instance_model.Instance) (*BusinessContext, error) {
	c, err := s.repo.GetCredentials(instance.Id)
	if err != nil {
		return nil, errors.New("cloud: credentials are missing")
	}
	return &BusinessContext{WABAID: c.WABAID, PhoneID: c.PhoneNumberID, BusinessID: c.BusinessID}, nil
}

func (s *Service) ListBusinessPhoneNumbers(instance *instance_model.Instance) ([]PhoneNumberInfo, error) {
	c, token, err := s.credentialsToken(instance.Id)
	if err != nil {
		return nil, err
	}
	var all []PhoneNumberInfo
	after := ""
	for page := 0; page < 50; page++ {
		items, next, err := s.client.ListPhoneNumbers(context.Background(), token, c.WABAID, after)
		if err != nil {
			return nil, MapGraphError(err)
		}
		all = append(all, items...)
		if next == "" || next == after {
			break
		}
		after = next
	}
	return all, nil
}

func (s *Service) GetBusinessPhone(instance *instance_model.Instance, phoneID string) (PhoneNumberInfo, error) {
	if err := s.requireOwnPhone(instance, phoneID); err != nil {
		return PhoneNumberInfo{}, err
	}
	_, token, err := s.credentialsToken(instance.Id)
	if err != nil {
		return PhoneNumberInfo{}, err
	}
	info, err := s.client.GetPhoneNumber(context.Background(), token, phoneID)
	return info, mapGraphErr(err)
}

// requireOwnPhone prevents an instance token from reading or mutating a number
// outside the WABA it is configured for, even if the Meta token is broadly
// scoped.
func (s *Service) requireOwnPhone(instance *instance_model.Instance, phoneID string) error {
	if strings.TrimSpace(phoneID) == "" {
		return &APIError{Status: http.StatusBadRequest, Code: "invalid_request", Message: "phone number id is required"}
	}
	phones, err := s.ListBusinessPhoneNumbers(instance)
	if err != nil {
		return err
	}
	for _, p := range phones {
		if p.ID == phoneID {
			return nil
		}
	}
	return &APIError{Status: http.StatusNotFound, Code: "not_found", Message: "phone number is not part of this WABA"}
}

func (s *Service) RegisterBusinessPhone(instance *instance_model.Instance, phoneID, pin string) error {
	if err := s.requireOwnPhone(instance, phoneID); err != nil {
		return err
	}
	_, token, err := s.credentialsToken(instance.Id)
	if err != nil {
		return err
	}
	if len(pin) != 6 {
		return &APIError{Status: http.StatusBadRequest, Code: "invalid_request", Message: "a six-digit PIN is required"}
	}
	return mapGraphErr(s.client.RegisterPhone(context.Background(), token, phoneID, pin))
}

func (s *Service) DeregisterBusinessPhone(instance *instance_model.Instance, phoneID string) error {
	if err := s.requireOwnPhone(instance, phoneID); err != nil {
		return err
	}
	_, token, err := s.credentialsToken(instance.Id)
	if err != nil {
		return err
	}
	return mapGraphErr(s.client.DeregisterPhone(context.Background(), token, phoneID))
}

func (s *Service) RequestBusinessCode(instance *instance_model.Instance, phoneID, method, language string) error {
	if err := s.requireOwnPhone(instance, phoneID); err != nil {
		return err
	}
	_, token, err := s.credentialsToken(instance.Id)
	if err != nil {
		return err
	}
	if method == "" {
		method = "SMS"
	}
	if language == "" {
		language = "en_US"
	}
	return mapGraphErr(s.client.RequestPhoneCode(context.Background(), token, phoneID, method, language))
}

func (s *Service) VerifyBusinessCode(instance *instance_model.Instance, phoneID, code string) error {
	if err := s.requireOwnPhone(instance, phoneID); err != nil {
		return err
	}
	_, token, err := s.credentialsToken(instance.Id)
	if err != nil {
		return err
	}
	if strings.TrimSpace(code) == "" {
		return &APIError{Status: http.StatusBadRequest, Code: "invalid_request", Message: "verification code is required"}
	}
	return mapGraphErr(s.client.VerifyPhoneCode(context.Background(), token, phoneID, code))
}

func (s *Service) GetBusinessWABA(instance *instance_model.Instance) (WABAInfo, error) {
	c, token, err := s.credentialsToken(instance.Id)
	if err != nil {
		return WABAInfo{}, err
	}
	info, err := s.client.GetWABA(context.Background(), token, c.WABAID)
	return info, mapGraphErr(err)
}

func (s *Service) GetBusinessAnalytics(instance *instance_model.Instance, start, end, granularity, phones string) (json.RawMessage, error) {
	c, token, err := s.credentialsToken(instance.Id)
	if err != nil {
		return nil, err
	}
	key := c.WABAID + "|" + start + "|" + end + "|" + granularity + "|" + phones
	if raw := s.cachedAnalytics(key); raw != nil {
		return raw, nil
	}
	raw, err := s.client.GetAnalytics(context.Background(), token, c.WABAID, start, end, granularity, phones)
	if err != nil {
		return nil, mapGraphErr(err)
	}
	s.cacheAnalytics(key, raw)
	return raw, nil
}

func (s *Service) cachedAnalytics(key string) json.RawMessage {
	s.businessMu.Lock()
	defer s.businessMu.Unlock()
	if e, ok := s.businessCache[key]; ok && time.Now().Before(e.expires) {
		return e.raw
	}
	return nil
}

func (s *Service) cacheAnalytics(key string, raw json.RawMessage) {
	s.businessMu.Lock()
	defer s.businessMu.Unlock()
	if s.businessCache == nil {
		s.businessCache = map[string]analyticsCacheEntry{}
	}
	s.businessCache[key] = analyticsCacheEntry{raw: raw, expires: time.Now().Add(2 * time.Minute)}
}

func (s *Service) ListBusinessCatalogs(instance *instance_model.Instance) ([]CatalogInfo, error) {
	c, token, err := s.credentialsToken(instance.Id)
	if err != nil {
		return nil, err
	}
	businessID := c.BusinessID
	var all []CatalogInfo
	after := ""
	for page := 0; page < 50; page++ {
		items, next, err := s.client.ListCatalogs(context.Background(), token, businessID, after)
		if err != nil {
			return nil, MapGraphError(err)
		}
		all = append(all, items...)
		if next == "" || next == after {
			break
		}
		after = next
	}
	return all, nil
}

func (s *Service) CreateBusinessCatalog(instance *instance_model.Instance, name string) (CatalogInfo, error) {
	c, token, err := s.credentialsToken(instance.Id)
	if err != nil {
		return CatalogInfo{}, err
	}
	item, err := s.client.CreateCatalog(context.Background(), token, c.BusinessID, name)
	return item, mapGraphErr(err)
}

func (s *Service) requireOwnCatalog(instance *instance_model.Instance, catalogID string) error {
	catalogs, err := s.ListBusinessCatalogs(instance)
	if err != nil {
		return err
	}
	for _, c := range catalogs {
		if c.ID == catalogID {
			return nil
		}
	}
	return &APIError{Status: http.StatusNotFound, Code: "not_found", Message: "catalog is not part of this business"}
}

func (s *Service) ListBusinessProducts(instance *instance_model.Instance, catalogID string) ([]ProductInfo, error) {
	if err := s.requireOwnCatalog(instance, catalogID); err != nil {
		return nil, err
	}
	_, token, err := s.credentialsToken(instance.Id)
	if err != nil {
		return nil, err
	}
	var all []ProductInfo
	after := ""
	for page := 0; page < 50; page++ {
		items, next, err := s.client.ListProducts(context.Background(), token, catalogID, after)
		if err != nil {
			return nil, MapGraphError(err)
		}
		all = append(all, items...)
		if next == "" || next == after {
			break
		}
		after = next
	}
	return all, nil
}

func (s *Service) CreateBusinessProduct(instance *instance_model.Instance, catalogID string, payload any) (ProductInfo, error) {
	if err := s.requireOwnCatalog(instance, catalogID); err != nil {
		return ProductInfo{}, err
	}
	_, token, err := s.credentialsToken(instance.Id)
	if err != nil {
		return ProductInfo{}, err
	}
	item, err := s.client.CreateProduct(context.Background(), token, catalogID, payload)
	return item, mapGraphErr(err)
}

func mapGraphErr(err error) error {
	if err == nil {
		return nil
	}
	var api *APIError
	if errors.As(err, &api) {
		return api
	}
	return MapGraphError(err)
}

type analyticsCacheEntry struct {
	raw     json.RawMessage
	expires time.Time
}
