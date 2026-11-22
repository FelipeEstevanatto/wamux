package whatsmeow_service

import (
	"context"
	"time"
)

const idleEvictCheck = time.Minute

// shouldEvictIdle is the guard set for idle runtime eviction: the client must be
// disconnected, no reconnect may be waiting, and it must not be mid-pairing.
func shouldEvictIdle(now, idleSince time.Time, minutes int, connected, reconnectPending, pairing bool) bool {
	if minutes <= 0 || connected || reconnectPending || pairing {
		return false
	}
	if idleSince.IsZero() {
		return false
	}
	return now.Sub(idleSince) >= time.Duration(minutes)*time.Minute
}

// StartIdleEviction removes a disconnected instance's runtime client after
// INSTANCE_IDLE_EVICT_MINUTES. The DB row, JID and stored session are kept, so a
// later POST /instance/connect starts a fresh client and resumes it. This bounds
// memory for fleets with many long-offline instances.
func (w whatsmeowService) StartIdleEviction(ctx context.Context) {
	if w.config == nil || w.config.InstanceIdleEvictMinutes <= 0 {
		return
	}
	ticker := time.NewTicker(idleEvictCheck)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			now := time.Now()
			for id, mycli := range w.myClientPointer.Snapshot() {
				if mycli == nil || mycli.WAClient == nil {
					continue
				}
				connected := mycli.WAClient.IsConnected()
				pairing := mycli.qrcodeCount > 0 && (mycli.Instance == nil || mycli.Instance.Jid == "")
				if mycli.passkeyCeremony != nil && mycli.passkeyCeremony.HasActiveByInstance(id) {
					pairing = true
				}
				var idleSince time.Time
				if health := w.InstanceHealth(id); health != nil {
					if health.LastDisconnectAt != "" {
						idleSince, _ = time.Parse(time.RFC3339, health.LastDisconnectAt)
					} else if health.ObservedSince != "" {
						idleSince, _ = time.Parse(time.RFC3339, health.ObservedSince)
					}
				}
				if !shouldEvictIdle(now, idleSince, w.config.InstanceIdleEvictMinutes, connected, reconnectPending(id), pairing) {
					continue
				}
				w.loggerWrapper.GetLogger(id).LogInfo("[%s] evicting idle runtime client (disconnected for more than %d minute(s)); session kept", id, w.config.InstanceIdleEvictMinutes)
				if err := w.instanceRepository.UpdateConnected(id, false, "Runtime evicted while idle; connect again to resume"); err != nil {
					w.loggerWrapper.GetLogger(id).LogWarn("[%s] could not record idle eviction: %v", id, err)
				}
				go w.StopClientRuntime(id)
			}
		}
	}
}
