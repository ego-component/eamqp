package eamqp

import (
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/gotomicro/ego/core/elog"
	"github.com/gotomicro/ego/core/emetric"
	"github.com/gotomicro/ego/server/egovernor"
)

var instances = sync.Map{}

// InstanceStats is the governor/debug snapshot for a built eamqp client.
type InstanceStats struct {
	Health HealthInfo `json:"health"`
	Pool   PoolStats  `json:"pool"`
}

func init() {
	egovernor.HandleFunc("/debug/amqp/stats", func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewEncoder(w).Encode(stats()); err != nil {
			elog.EgoLogger.With(elog.FieldComponent(PackageName)).Error("encode amqp stats fail", elog.FieldErr(err))
		}
	})
	go monitor()
}

// LoadInstance returns a built client by component name.
func LoadInstance(name string) *Client {
	value, ok := instances.Load(name)
	if !ok {
		return nil
	}
	client, _ := value.(*Client)
	return client
}

func monitor() {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		instances.Range(func(key, val interface{}) bool {
			name, ok := key.(string)
			if !ok {
				return true
			}
			client, ok := val.(*Client)
			if !ok || client == nil {
				return true
			}
			recordStatsMetrics(name, client)
			return true
		})
	}
}

func stats() map[string]InstanceStats {
	out := make(map[string]InstanceStats)
	instances.Range(func(key, val interface{}) bool {
		name, ok := key.(string)
		if !ok {
			return true
		}
		client, ok := val.(*Client)
		if !ok || client == nil {
			return true
		}
		out[name] = InstanceStats{
			Health: client.HealthStatus(),
			Pool:   client.Stats(),
		}
		return true
	})
	return out
}

func recordStatsMetrics(name string, client *Client) {
	pool := client.Stats()
	emetric.ClientStatsGauge.Set(float64(pool.ConnectionsActive), metricTypeAMQP, name, "connections_active")
	emetric.ClientStatsGauge.Set(float64(pool.ConnectionsTotal), metricTypeAMQP, name, "connections_total")
	emetric.ClientStatsGauge.Set(float64(pool.ChannelsActive), metricTypeAMQP, name, "channels_active")
	emetric.ClientStatsGauge.Set(float64(pool.ChannelsAcquired), metricTypeAMQP, name, "channels_acquired")
	emetric.ClientStatsGauge.Set(float64(pool.ChannelsReturned), metricTypeAMQP, name, "channels_returned")
	emetric.ClientStatsGauge.Set(float64(pool.Reconnects), metricTypeAMQP, name, "reconnects")
}
