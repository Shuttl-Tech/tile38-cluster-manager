package core

import (
	"context"
	"fmt"
	"github.com/DataDog/datadog-go/statsd"
	"github.com/go-redis/redis/v8"
	"github.com/mitchellh/mapstructure"
	"log"
	"time"
)

func (m *Manager) StartMetricsFunnel(ctx *Context) {
	ctx.Begin()
	defer ctx.End()

	ticker := time.NewTicker(10 * time.Second)

	log.Println("starting metrics funnel")

	for {
		select {
		case <-ctx.Done():
			log.Println("stopping metrics funnel")
			return
		case <-ticker.C:
			flushMetrics(ctx, m.rdb, m.stats)
		}
	}
}

type SrvInfo struct {
	ID             string  `mapstructure:"id"`
	MemAlloc       float64 `mapstructure:"mem_alloc"`
	NumHooks       float64 `mapstructure:"num_hooks"`
	PointerSize    float64 `mapstructure:"pointer_size"`
	AofSize        float64 `mapstructure:"aof_size"`
	AvgItemSize    float64 `mapstructure:"avg_item_size"`
	HeapSize       float64 `mapstructure:"heap_size"`
	NumObjects     float64 `mapstructure:"num_objects"`
	Threads        float64 `mapstructure:"threads"`
	CPUs           float64 `mapstructure:"cpus"`
	HeapReleased   float64 `mapstructure:"heap_released"`
	MaxHeapSize    float64 `mapstructure:"max_heap_size"`
	NumCollections float64 `mapstructure:"num_collections"`
	NumPoints      float64 `mapstructure:"num_points"`
	InMemSize      float64 `mapstructure:"in_memory_size"`
	NumStrings     float64 `mapstructure:"num_strings"`
}

func getSrvInfo(ctx context.Context, srv *redis.Client) (*SrvInfo, error) {
	cmd := redis.NewStringStringMapCmd(ctx, "server")
	err := srv.Process(ctx, cmd)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve server stats. %s", err)
	}

	result, err := cmd.Result()
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve command results. %s", err)
	}

	m := new(SrvInfo)
	err = mapstructure.WeakDecode(result, m)
	if err != nil {
		return nil, fmt.Errorf("failed to decode server stats result. %s", err)
	}

	return m, nil
}

func flushMetrics(ctx context.Context, srv *redis.Client, metrics *statsd.Client) {
	m, err := getSrvInfo(ctx, srv)
	if err != nil {
		log.Printf("failed to load server info. %s", err)
		return
	}

	_ = metrics.Gauge("mem_alloc", m.MemAlloc, nil, 1)
	_ = metrics.Gauge("num_hooks", m.NumHooks, nil, 1)
	_ = metrics.Gauge("pointer_size", m.PointerSize, nil, 1)
	_ = metrics.Gauge("aof_size", m.AofSize, nil, 1)
	_ = metrics.Gauge("avg_item_size", m.AvgItemSize, nil, 1)
	_ = metrics.Gauge("heap_size", m.HeapSize, nil, 1)
	_ = metrics.Gauge("num_objects", m.NumObjects, nil, 1)
	_ = metrics.Gauge("threads", m.Threads, nil, 1)
	_ = metrics.Gauge("cpus", m.CPUs, nil, 1)
	_ = metrics.Gauge("heap_released", m.HeapReleased, nil, 1)
	_ = metrics.Gauge("max_heap_size", m.MaxHeapSize, nil, 1)
	_ = metrics.Gauge("num_collections", m.NumCollections, nil, 1)
	_ = metrics.Gauge("num_points", m.NumPoints, nil, 1)
	_ = metrics.Gauge("in_mem_size", m.InMemSize, nil, 1)
	_ = metrics.Gauge("num_strings", m.NumStrings, nil, 1)
}
