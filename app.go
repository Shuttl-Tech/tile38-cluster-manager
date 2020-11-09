package main

import (
	"github.com/urfave/cli/v2"
	"tile38-cluster-manager/core"
	"tile38-cluster-manager/version"
)

var config = new(core.Config)

var App = &cli.App{
	Name:    "manager",
	Usage:   "Start the cluster manager",
	Version: version.String(),
	Action:  core.Handler(config),
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:        "id",
			Usage:       "Unique ID for the service instance",
			EnvVars:     []string{"SERVICE_ID"},
			Destination: &config.ID,
			Required:    true,
		},
		&cli.StringFlag{
			Name:        "advertise",
			Usage:       "Network address to advertise in consul catalog",
			EnvVars:     []string{"ADVERTISE"},
			Destination: &config.Advertise,
			Required:    true,
		},
		&cli.StringFlag{
			Name:        "server.addr",
			Usage:       "Address of the server",
			EnvVars:     []string{"SERVER_ADDR"},
			Value:       "127.0.0.1",
			Destination: &config.ServerAddr,
		},
		&cli.IntFlag{
			Name:        "server.port",
			Usage:       "Network port of the server",
			EnvVars:     []string{"SERVER_PORT"},
			Required:    true,
			Destination: &config.ServerPort,
		},
		&cli.StringFlag{
			Name:        "bind.addr.leader",
			Usage:       "Network address for leader listener",
			EnvVars:     []string{"BIND_ADDR_LEADER"},
			Value:       "0.0.0.0",
			Destination: &config.LeaderBindAddr,
		},
		&cli.IntFlag{
			Name:        "bind.port.leader",
			Usage:       "Network port for the leader listener",
			EnvVars:     []string{"BIND_PORT_LEADER"},
			Required:    true,
			Destination: &config.LeaderBindPort,
		},
		&cli.StringFlag{
			Name:        "bind.addr.follower",
			Usage:       "Network address for follower listener",
			EnvVars:     []string{"BIND_ADDR_FOLLOWER"},
			Value:       "0.0.0.0",
			Destination: &config.FollowerBindAddr,
		},
		&cli.IntFlag{
			Name:        "bind.port.follower",
			Usage:       "Network port for the follower listener",
			EnvVars:     []string{"BIND_PORT_FOLLOWER"},
			Required:    true,
			Destination: &config.FollowerBindPort,
		},
		&cli.StringFlag{
			Name:        "consul.addr",
			Usage:       "IP and port of the consul agent",
			EnvVars:     []string{"CONSUL_HTTP_ADDR"},
			Value:       "127.0.0.1:8500",
			Destination: &config.ConsulAddr,
		},
		&cli.StringFlag{
			Name:        "consul.lock.prefix",
			Usage:       "KV path prefix to manage locks",
			EnvVars:     []string{"CONSUL_LOCK_PREFIX"},
			Value:       "_locks/tile38/cluster/manager",
			Destination: &config.ConsulLockPrefix,
		},
		&cli.StringFlag{
			Name:        "dogstatsd.addr",
			Usage:       "Address of the DogStatsd agent",
			EnvVars:     []string{"DD_AGENT_HOST"},
			Value:       "127.0.0.1",
			Destination: &config.DogstatsdAddr,
		},
		&cli.StringFlag{
			Name:        "dogstatsd.port",
			Usage:       "Port of the DogStatsd agent",
			EnvVars:     []string{"DD_AGENT_PORT"},
			Value:       "8126",
			Destination: &config.DogstatsdPort,
		},
		&cli.BoolFlag{
			Name:        "service.manage",
			Usage:       "Manage self registration in consul catalog when true",
			EnvVars:     []string{"SERVICE_MANAGE"},
			Value:       false,
			Destination: &config.ManageServiceRegistration,
		},
		&cli.StringFlag{
			Name:        "service.name.leader",
			Usage:       "Service name of the leader listener",
			EnvVars:     []string{"SERVICE_NAME_LEADER"},
			Value:       "tile38-writer",
			Destination: &config.LeaderServiceName,
		},
		&cli.StringFlag{
			Name:        "service.name.follower",
			Usage:       "Service name of the follower listener",
			EnvVars:     []string{"SERVICE_NAME_FOLLOWER"},
			Value:       "tile38-reader",
			Destination: &config.FollowerServiceName,
		},
	},
}
