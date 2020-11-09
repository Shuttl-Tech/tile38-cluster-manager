package core

import (
	"context"
	"fmt"
	"github.com/hashicorp/consul/agent/connect"
	"github.com/hashicorp/consul/api"
	"log"
	"net/url"
	"sort"
	"strings"
	"time"
)

type Peer struct {
	ID      string
	Kind    Role
	Address string
	Port    int
	CertURI connect.CertURI
}

func (p *Peer) String() string {
	return fmt.Sprintf("%s:%d", p.ID[:8], p.Port)
}

func (p *Peer) Equal(other *Peer) bool {
	if p == nil {
		return other == nil
	}

	if other == nil {
		return p == nil
	}

	return p.ID == other.ID &&
		p.Kind == other.Kind &&
		p.Address == other.Address &&
		p.Port == other.Port
}

type Peers []*Peer

func (p Peers) String() string {
	var result []string
	for _, peer := range p {
		result = append(result, peer.String())
	}
	return strings.Join(result, ",")
}

func (p Peers) GetById(id string) *Peer {
	for _, peer := range p {
		if peer.ID == id {
			return peer
		}
	}

	return nil
}

func (p Peers) GetExceptId(id string) Peers {
	var result Peers
	for _, peer := range p {
		if peer.ID != id {
			result = append(result, peer)
		}
	}

	return result
}

func (p Peers) Sort() {
	sortFn := func(i, j int) bool {
		return p[i].ID < p[j].ID
	}

	sort.Slice(p, sortFn)
}

func (p Peers) Equal(others Peers) bool {
	if len(p) != len(others) {
		return false
	}

	if len(p) == 0 {
		return true
	}

	p.Sort()
	others.Sort()

	for i := 0; i < len(p); i++ {
		if !p[i].Equal(others[i]) {
			return false
		}
	}

	return true
}

type forcedUpdate struct {
	change   <-chan Membership
	leader   chan struct{}
	follower chan struct{}
}

func (f *forcedUpdate) WatchAndForce(ctx *Context) {
	ctx.Begin()
	defer ctx.End()

	for {
		select {
		case <-ctx.Done():
			return
		case <-f.change:
			go func() {
				f.leader <- struct{}{}
				f.follower <- struct{}{}
			}()
		}
	}
}

func (m *Manager) WatchPeers(ctx *Context, leaders chan<- Peers, followers chan<- Peers, force *forcedUpdate) {
	ctx.Begin()
	defer ctx.End()

	log.Println("starting peer watcher loop")

	go m.watchService(ctx, m.config.LeaderServiceName, leaders, force.leader)
	go m.watchService(ctx, m.config.FollowerServiceName, followers, force.follower)

	<-ctx.Done()
	log.Println("stopping peer watcher loop")
}

type watchResult struct {
	services []*api.CatalogService
	meta     *api.QueryMeta
	err      error
}

func (m *Manager) watchService(ctx context.Context, name string, out chan<- Peers, force <-chan struct{}) {
	opts := &api.QueryOptions{
		RequireConsistent: true,
		UseCache:          true,
		WaitIndex:         0,
		WaitTime:          30 * time.Second,
	}

	outChan := make(chan *watchResult)

	for {
		optCx, cancel := context.WithCancel(ctx)
		go watchCatalog(m.consul, name, opts.WithContext(optCx), outChan)

		select {
		case <-ctx.Done():
			cancel()
			return

		case <-force:
			cancel()
			opts.WaitIndex = 0

		case result := <-outChan:
			if result.err != nil {
				if uer, ok := result.err.(*url.Error); ok && uer.Err.Error() == "context canceled" {
					// the blocking query was interrupted by another event.
					// we cause this deliberately and expect this to happen, so this isn't really an error.
					// TODO: Do something about this? preferably in a way that is not misleading or verbose
				} else {
					log.Printf("failed to query consul for available %s service. %s. %T", name, result.err, result.err)
				}

				time.Sleep(time.Second)
				continue
			}

			if result.meta.LastIndex <= opts.WaitIndex {
				continue
			}

			opts.WaitIndex = result.meta.LastIndex

			if len(result.services) == 0 {
				continue
			}

			peers := m.decodePeers(result.services)
			if len(peers) == 0 {
				continue
			}

			out <- peers
		}
	}
}

func (m *Manager) decodePeers(services []*api.CatalogService) Peers {
	var peers Peers

	for _, entry := range services {
		var kind Role

		switch entry.ServiceName {
		case m.config.LeaderServiceName:
			kind = RoleLeader
		case m.config.FollowerServiceName:
			kind = RoleFollower
		default:
			// should never happen, but the case exists nonetheless.
			continue
		}

		id, ok := entry.ServiceMeta["id"]
		if !ok {
			// registered out of band?
			continue
		}

		peers = append(peers, &Peer{
			ID:      id,
			Kind:    kind,
			Address: entry.Address,
			Port:    entry.ServicePort,
			CertURI: &connect.SpiffeIDService{
				Namespace:  "default",
				Datacenter: entry.Datacenter,
				Service:    entry.ServiceName,
			},
		})
	}

	return peers
}

func watchCatalog(client *api.Client, name string, opts *api.QueryOptions, out chan<- *watchResult) {
	s, m, e := client.Catalog().Connect(name, "", opts)
	out <- &watchResult{
		services: s,
		meta:     m,
		err:      e,
	}
}
