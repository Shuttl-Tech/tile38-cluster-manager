package core

type Config struct {
	// unique ID for our instance
	ID string

	// network IP to advertise in service catalog
	Advertise string

	// network address of our own instance of the database
	ServerAddr string
	ServerPort int

	// interface and port where our instance is listening
	// as leader service
	LeaderBindAddr string
	LeaderBindPort int

	// interface and port where our instance is listening
	// as follower service
	FollowerBindAddr string
	FollowerBindPort int

	// network address of consul agent and a KV prefix where
	// the leadership locks are managed
	ConsulAddr       string
	ConsulLockPrefix string

	// network address of dogstatsd agent
	DogstatsdAddr string
	DogstatsdPort string

	// whether to manage service registration and
	// deregistration in consul catalog
	ManageServiceRegistration bool

	// service name for leader and follower listeners
	LeaderServiceName   string
	FollowerServiceName string
}
