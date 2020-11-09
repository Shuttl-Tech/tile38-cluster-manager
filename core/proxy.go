package core

import (
	"fmt"
	"github.com/hashicorp/consul/connect"
	"log"
)

func (m *Manager) StartProxy(ctx *Context, leaders <-chan Peers, followers <-chan Peers) error {
	ctx.Begin()
	defer ctx.End()

	log.Println("starting proxy listeners")
	tlsLeader, err := connect.NewService(m.config.LeaderServiceName, m.consul)
	if err != nil {
		return fmt.Errorf("failed to acquire certificates for leader service. %s", err)
	}
	defer tlsLeader.Close()

	tlsFollower, err := connect.NewService(m.config.FollowerServiceName, m.consul)
	if err != nil {
		return fmt.Errorf("failed to acquire certificates for follower service. %s", err)
	}
	defer tlsFollower.Close()

	config := &ListenConfig{
		bindLeader:       fmt.Sprintf("%s:%d", m.config.LeaderBindAddr, m.config.LeaderBindPort),
		bindFollower:     fmt.Sprintf("%s:%d", m.config.FollowerBindAddr, m.config.FollowerBindPort),
		bindBridge:       fmt.Sprintf("%s:%d", m.bridgeAddr, m.bridgePort),
		tlsLeader:        tlsLeader.ServerTLSConfig(),
		tlsFollower:      tlsFollower.ServerTLSConfig(),
		peerDialFollower: tlsFollower.Dial,
		peerDialLeader:   tlsLeader.Dial,
	}

	listeners := NewListeners(config)
	err = listeners.Start(ctx)
	if err != nil {
		return fmt.Errorf("failed to start proxy listeners. %s", err)
	}

	repl := NewReplicator(m.rdb, m.bridgeAddr, m.bridgePort)

	<-m.roleReady

	var membership Membership
	roleChan := m.RoleChanged(ctx)

	self := &Peer{
		ID:      m.config.ID,
		Address: m.config.ServerAddr,
		Port:    m.config.ServerPort,
	}

	for {
		select {
		case <-ctx.Done():
			log.Println("shutting down the listeners")
			return listeners.Close()

		case update := <-roleChan:
			membership = update
			switch update.Role {
			case RoleLeader:
				repl.StopFollow(ctx)
			case RoleFollower:
				repl.BeginFollow(ctx)
			}

		case update := <-leaders:
			var newLeader *Peer
			currentLeader := listeners.GetLeader()

			if membership.Role == RoleLeader {
				newLeader = self
			} else {
				newLeader = update.GetById(membership.Leader)
			}

			if newLeader == nil {
				log.Printf("leader %s is not available in peer list. waiting for peer update", membership.Leader)
				continue
			}

			if currentLeader.Equal(newLeader) {
				continue
			}

			listeners.SetLeader(newLeader)

		case update := <-followers:
			var newFollowers Peers
			currentFollowers := listeners.GetFollowers()

			if membership.Role == RoleFollower {
				newFollowers = Peers{self}
			} else {
				newFollowers = update.GetExceptId(m.config.ID)
			}

			if len(newFollowers) == 0 {
				newFollowers = Peers{self}
			}

			if currentFollowers.Equal(newFollowers) {
				continue
			}

			listeners.SetFollowers(newFollowers)
		}
	}
}
