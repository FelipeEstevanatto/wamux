package cloud

import "time"

// Observer receives Cloud-specific operational events. Implementations must be
// cheap and non-blocking; the methods run on request/send/webhook paths.
type Observer interface {
	ObserveCloudGraph(operation string, duration time.Duration, err error)
	ObserveCloudSend(kind, result string)
	ObserveCloudWebhook(field, result string)
	ObserveCloudWebhookSignatureFailure()
	ObserveCloudUnknownField(field string)
	ObserveCloudWindowExpired()
	ObserveCloudMediaJob(status string)
	ObserveCloudRateLimit(scope string, rps float64)
	ObserveCloudUnknownOutcome()
	SetCloudMediaQueueDepth(depth int)
}

// NopObserver is used when observability is not wired (tests, tools).
type NopObserver struct{}

func (NopObserver) ObserveCloudGraph(string, time.Duration, error) {}
func (NopObserver) ObserveCloudSend(string, string)                {}
func (NopObserver) ObserveCloudWebhook(string, string)             {}
func (NopObserver) ObserveCloudWindowExpired()                     {}
func (NopObserver) ObserveCloudMediaJob(string)                    {}
func (NopObserver) ObserveCloudRateLimit(string, float64)          {}
func (NopObserver) ObserveCloudUnknownOutcome()                    {}
func (NopObserver) SetCloudMediaQueueDepth(int)                    {}

func (s *Service) SetObserver(o Observer) { s.observer = o }

func (s *Service) SetPhoneLimiter(l PhoneLimiter) { s.phoneLimiter = l }

func (c *Client) SetObserver(o Observer) { c.observer = o }

func (s *Service) observeSend(kind string, err error) {
	if s.observer == nil {
		return
	}
	result := "ok"
	if err != nil {
		result = "error"
	}
	s.observer.ObserveCloudSend(kind, result)
}

func (s *Service) observeWebhook(field string, err error) {
	if s.observer == nil {
		return
	}
	result := "ok"
	if err != nil {
		result = "error"
	}
	s.observer.ObserveCloudWebhook(field, result)
}

func (s *Service) observeMedia(status string) {
	if s.observer != nil {
		s.observer.ObserveCloudMediaJob(status)
	}
}
