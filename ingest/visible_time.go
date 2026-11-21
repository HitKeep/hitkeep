package ingest

import (
	"context"
	"time"

	"github.com/nsqio/go-nsq"

	"hitkeep/api"
	"hitkeep/database"
	json "hitkeep/jsonapi"
)

const (
	// Both the topic and the consumer channel are ephemeral so unmatched public
	// samples cannot spill to disk during a burst or persistence outage.
	VisibleTimeTopic       = "page_visible_time#ephemeral"
	visibleTimeChannel     = "db-writer#ephemeral"
	visibleTimeAttempts    = 4
	visibleTimeMaxQueueAge = 10 * time.Second
	visibleTimeRetryDelay  = time.Second
)

type visibleTimeDelivery struct {
	sample  api.PageVisibleTimeSample
	matched bool
}

func (c *Consumer) writeVisibleTimeBatch(store *database.Store, ctx context.Context, deliveries []*visibleTimeDelivery) error {
	samples := make([]api.PageVisibleTimeSample, len(deliveries))
	for i, delivery := range deliveries {
		samples[i] = delivery.sample
	}
	matched, err := c.tenantMgr.ApplyPageVisibleTime(ctx, store, samples)
	if err != nil {
		return err
	}
	for i, delivery := range deliveries {
		delivery.matched = matched[i]
	}
	return nil
}

func (c *Consumer) handleVisibleTime(m *nsq.Message) error {
	m.DisableAutoResponse()
	if time.Since(time.Unix(0, m.Timestamp)) >= visibleTimeMaxQueueAge {
		m.Finish()
		return nil
	}

	var delivery visibleTimeDelivery
	if err := json.Unmarshal(m.Body, &delivery.sample); err != nil {
		m.Finish()
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	siteID := delivery.sample.SiteID
	store, tenantID, err := c.tenantMgr.ResolveSiteStore(ctx, siteID)
	if err != nil {
		c.retryVisibleTime(m)
		return nil
	}
	if tenantID != delivery.sample.TenantID {
		m.Finish()
		return nil
	}

	result, err := c.visibleTimeBatcher.Enqueue(batchItem[*visibleTimeDelivery]{value: &delivery, store: store})
	if err == nil {
		err = <-result
	}
	if err != nil {
		c.logger.Error("Failed to persist page visible time", "site_id", siteID, "error", err)
	}
	if err != nil || !delivery.matched {
		c.retryVisibleTime(m)
		return nil
	}
	m.Finish()
	return nil
}

func (c *Consumer) retryVisibleTime(m *nsq.Message) {
	if m.Attempts < visibleTimeAttempts && time.Since(time.Unix(0, m.Timestamp))+visibleTimeRetryDelay < visibleTimeMaxQueueAge {
		m.RequeueWithoutBackoff(visibleTimeRetryDelay)
		return
	}
	m.Finish()
}
