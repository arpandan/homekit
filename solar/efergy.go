package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/Sirupsen/logrus"
)

const readingsEndpointFmt = "https://engage.efergy.com/mobile_proxy/getCurrentValuesSummary?token=%s"

type EfergyClient struct {
	token           string
	log             *logrus.Entry
	refreshInterval time.Duration
}

type Reading struct {
	Cid       string `json:"cid"`
	LastValue int
	Data      []map[string]int `json:"data"`
	Sid       string           `json:"sid"`
	Units     string           `json:"units"`
	Age       int              `json:"age"`
}

func (reading *Reading) ParseLastValue() {
	// Find the newest reading key.
	if len(reading.Data) > 0 {
		for key := range reading.Data[0] {
			reading.LastValue = reading.Data[0][key]
			return
		}
	}
}

func NewEfergyClient(token string, refreshInterval time.Duration, log *logrus.Entry) *EfergyClient {
	return &EfergyClient{
		token:           token,
		log:             log,
		refreshInterval: refreshInterval,
	}
}

type Subscription struct {
	readings chan []*Reading
	quitChan chan struct{}
	wg       sync.WaitGroup

	client *EfergyClient
}

func (e *EfergyClient) Subscribe() *Subscription {
	s := &Subscription{
		readings: make(chan []*Reading),
		quitChan: make(chan struct{}),
		wg:       sync.WaitGroup{},
		client:   e,
	}

	go func() {
		ticker := time.NewTicker(e.refreshInterval)
		for {
			select {
			case <-ticker.C:
				readings, err := e.getReadings()
				if err != nil {
					log.Error(err)
				}
				s.write(readings)
			case <-s.quitChan:
				ticker.Stop()
			}
		}
	}()

	return s
}

func (s *Subscription) Readings() <-chan []*Reading {
	return s.readings
}

func (s *Subscription) write(reading []*Reading) {
	s.wg.Add(1)
	defer s.wg.Done()
	s.readings <- reading
}

func (s *Subscription) Close() {
	select {
	default:
		close(s.quitChan)

		// Wait for write operations to complete.
		s.wg.Wait()
		close(s.readings)
	}
}

func (client *EfergyClient) getReadings() ([]*Reading, error) {
	resp, err := http.Get(fmt.Sprintf(readingsEndpointFmt, client.token))
	if resp.StatusCode != http.StatusOK {
		return nil, err
	}

	client.log.Println(resp.Body)

	var data []*Reading
	decoder := json.NewDecoder(resp.Body)
	err = decoder.Decode(&data)
	if err != nil {
		return nil, err
	}

	// Parse the readings.
	for _, reading := range data {
		reading.ParseLastValue()
		client.log.Println(reading)
	}

	return data, nil
}
