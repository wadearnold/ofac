// Copyright 2018 The Moov Authors
// Use of this source code is governed by an Apache License
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"time"
)

var (
	watchResearchBatchSize = 100
)

func init() {
	watchResearchBatchSize = readWebhookBatchSize(os.Getenv("WEBHOOK_BATCH_SIZE"))
}

func readWebhookBatchSize(str string) int {
	if str == "" {
		return watchResearchBatchSize
	}
	d, _ := strconv.Atoi(str)
	if d > 0 {
		return d
	}
	return watchResearchBatchSize
}

// spawnResearching will block and select on updates for when to re-inspect all watches setup.
// Since watches are used to post OFAC data via webhooks they are used as catalysts in other systems.
func (s *searcher) spawnResearching(companyRepo companyRepository, custRepo customerRepository, watchRepo watchRepository, webhookRepo webhookRepository, updates chan *downloadStats) {
	for {
		select {
		case <-updates:
			s.logger.Log("search", "async: starting re-search of watches")
			cursor := watchRepo.getWatchesCursor(watchResearchBatchSize)
			for {
				watches, _ := cursor.Next()
				if len(watches) == 0 {
					break
				}
				for i := range watches {
					var body *bytes.Buffer
					var err error

					// Load up JSON body for webhook request
					if watches[i].companyId != "" {
						s.logger.Log("search", fmt.Sprintf("async: watch %s for company %s found", watches[i].id, watches[i].companyId))
						body, err = getCompanyBody(s, watches[i].id, watches[i].companyId, companyRepo)
					}
					if watches[i].customerId != "" {
						s.logger.Log("search", fmt.Sprintf("async: watch %s for customer %s found", watches[i].id, watches[i].customerId))
						body, err = getCustomerBody(s, watches[i].id, watches[i].customerId, custRepo)
					}
					if err != nil {
						s.logger.Log("search", fmt.Sprintf("async: watch %s: %v", watches[i].id, err))
						continue // skip to next watch
					}

					// Send HTTP webhook
					now := time.Now()
					status, err := callWebhook(watches[i].id, body, watches[i].webhook, watches[i].authToken)
					if err != nil {
						s.logger.Log("search", fmt.Errorf("async: problem writing watch (%s) webhook status: %v", watches[i].id, err))
					}
					if err := webhookRepo.recordWebhook(watches[i].id, now, status); err != nil {
						s.logger.Log("search", fmt.Errorf("async: problem writing watch (%s) webhook status: %v", watches[i].id, err))
					}
				}
			}

		}
	}
}

// getCustomerBody returns the JSON encoded form of a given customer by their EntityID
func getCustomerBody(s *searcher, watchId string, customerId string, repo customerRepository) (*bytes.Buffer, error) {
	customer, _ := getCustomerById(customerId, s, repo)
	if customer == nil {
		return nil, fmt.Errorf("async: watch %s customer %v not found", watchId, customerId)
	}
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(customer); err != nil {
		return nil, fmt.Errorf("problem creating JSON for customer watch %s: %v", watchId, err)
	}
	return &buf, nil
}

// getCompanyBody returns the JSON encoded form of a given customer by their EntityID
func getCompanyBody(s *searcher, watchId string, companyId string, repo companyRepository) (*bytes.Buffer, error) {
	company, _ := getCompanyById(companyId, s, repo)
	if company == nil {
		return nil, fmt.Errorf("async: watch %s company %v not found", watchId, companyId)
	}
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(company); err != nil {
		return nil, fmt.Errorf("problem creating JSON for company watch %s: %v", watchId, err)
	}
	return &buf, nil
}
